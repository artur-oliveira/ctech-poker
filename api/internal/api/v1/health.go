// Package v1 wires the poker HTTP routes onto a Fiber app.
package v1

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

var startTime = time.Now()

// checkTimeout caps the whole dependency fan-out so a hung dependency can never
// hold the probe open. Unused today — poker has no external dependency checks
// yet (see componentServer/CPU/Memory below) — kept named for when Phase 2
// wires in a load-bearing dependency (e.g. cache.Backend) whose check needs it.
const checkTimeout = 2 * time.Second

// Health check statuses (draft-inadarei-api-health-check).
const (
	statusPass = "pass"
	statusWarn = "warn"
	statusFail = "fail"
)

// statusMultiStatus is the HTTP code returned when the API serves traffic with a
// degraded (warn) dependency — the instance must stay in the load balancer.
const statusMultiStatus = 207

// Health check identity.
const (
	healthAPIVersion   = "/v1.0"
	healthServiceID    = "CTech Poker"
	healthDescription  = "Health check details for CTech Poker API"
	healthUnavailableV = -1 // observedValue when a check could not be measured
)

// Health check component names.
const (
	componentServer = "server"
	componentCPU    = "cpu"
	componentMemory = "memory"
)

// Health check component types and measurements.
const (
	typeSystem         = "system"
	measureUptime      = "uptime"
	measureUtilization = "utilization"
	unitSecond         = "second"
	unitPercent        = "percent"
)

// utilizationWarnPercent is the CPU/memory level above which the instance is
// reported as degraded.
const utilizationWarnPercent = 90

type healthEntry struct {
	ComponentName   string  `json:"componentName"`
	MeasurementName string  `json:"measurementName"`
	ComponentType   string  `json:"componentType"`
	ObservedValue   float64 `json:"observedValue"`
	ObservedUnit    string  `json:"observedUnit"`
	Status          string  `json:"status"`
	Time            string  `json:"time"`
}

type healthResponse struct {
	Status      string                 `json:"status"`
	Version     string                 `json:"version"`
	ReleaseID   string                 `json:"releaseId"`
	ServiceID   string                 `json:"serviceId"`
	Description string                 `json:"description"`
	Checks      map[string]healthEntry `json:"checks"`
}

// liveness is the dependency-free probe: it answers "is the process up", nothing
// more. It carries the running release so a deploy can be verified without an
// authenticated call.
type liveness struct {
	Status    string `json:"status"`
	ReleaseID string `json:"releaseId"`
	ServiceID string `json:"serviceId"`
}

// RegisterHealth mounts the liveness probe (/v1.0/health) and the detailed health
// check (/v1.0/health-check). The ALB target group probes the detailed one and
// accepts 200 and 207, so a degraded (warn) instance keeps serving traffic while
// a 503 takes it out of rotation.
//
// Poker has no external dependency wired into the Fx app yet (no DynamoDB, no
// cache.Backend) — only the system-level checks every service reports
// regardless of external deps are included here. Since none of these checks are
// load-bearing, aggregate() can only ever return pass or warn at this stage;
// that is correct, not a gap. Add dependency checks here once Phase 2 wires
// them into internal/app.
func RegisterHealth(router fiber.Router, cfg *config.Config) {
	router.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(liveness{Status: statusPass, ReleaseID: cfg.AppVersion, ServiceID: healthServiceID})
	})

	router.Get("/health-check", func(c fiber.Ctx) error {
		nowStr := time.Now().UTC().Format(time.RFC3339Nano)

		cpu := checkCPU(nowStr)
		mem := checkMemory(nowStr)

		uptime := healthEntry{
			ComponentName:   componentServer,
			MeasurementName: measureUptime,
			ComponentType:   typeSystem,
			ObservedValue:   time.Since(startTime).Seconds(),
			ObservedUnit:    unitSecond,
			Status:          statusPass,
			Time:            nowStr,
		}

		checks := map[string]healthEntry{
			measureUptime:   uptime,
			componentCPU:    cpu,
			componentMemory: mem,
		}

		overall, statusCode := aggregate(checks)
		return c.Status(statusCode).JSON(healthResponse{
			Status:      overall,
			Version:     healthAPIVersion,
			ReleaseID:   cfg.AppVersion,
			ServiceID:   healthServiceID,
			Description: healthDescription,
			Checks:      checks,
		})
	})
}

// aggregate reduces the individual checks to the overall status and HTTP code:
// any fail → 503, else any warn → 207, else 200.
func aggregate(checks map[string]healthEntry) (string, int) {
	overall := statusPass
	for _, e := range checks {
		if e.Status == statusFail {
			return statusFail, fiber.StatusServiceUnavailable
		}
		if e.Status == statusWarn {
			overall = statusWarn
		}
	}
	if overall == statusWarn {
		return statusWarn, statusMultiStatus
	}
	return statusPass, fiber.StatusOK
}

func checkCPU(nowStr string) healthEntry {
	pct := cpuPercent()
	st := statusPass
	if pct < 0 || pct > utilizationWarnPercent {
		st = statusWarn
	}
	return healthEntry{componentCPU, measureUtilization, typeSystem, pct, unitPercent, st, nowStr}
}

func checkMemory(nowStr string) healthEntry {
	pct := memoryPercent()
	st := statusPass
	if pct < 0 || pct > utilizationWarnPercent {
		st = statusWarn
	}
	return healthEntry{componentMemory, measureUtilization, typeSystem, pct, unitPercent, st, nowStr}
}

func cpuPercent() float64 {
	if runtime.GOOS != "linux" {
		return healthUnavailableV
	}
	f, err := os.Open("/proc/stat")
	if err != nil {
		return healthUnavailableV
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return healthUnavailableV
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return healthUnavailableV
	}
	var vals []int64
	for _, s := range fields[1:] {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			break
		}
		vals = append(vals, v)
	}
	if len(vals) < 4 {
		return healthUnavailableV
	}
	idle := vals[3]
	total := int64(0)
	for _, v := range vals {
		total += v
	}
	if total == 0 {
		return healthUnavailableV
	}
	return roundOne(100.0 * float64(total-idle) / float64(total))
}

func memoryPercent() float64 {
	if runtime.GOOS != "linux" {
		return healthUnavailableV
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return healthUnavailableV
	}
	defer func() { _ = f.Close() }()
	info := map[string]int64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.Fields(strings.TrimSpace(parts[1]))
		if len(valStr) == 0 {
			continue
		}
		v, err := strconv.ParseInt(valStr[0], 10, 64)
		if err == nil {
			info[key] = v
		}
	}
	total, ok1 := info["MemTotal"]
	available, ok2 := info["MemAvailable"]
	if !ok1 || !ok2 || total == 0 {
		return healthUnavailableV
	}
	return roundOne(100.0 * float64(total-available) / float64(total))
}

func roundOne(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

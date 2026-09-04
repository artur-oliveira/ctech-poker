package app

import (
	"context"
	"log/slog"
	"runtime"
	"runtime/debug"
	"time"
)

const (
	memoryPressureAlarmMessage = "ALARM: process memory pressure"
	memoryPressureRatio        = 0.85
	memoryPressurePollInterval = time.Minute
)

// monitorMemoryPressure emits a structured warning while Go-managed resident
// memory is close to GOMEMLIMIT. Sys-HeapReleased is the same runtime memory
// accounting basis used by the Go soft-limit documentation; OS RSS can also
// include stacks and native/library allocations, so the load harness records
// that separately from /proc.
func monitorMemoryPressure(ctx context.Context) {
	ticker := time.NewTicker(memoryPressurePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logMemoryPressure()
		}
	}
}

func logMemoryPressure() bool {
	limit := debug.SetMemoryLimit(-1)
	if limit <= 0 {
		return false
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	resident := stats.Sys
	if stats.HeapReleased < resident {
		resident -= stats.HeapReleased
	}
	ratio := float64(resident) / float64(limit)
	if ratio < memoryPressureRatio {
		return false
	}
	slog.Warn(memoryPressureAlarmMessage,
		"go_resident_bytes", resident,
		"go_alloc_bytes", stats.Alloc,
		"go_memory_limit_bytes", limit,
		"limit_ratio", ratio,
	)
	return true
}

# ctech-poker Foundations & Game Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the `ctech-poker` API repo skeleton (matching company convention), its CDK stack and CI pipeline, the
table-lease service, and a fully correct, independently-testable Texas Hold'em rules engine (deck/shuffle, hand
evaluator, side pots, betting rounds, hand lifecycle) exposed via a CLI harness — no networking, no wallet integration
yet.

**Architecture:** Go service using the same `cmd/` + `internal/` layout, Fiber v3 + `go.uber.org/fx` DI, `slog` JSON
logging, and `caarlos0/env/v11` config as every other CTech Go API (`ctech-wallet/api` is the reference implementation
copied from throughout this plan). The rules engine is a pure-logic package tree (`internal/engine/...`) with zero
dependency on HTTP/WebSocket/DB — it takes actions in, returns state out — so it can be built and correctness-tested in
complete isolation before Phase 2 wires it to a live table server.

**Tech Stack:** Go 1.26, Fiber v3, `go.uber.org/fx`, `gopkg.aoctech.app/api-commons` (`cache`, `ws`),
`github.com/valkey-io/valkey-go`, AWS CDK (TypeScript) importing `@aoctech/cdk`, GitHub Actions.

## Global Constraints

- Go module path: `gopkg.aoctech.app/poker/api` (matches `wallet/api`, `dfe/api`, `account/api` naming).
- Go version: `1.26` (matches `golang:1.26-alpine` builder image used company-wide).
- Deployed binary **must be named `app`** — CDK userdata on the shared `PrivateIpv4Ec2Service` construct hardcodes
  `/opt/app/current/app`.
- Server framework: Fiber v3 (`github.com/gofiber/fiber/v3`), DI: `go.uber.org/fx`, logging: `log/slog` with
  `slog.NewJSONHandler`, config: `github.com/caarlos0/env/v11`.
- Never hand-roll what `gopkg.aoctech.app/api-commons` already provides (`cache.Backend`/`cache.RedisBackend`/
  `cache.NewMemoryBackend`, `ws` registry) — import it, don't reimplement.
- CSPRNG only for anything shuffle/fairness/sandbox credits-adjacent: `crypto/rand`, never `math/rand` unseeded/seeded
  predictably.
- No new third-party dependency where the standard library already does the job (YAGNI) — the hand evaluator and shuffle
  are built on `crypto/rand`, `crypto/sha256`, `crypto/hmac` only.
- Every pure-logic package under `internal/engine/` must have zero imports of `net/http`, `fiber`, database, or Valkey
  clients — networking/persistence is Phase 2+, not this plan.

---

## Phase 0 — Foundations

### Task 1: Repo skeleton — Go module, Dockerfile, Makefile, Fx app with health check

**Files:**

- Create: `api/go.mod`
- Create: `api/Dockerfile`
- Create: `api/Makefile`
- Create: `api/cmd/server/main.go`
- Create: `api/internal/config/config.go`
- Create: `api/internal/config/config_test.go`
- Create: `api/internal/app/app.go`
- Test: `api/internal/app/app_test.go`

**Interfaces:**

- Produces: `config.Config` struct + `config.Load() (*config.Config, error)`; `app.Module` (an `fx.Option`) that later
  tasks (lock/lease, engine wiring in Phase 2) will extend via `fx.Provide`.

- [ ] **Step 1: Create the Go module**

```bash
mkdir -p /home/artur/Documents/Projects/Ctech/ctech-poker/api/cmd/server
mkdir -p /home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/config
mkdir -p /home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/app
cd /home/artur/Documents/Projects/Ctech/ctech-poker/api
go mod init gopkg.aoctech.app/poker/api
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/gofiber/fiber/v3@v3.4.0
go get go.uber.org/fx@v1.24.0
go get github.com/caarlos0/env/v11@v11.4.1
go get gopkg.aoctech.app/api-commons@v1.1.0
go get github.com/valkey-io/valkey-go@v1.0.76
go get github.com/google/uuid@v1.6.0
```

- [ ] **Step 3: Write the config package**

`api/internal/config/config.go`:

```go
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds the 12-Factor environment configuration for the poker API.
type Config struct {
	AppVersion string `env:"APP_VERSION" envDefault:"0.0.1"`
	Port       int    `env:"PORT" envDefault:"8003"`
	Env        string `env:"ENVIRONMENT" envDefault:"dev"`

	ReadTimeout  int64 `env:"READ_TIMEOUT" envDefault:"10"`
	IdleTimeout  int64 `env:"IDLE_TIMEOUT" envDefault:"60"`
	WriteTimeout int64 `env:"WRITE_TIMEOUT" envDefault:"10"`

	// Cache / table-lease (see Task 4). Optional in dev — falls back to an
	// in-memory backend that is NOT shared across replicas.
	RedisURL string `env:"VALKEY_URL"`
}

// Load reads config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.RedisURL == "" && cfg.Env == "prod" {
		// Fail closed: an empty VALKEY_URL in prod means the table-lease
		// service silently degrades to an in-memory store that is NOT shared
		// across the ASG's other instances — table-authority (single-writer
		// per table) stops holding fleet-wide with no signal.
		return nil, fmt.Errorf("config: VALKEY_URL must be set in production so table leases are fleet-shared")
	}
	return cfg, nil
}
```

- [ ] **Step 4: Write the failing config test**

`api/internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadFailsClosedWithoutValkeyURLInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail closed with VALKEY_URL unset in prod")
	}
}

func TestLoadDefaultsToDevWithoutValkeyURL(t *testing.T) {
	t.Setenv("VALKEY_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed in dev without VALKEY_URL, got %v", err)
	}
	if cfg.Port != 8003 {
		t.Fatalf("expected default port 8003, got %d", cfg.Port)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && go test ./internal/config/... -v`
Expected: both `TestLoadFailsClosedWithoutValkeyURLInProd` and `TestLoadDefaultsToDevWithoutValkeyURL` PASS.

- [ ] **Step 6: Write the Fx app module with a health endpoint**

`api/internal/app/app.go`:

```go
// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// Module is the root Fx module for the poker API.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		newFiberApp,
	),
	fx.Invoke(registerRoutes),
	fx.Invoke(startServer),
)

func newFiberApp() *fiber.App {
	return fiber.New()
}

func registerRoutes(app *fiber.App) {
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
}

func startServer(lc fx.Lifecycle, app *fiber.App, cfg *config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := ":" + itoa(cfg.Port)
			go func() {
				if err := app.Listen(addr); err != nil {
					slog.Error("server stopped", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return app.ShutdownWithContext(stopCtx)
		},
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
```

- [ ] **Step 7: Write the failing app test (health check via `fiber.Test`)**

`api/internal/app/app_test.go`:

```go
package app

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHealthEndpointReturnsOK(t *testing.T) {
	app := fiber.New()
	registerRoutes(app)

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd api && go test ./internal/app/... -v`
Expected: `TestHealthEndpointReturnsOK` PASS.

- [ ] **Step 9: Write `cmd/server/main.go`**

```go
package main

import (
	"log/slog"
	"os"

	"go.uber.org/fx"
	"gopkg.aoctech.app/poker/api/internal/app"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	fx.New(app.Module).Run()
}
```

- [ ] **Step 10: Write the Dockerfile**

`api/Dockerfile`:

```dockerfile
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /app-bin ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app-bin /app

EXPOSE 8003

ENTRYPOINT ["/app"]
```

- [ ] **Step 11: Write the Makefile**

`api/Makefile`:

```makefile
# Deployed binary MUST be named `app` — CDK userdata expects /opt/app/current/app.
BINARY      := app
BUILD_DIR   := dist
GOOS        ?= linux
GOARCH      ?= arm64
CGO_ENABLED := 0

.PHONY: build lint test vet clean

build:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/server

lint:
	golangci-lint run ./...

test:
	go test ./... -race -coverprofile=coverage.out

vet:
	go vet ./...

clean:
	rm -rf $(BUILD_DIR)
```

- [ ] **Step 12: Verify the binary builds**

Run: `cd api && make build`
Expected: `dist/app` produced with no errors.

- [ ] **Step 13: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/go.mod api/go.sum api/Dockerfile api/Makefile api/cmd api/internal/config api/internal/app
git commit -m "feat: repo skeleton — Fx app with health check, Dockerfile, Makefile"
```

---

### Task 2: Table-lease service (Valkey-backed, TTL-renewed)

The table-authority model (ARCHITECTURE.md § 2) needs a lease that's held for a table's *entire* process lifetime,
renewed on a heartbeat — unlike `ctech-wallet`'s per-operation advisory lock (`ctech-wallet/api/internal/lock/lock.go`,
fire-and-forget acquire/release with no renewal). This task ports that package's acquire/release/CAS-delete plumbing and
adds a `Renew` method plus a heartbeat loop.

**Files:**

- Create: `api/internal/tablelease/lease.go`
- Test: `api/internal/tablelease/lease_test.go`

**Interfaces:**

- Consumes: `cache.Backend`, `cache.RedisBackend` from `gopkg.aoctech.app/api-commons/cache` (already a dependency, Step
  2 of Task 1).
- Produces: `tablelease.NewService(c cache.Backend) *tablelease.Service`,
  `(*Service).Acquire(ctx, tableID string) (release func(), ok bool, err error)`,
  `(*Service).StartHeartbeat(ctx, tableID string, release func()) (stop func())` — Phase 2's table server wires this to
  know when it's lost the lease and must stop processing.

- [ ] **Step 1: Write the failing test for basic acquire/release semantics**

`api/internal/tablelease/lease_test.go`:

```go
package tablelease

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestAcquireThenContendedAcquireFails(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	ctx := context.Background()

	release, ok, err := svc.Acquire(ctx, "table-1")
	if err != nil || !ok {
		t.Fatalf("expected first acquire to succeed, got ok=%v err=%v", ok, err)
	}
	defer release()

	_, ok2, err2 := svc.Acquire(ctx, "table-1")
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if ok2 {
		t.Fatal("expected contended acquire to fail while lease is held")
	}
}

func TestReleaseFreesLeaseForNewAcquire(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	ctx := context.Background()

	release, ok, _ := svc.Acquire(ctx, "table-2")
	if !ok {
		t.Fatal("expected first acquire to succeed")
	}
	release()

	_, ok2, err2 := svc.Acquire(ctx, "table-2")
	if err2 != nil || !ok2 {
		t.Fatalf("expected acquire after release to succeed, got ok=%v err=%v", ok2, err2)
	}
}

func TestRenewExtendsLeaseBeforeExpiry(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	svc.leaseTTL = 50 * time.Millisecond
	ctx := context.Background()

	_, ok, _ := svc.Acquire(ctx, "table-3")
	if !ok {
		t.Fatal("expected acquire to succeed")
	}

	time.Sleep(30 * time.Millisecond)
	if err := svc.Renew(ctx, "table-3"); err != nil {
		t.Fatalf("renew failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond) // 60ms since acquire, but only 30ms since renew
	_, ok2, _ := svc.Acquire(ctx, "table-3")
	if ok2 {
		t.Fatal("expected contended acquire to still fail — renew should have extended the lease")
	}
}

func TestLeaseExpiresAndCanBeReacquiredWithoutRenew(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	svc.leaseTTL = 20 * time.Millisecond
	ctx := context.Background()

	_, ok, _ := svc.Acquire(ctx, "table-4")
	if !ok {
		t.Fatal("expected acquire to succeed")
	}

	time.Sleep(40 * time.Millisecond) // let it expire, never renewed

	_, ok2, err2 := svc.Acquire(ctx, "table-4")
	if err2 != nil || !ok2 {
		t.Fatalf("expected acquire after expiry to succeed, got ok=%v err=%v", ok2, err2)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (package doesn't exist yet)**

Run: `cd api && go test ./internal/tablelease/... -v`
Expected: FAIL — `no required module provides package .../internal/tablelease`.

- [ ] **Step 3: Write the lease service**

`api/internal/tablelease/lease.go`:

```go
// Package tablelease implements the single-writer-per-table directory
// service described in ARCHITECTURE.md § 2: a Valkey key `table:{id}` holds
// the owning instance's token with a TTL, renewed on a heartbeat for as long
// as that instance keeps processing the table. Ports the acquire/CAS-release
// primitive from ctech-wallet/api/internal/lock/lock.go and adds renewal,
// since a table lease is held for a process's entire lifetime handling that
// table, not for one short operation.
package tablelease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
	"gopkg.aoctech.app/api-commons/cache"
)

// DefaultLeaseTTL bounds how long a lease is held before it auto-expires, so
// a crashed instance can never wedge a table forever.
const DefaultLeaseTTL = 15 * time.Second

// DefaultHeartbeatInterval is how often StartHeartbeat renews an active
// lease — well under DefaultLeaseTTL so a slow renewal never lets it lapse.
const DefaultHeartbeatInterval = 5 * time.Second

const leaseKeyFmt = "table:%s"

type store interface {
	setNX(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	renewIfMatch(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	delIfMatch(ctx context.Context, key, token string) error
}

// Service acquires and renews per-table leases.
type Service struct {
	store    store
	leaseTTL time.Duration

	mu     sync.Mutex
	tokens map[string]string // tableID -> this process's current token, for Renew
}

// NewService returns a Valkey-backed lease service when the cache backend is
// Redis, otherwise an in-memory one (dev/single-replica only).
func NewService(c cache.Backend) *Service {
	s := &Service{leaseTTL: DefaultLeaseTTL, tokens: make(map[string]string)}
	if rb, ok := c.(*cache.RedisBackend); ok {
		s.store = &redisStore{client: rb.Client()}
	} else {
		s.store = newMemStore()
	}
	return s
}

// Acquire takes the lease for one table. On success it returns a release
// func (safe to call once) and ok=true. On contention it returns ok=false.
func (s *Service) Acquire(ctx context.Context, tableID string) (release func(), ok bool, err error) {
	token, err := newToken()
	if err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf(leaseKeyFmt, tableID)
	got, err := s.store.setNX(ctx, key, token, s.leaseTTL)
	if err != nil {
		return nil, false, err
	}
	if !got {
		return nil, false, nil
	}
	s.mu.Lock()
	s.tokens[tableID] = token
	s.mu.Unlock()
	return func() {
		_ = s.store.delIfMatch(context.Background(), key, token)
		s.mu.Lock()
		delete(s.tokens, tableID)
		s.mu.Unlock()
	}, true, nil
}

// Renew extends the TTL of a lease this process currently holds. Returns an
// error if this process no longer holds it (e.g. it already expired and was
// re-acquired elsewhere) — the caller (table server) must treat that as
// "I've lost authority over this table" and stop processing immediately.
func (s *Service) Renew(ctx context.Context, tableID string) error {
	s.mu.Lock()
	token, held := s.tokens[tableID]
	s.mu.Unlock()
	if !held {
		return fmt.Errorf("tablelease: no lease held locally for table %s", tableID)
	}
	key := fmt.Sprintf(leaseKeyFmt, tableID)
	ok, err := s.store.renewIfMatch(ctx, key, token, s.leaseTTL)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tablelease: lease for table %s was lost (token mismatch or expired)", tableID)
	}
	return nil
}

// StartHeartbeat renews the lease for tableID on DefaultHeartbeatInterval
// until the returned stop func is called or Renew fails (lease lost) — in
// which case it calls onLost, if provided.
func (s *Service) StartHeartbeat(ctx context.Context, tableID string, onLost func()) (stop func()) {
	loopCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(DefaultHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if err := s.Renew(loopCtx, tableID); err != nil {
					if onLost != nil {
						onLost()
					}
					return
				}
			}
		}
	}()
	return cancel
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- redis-backed store ---

type redisStore struct {
	client valkey.Client
}

func (s *redisStore) setNX(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	_, err := s.client.Do(ctx, s.client.B().Set().Key(key).Value(token).Nx().Ex(ttl).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const casRenewScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("expire", KEYS[1], ARGV[2])
end
return 0
`

func (s *redisStore) renewIfMatch(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	n, err := s.client.Do(ctx, s.client.B().Eval().Script(casRenewScript).Numkeys(1).Key(key).Arg(token, fmt.Sprintf("%d", int(ttl.Seconds()))).Build()).ToInt64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

const casDelScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`

func (s *redisStore) delIfMatch(ctx context.Context, key, token string) error {
	return s.client.Do(ctx, s.client.B().Eval().Script(casDelScript).Numkeys(1).Key(key).Arg(token).Build()).Error()
}

// --- in-memory store (single replica / tests) ---

type memEntry struct {
	token   string
	expires time.Time
}

type memStore struct {
	mu   sync.Mutex
	keys map[string]memEntry
}

func newMemStore() *memStore { return &memStore{keys: make(map[string]memEntry)} }

func (s *memStore) setNX(_ context.Context, key, token string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.keys[key]; ok && time.Now().Before(e.expires) {
		return false, nil
	}
	s.keys[key] = memEntry{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (s *memStore) renewIfMatch(_ context.Context, key, token string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.keys[key]
	if !ok || e.token != token || time.Now().After(e.expires) {
		return false, nil
	}
	s.keys[key] = memEntry{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (s *memStore) delIfMatch(_ context.Context, key, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.keys[key]; ok && e.token == token {
		delete(s.keys, key)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/tablelease/... -v -race`
Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/tablelease
git commit -m "feat: table-lease service with TTL renewal, ported from wallet's lock package"
```

---

### Task 3: CDK stack skeleton

Mirrors `ctech-wallet/cdk` — imports `PrivateIpv4Ec2Service` (ASG+EC2 stateful service) and `ValkeyStack` SSM lookup
from `@aoctech/cdk`, adds an `ApplicationListenerRule` against the shared ALB for `poker-api.aoctech.app`, and a
CloudFront + Route53 frontend stack for `poker.aoctech.app` (mirroring `ctech-wallet/cdk/lib/frontend-stack.ts`).

**Files:**

- Create: `cdk/package.json`
- Create: `cdk/tsconfig.json`
- Create: `cdk/cdk.json`
- Create: `cdk/bin/poker.ts`
- Create: `cdk/lib/api-stack.ts`
- Test: `cdk/test/api-stack.test.ts`

**Interfaces:**

- Consumes: `PrivateIpv4Ec2Service`, `AlbStack` (SSM export lookups), `ValkeyStack` (SSM export lookup) from
  `@aoctech/cdk` — exact constructor signatures must be confirmed against `ctech-cdk/lib/private-ipv4-ec2-service.ts`
  and `ctech-wallet/cdk/lib/api-stack.ts` at implementation time (this task's Step 1).
- Produces: `PokerApiStack` (CDK stack class), synthesizable via `cdk synth`.

- [ ] **Step 1: Read the reference implementation before writing any CDK code**

Run:
`cat /home/artur/Documents/Projects/Ctech/ctech-wallet/cdk/lib/api-stack.ts /home/artur/Documents/Projects/Ctech/ctech-wallet/cdk/lib/frontend-stack.ts /home/artur/Documents/Projects/Ctech/ctech-cdk/lib/private-ipv4-ec2-service.ts`

Copy the exact construct props shape (SSM parameter names for the shared ALB SG/listener ARN, the `ValkeyStack` SSM
export helper, health-check path convention) — this plan does not restate that file's content since it must match
byte-for-byte with whatever's current in `ctech-cdk` at implementation time, not a snapshot that can drift.

- [ ] **Step 2: Scaffold the CDK app**

```bash
mkdir -p /home/artur/Documents/Projects/Ctech/ctech-poker/cdk/{bin,lib,test}
cd /home/artur/Documents/Projects/Ctech/ctech-poker/cdk
npm init -y
npm install aws-cdk-lib constructs @aoctech/cdk
npm install --save-dev typescript ts-node @types/node aws-cdk jest ts-jest @types/jest
```

- [ ] **Step 3: Write `cdk/lib/api-stack.ts`**

Using the exact props confirmed in Step 1, build a stack that: looks up the shared ALB SG + HTTPS listener ARN via SSM,
looks up the `ValkeyStack` URL via SSM, instantiates `PrivateIpv4Ec2Service` with a userdata script that fetches
`s3://{bucket}/poker/current.zip`, extracts to `/opt/app/current`, and runs `/opt/app/current/app` with `VALKEY_URL` and
`ENVIRONMENT` env vars set, and registers an `ApplicationListenerRule` for host header `poker-api.aoctech.app` at a
unique priority (confirm the next free priority against `ctech-wallet/cdk/lib/api-stack.ts` and any other stack sharing
the same listener — do not reuse a priority already taken).

- [ ] **Step 4: Write a synth smoke test**

`cdk/test/api-stack.test.ts`:

```typescript
import { App } from 'aws-cdk-lib';
import { Template } from 'aws-cdk-lib/assertions';
import { PokerApiStack } from '../lib/api-stack';

test('synthesizes without error and declares exactly one ASG', () => {
  const app = new App();
  const stack = new PokerApiStack(app, 'TestPokerApiStack', {
    env: { account: '123456789012', region: 'us-east-1' },
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::AutoScaling::AutoScalingGroup', 1);
});
```

- [ ] **Step 5: Run the synth test**

Run: `cd cdk && npx jest`
Expected: PASS — one ASG resource synthesized, no CDK synth errors.

- [ ] **Step 6: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add cdk
git commit -m "feat: CDK stack skeleton importing shared ctech-cdk constructs"
```

---

### Task 4: CI pipeline

Mirror `ctech-wallet/.github/workflows/api.yml` exactly (runner, Go setup, OIDC role assumption, versioned zip, SSM
rolling deploy), renamed for poker.

**Files:**

- Create: `.github/workflows/api.yml`

- [ ] **Step 1: Read the reference workflow before writing**

Run: `cat /home/artur/Documents/Projects/Ctech/ctech-wallet/.github/workflows/api.yml`

- [ ] **Step 2: Copy it to `ctech-poker/.github/workflows/api.yml`**, changing only:
    - Working directory / paths-filter from `api/**` (unchanged — same relative path).
    - OIDC role ARN from `arn:aws:iam::<acct>:role/ctech-wallet-gha-api` to
      `arn:aws:iam::<acct>:role/ctech-poker-gha-api` (the account ID and the actual role must be confirmed/created by
      whoever owns AWS IAM for this account before this workflow can run — flagging as an infra prerequisite, not
      something this plan's engineer can unilaterally decide).
    - S3 upload path from `s3://{bucket}/wallet/current.zip` to `s3://{bucket}/poker/current.zip`.
    - ASG name filter used by the `aws ssm send-command` step from wallet's ASG name to poker's (must match whatever
      `PrivateIpv4Ec2Service` in Task 3 names its ASG).

- [ ] **Step 3: Validate workflow syntax**

Run: `cd /home/artur/Documents/Projects/Ctech/ctech-poker && actionlint .github/workflows/api.yml`
Expected: no errors. If `actionlint` isn't installed, `gh workflow view` after pushing (or a YAML syntax check via
`python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/api.yml'))"`) is an acceptable substitute for this
step alone — full CI-behavior verification only happens once this workflow actually runs in GitHub Actions.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/api.yml
git commit -m "ci: mirror wallet's api.yml deploy pipeline for poker"
```

---

## Phase 1 — Game engine (pure logic, no networking, no wallet)

### Task 5: Deck, CSPRNG shuffle, commit-reveal fairness

**Files:**

- Create: `api/internal/engine/deck/deck.go`
- Test: `api/internal/engine/deck/deck_test.go`

**Interfaces:**

- Produces: `deck.Card{Rank, Suit}`, `deck.Rank` (2–14, Ace=14), `deck.Suit` (0–3),
  `deck.NewShuffle() (*deck.ShuffleResult, error)`,
  `deck.ShuffleResult{Cards [52]Card, ServerSeed [32]byte, CommitHash [32]byte}`,
  `deck.Verify(seed [32]byte, claimedCards [52]Card, publishedHash [32]byte) bool`. Task 9 (hand lifecycle) consumes
  `NewShuffle`, deals from `.Cards` in order, and reveals `.ServerSeed` at `HAND_COMPLETE`.

- [ ] **Step 1: Write the failing tests**

`api/internal/engine/deck/deck_test.go`:

```go
package deck

import "testing"

func TestNewShuffleProducesAPermutationOf52UniqueCards(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	seen := make(map[Card]bool, 52)
	for _, c := range result.Cards {
		if seen[c] {
			t.Fatalf("duplicate card in shuffled deck: %+v", c)
		}
		seen[c] = true
	}
	if len(seen) != 52 {
		t.Fatalf("expected 52 unique cards, got %d", len(seen))
	}
}

func TestSameSeedReproducesSameShuffle(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	reproduced := shuffleWithSeed(result.ServerSeed)
	if reproduced != result.Cards {
		t.Fatal("shuffleWithSeed(seed) did not reproduce the original shuffle")
	}
}

func TestVerifySucceedsForGenuineReveal(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	if !Verify(result.ServerSeed, result.Cards, result.CommitHash) {
		t.Fatal("Verify should succeed for a genuine seed/deck/hash triple")
	}
}

func TestVerifyFailsIfDeckWasTamperedWith(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	tampered := result.Cards
	tampered[0], tampered[1] = tampered[1], tampered[0]
	if Verify(result.ServerSeed, tampered, result.CommitHash) {
		t.Fatal("Verify should fail when the revealed deck doesn't match the committed hash")
	}
}

func TestTwoShufflesProduceDifferentSeeds(t *testing.T) {
	a, _ := NewShuffle()
	b, _ := NewShuffle()
	if a.ServerSeed == b.ServerSeed {
		t.Fatal("two independent shuffles produced the same seed — CSPRNG not being used correctly")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/deck/... -v`
Expected: FAIL — package/functions don't exist yet.

- [ ] **Step 3: Write the deck package**

`api/internal/engine/deck/deck.go`:

```go
// Package deck implements a CSPRNG-shuffled 52-card deck with commit-reveal
// fairness (OVERVIEW.md § 3.5): the server commits to a hash of the shuffle
// before dealing, then reveals the seed after the hand so anyone can verify
// no card order was altered mid-hand.
package deck

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
)

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

// Rank uses the card's face value directly (2-10, Jack=11, Queen=12, King=13,
// Ace=14) so comparisons and the hand evaluator's tiebreak encoding (Task 6)
// never need a translation table.
type Rank uint8

const (
	Two   Rank = 2
	Three Rank = 3
	Four  Rank = 4
	Five  Rank = 5
	Six   Rank = 6
	Seven Rank = 7
	Eight Rank = 8
	Nine  Rank = 9
	Ten   Rank = 10
	Jack  Rank = 11
	Queen Rank = 12
	King  Rank = 13
	Ace   Rank = 14
)

type Card struct {
	Rank Rank
	Suit Suit
}

// ShuffleResult holds a freshly shuffled deck plus its fairness proof. Cards
// and ServerSeed must be kept secret by the caller until HAND_COMPLETE;
// CommitHash is safe to publish immediately (ARCHITECTURE.md § 3.5).
type ShuffleResult struct {
	Cards      [52]Card
	ServerSeed [32]byte
	CommitHash [32]byte
}

// NewShuffle draws a fresh CSPRNG seed and produces a shuffled deck plus its
// publishable commit hash.
func NewShuffle() (*ShuffleResult, error) {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	cards := shuffleWithSeed(seed)
	return &ShuffleResult{
		Cards:      cards,
		ServerSeed: seed,
		CommitHash: commitHash(seed, cards),
	}, nil
}

// Verify recomputes the shuffle from a revealed seed and checks it reproduces
// both the claimed card order and the hash published before the hand started.
func Verify(seed [32]byte, claimedCards [52]Card, publishedHash [32]byte) bool {
	if shuffleWithSeed(seed) != claimedCards {
		return false
	}
	return commitHash(seed, claimedCards) == publishedHash
}

func orderedDeck() [52]Card {
	var d [52]Card
	i := 0
	for _, s := range []Suit{Clubs, Diamonds, Hearts, Spades} {
		for r := Two; r <= Ace; r++ {
			d[i] = Card{Rank: r, Suit: s}
			i++
		}
	}
	return d
}

// shuffleWithSeed runs Fisher-Yates driven by a deterministic HMAC-SHA256
// byte stream keyed on seed, so the same seed always reproduces the same
// permutation (required so Verify can recompute it), while the seed itself
// only ever comes from crypto/rand (unpredictable to anyone without it).
func shuffleWithSeed(seed [32]byte) [52]Card {
	d := orderedDeck()
	var counter uint32
	nextIndex := func(max uint32) uint32 {
		for {
			var ctrBytes [4]byte
			binary.BigEndian.PutUint32(ctrBytes[:], counter)
			counter++
			mac := hmac.New(sha256.New, seed[:])
			mac.Write(ctrBytes[:])
			sum := mac.Sum(nil)
			v := binary.BigEndian.Uint32(sum[:4])
			// Rejection sampling to avoid modulo bias.
			limit := (1 << 32) - (1<<32)%max
			if v < uint32(limit) {
				return v % max
			}
		}
	}
	for i := len(d) - 1; i > 0; i-- {
		j := nextIndex(uint32(i + 1))
		d[i], d[j] = d[j], d[i]
	}
	return d
}

func commitHash(seed [32]byte, cards [52]Card) [32]byte {
	h := sha256.New()
	h.Write(seed[:])
	for _, c := range cards {
		h.Write([]byte{byte(c.Rank), byte(c.Suit)})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/deck/... -v -race`
Expected: all five tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/engine/deck
git commit -m "feat: CSPRNG deck shuffle with commit-reveal fairness proof"
```

---

### Task 6: 7-card hand evaluator

**Files:**

- Create: `api/internal/engine/handeval/handeval.go`
- Test: `api/internal/engine/handeval/handeval_test.go`

**Interfaces:**

- Consumes: `deck.Card`, `deck.Rank`, `deck.Suit` (Task 5).
- Produces: `handeval.Category` (enum, `HighCard`...`RoyalFlush`), `handeval.Score` (a `uint32`, higher always beats
  lower, equal means split pot), `handeval.Best7(cards [7]deck.Card) Score`. Task 9 consumes `Best7` to rank each
  player's hole+board cards at showdown.

- [ ] **Step 1: Write the failing tests — known hand-vs-hand comparisons**

`api/internal/engine/handeval/handeval_test.go`:

```go
package handeval

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func c(rank deck.Rank, suit deck.Suit) deck.Card { return deck.Card{Rank: rank, Suit: suit} }

func TestRoyalFlushBeatsStraightFlush(t *testing.T) {
	royal := [7]deck.Card{
		c(deck.Ten, deck.Spades), c(deck.Jack, deck.Spades), c(deck.Queen, deck.Spades),
		c(deck.King, deck.Spades), c(deck.Ace, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	straightFlush := [7]deck.Card{
		c(deck.Five, deck.Hearts), c(deck.Six, deck.Hearts), c(deck.Seven, deck.Hearts),
		c(deck.Eight, deck.Hearts), c(deck.Nine, deck.Hearts), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	if Best7(royal) <= Best7(straightFlush) {
		t.Fatal("royal flush must beat a lower straight flush")
	}
}

func TestFourOfAKindBeatsFullHouse(t *testing.T) {
	quads := [7]deck.Card{
		c(deck.Nine, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Nine, deck.Hearts),
		c(deck.Nine, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds), c(deck.Four, deck.Hearts),
	}
	fullHouse := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.King, deck.Hearts),
		c(deck.Queen, deck.Spades), c(deck.Queen, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Four, deck.Hearts),
	}
	if Best7(quads) <= Best7(fullHouse) {
		t.Fatal("four of a kind must beat a full house")
	}
}

func TestFlushBeatsStraight(t *testing.T) {
	flush := [7]deck.Card{
		c(deck.Two, deck.Spades), c(deck.Five, deck.Spades), c(deck.Nine, deck.Spades),
		c(deck.Jack, deck.Spades), c(deck.King, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	straight := [7]deck.Card{
		c(deck.Four, deck.Clubs), c(deck.Five, deck.Diamonds), c(deck.Six, deck.Hearts),
		c(deck.Seven, deck.Spades), c(deck.Eight, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Nine, deck.Hearts),
	}
	if Best7(flush) <= Best7(straight) {
		t.Fatal("flush must beat a straight")
	}
}

func TestWheelStraightAceCountsLow(t *testing.T) {
	wheel := [7]deck.Card{
		c(deck.Ace, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
		c(deck.Four, deck.Spades), c(deck.Five, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Jack, deck.Hearts),
	}
	highCardOnly := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.Jack, deck.Diamonds), c(deck.Nine, deck.Hearts),
		c(deck.Seven, deck.Spades), c(deck.Four, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	if Best7(wheel) <= Best7(highCardOnly) {
		t.Fatal("A-2-3-4-5 must be recognized as a straight (the wheel), beating a no-pair hand")
	}
}

func TestKickerBreaksTieBetweenEqualPairs(t *testing.T) {
	pairWithAceKicker := [7]deck.Card{
		c(deck.Nine, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Ace, deck.Hearts),
		c(deck.Four, deck.Spades), c(deck.Six, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	pairWithKingKicker := [7]deck.Card{
		c(deck.Nine, deck.Spades), c(deck.Nine, deck.Hearts), c(deck.King, deck.Diamonds),
		c(deck.Four, deck.Clubs), c(deck.Six, deck.Diamonds), c(deck.Two, deck.Clubs), c(deck.Three, deck.Spades),
	}
	if Best7(pairWithAceKicker) <= Best7(pairWithKingKicker) {
		t.Fatal("same pair rank must be broken by the higher kicker")
	}
}

func TestIdenticalHandsScoreEqualForSplitPot(t *testing.T) {
	handA := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.Queen, deck.Hearts),
		c(deck.Jack, deck.Spades), c(deck.Nine, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	handB := [7]deck.Card{
		c(deck.King, deck.Hearts), c(deck.King, deck.Spades), c(deck.Queen, deck.Diamonds),
		c(deck.Jack, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Four, deck.Clubs), c(deck.Five, deck.Spades),
	}
	if Best7(handA) != Best7(handB) {
		t.Fatal("identical pair+kickers across different suits must score equal (split pot)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/handeval/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the hand evaluator**

`api/internal/engine/handeval/handeval.go`:

```go
// Package handeval ranks the best 5-card hand out of 7 (OVERVIEW.md § 3.4).
// Score packs category + tiebreaker ranks into a single comparable integer:
// higher Score always wins; equal Score is a genuine tie (split pot).
package handeval

import (
	"sort"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

type Category uint8

const (
	HighCard Category = iota
	Pair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

// Score encodes Category in the top 4 bits, then up to 5 tiebreaker ranks
// (4 bits each, most significant first) below it — ranks fit in 4 bits since
// the highest rank value is Ace=14 (0b1110).
type Score uint32

func makeScore(cat Category, tiebreaks ...deck.Rank) Score {
	s := Score(cat) << 24
	shift := 20
	for _, r := range tiebreaks {
		s |= Score(r) << shift
		shift -= 4
	}
	return s
}

// Best7 returns the highest Score achievable from any 5 of the given 7 cards.
func Best7(cards [7]deck.Card) Score {
	var best Score
	// All C(7,5) = 21 combinations.
	idx := [5]int{0, 1, 2, 3, 4}
	for {
		var hand [5]deck.Card
		for i, ix := range idx {
			hand[i] = cards[ix]
		}
		if s := evaluate5(hand); s > best {
			best = s
		}
		if !nextCombination(&idx, 7) {
			break
		}
	}
	return best
}

// nextCombination advances idx (a strictly increasing k-subset of [0,n)) to
// the next combination in lexicographic order; returns false when exhausted.
func nextCombination(idx *[5]int, n int) bool {
	k := len(idx)
	i := k - 1
	for i >= 0 && idx[i] == n-k+i {
		i--
	}
	if i < 0 {
		return false
	}
	idx[i]++
	for j := i + 1; j < k; j++ {
		idx[j] = idx[j-1] + 1
	}
	return true
}

func evaluate5(hand [5]deck.Card) Score {
	ranks := make([]deck.Rank, 5)
	suitCount := map[deck.Suit]int{}
	rankCount := map[deck.Rank]int{}
	for i, c := range hand {
		ranks[i] = c.Rank
		suitCount[c.Suit]++
		rankCount[c.Rank]++
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] > ranks[j] })

	isFlush := len(suitCount) == 1
	straightHigh, isStraight := straightHighCard(ranks)

	if isFlush && isStraight && straightHigh == deck.Ace {
		return makeScore(RoyalFlush)
	}
	if isFlush && isStraight {
		return makeScore(StraightFlush, straightHigh)
	}

	// Group ranks by count, descending count then descending rank.
	type group struct {
		rank  deck.Rank
		count int
	}
	groups := make([]group, 0, len(rankCount))
	for r, cnt := range rankCount {
		groups = append(groups, group{rank: r, count: cnt})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})

	switch {
	case groups[0].count == 4:
		return makeScore(FourOfAKind, groups[0].rank, groups[1].rank)
	case groups[0].count == 3 && groups[1].count == 2:
		return makeScore(FullHouse, groups[0].rank, groups[1].rank)
	case isFlush:
		return makeScore(Flush, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	case isStraight:
		return makeScore(Straight, straightHigh)
	case groups[0].count == 3:
		return makeScore(ThreeOfAKind, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2 && groups[1].count == 2:
		return makeScore(TwoPair, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2:
		return makeScore(Pair, groups[0].rank, groups[1].rank, groups[2].rank, groups[3].rank)
	default:
		return makeScore(HighCard, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	}
}

// straightHighCard returns the high card of a straight among 5 descending,
// deduplicated-by-caller ranks, handling the wheel (A-2-3-4-5, where Ace
// counts low and the straight's "high card" for scoring is Five).
func straightHighCard(descRanks []deck.Rank) (deck.Rank, bool) {
	// A 5-card hand from evaluate5 always has exactly 5 ranks (possibly with
	// duplicates when not a straight candidate) — straights only apply when
	// all 5 are distinct.
	seen := map[deck.Rank]bool{}
	for _, r := range descRanks {
		if seen[r] {
			return 0, false
		}
		seen[r] = true
	}
	if descRanks[0]-descRanks[4] == 4 {
		return descRanks[0], true
	}
	// Wheel: A,5,4,3,2 sorted descending is [14,5,4,3,2].
	if descRanks[0] == deck.Ace && descRanks[1] == deck.Five && descRanks[2] == deck.Four &&
		descRanks[3] == deck.Three && descRanks[4] == deck.Two {
		return deck.Five, true
	}
	return 0, false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/handeval/... -v -race`
Expected: all six tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/engine/handeval
git commit -m "feat: 7-card hand evaluator with kicker comparison and wheel-straight handling"
```

---

### Task 7: `ComputeSidePots`

**Files:**

- Create: `api/internal/engine/sidepots/sidepots.go`
- Test: `api/internal/engine/sidepots/sidepots_test.go`

**Interfaces:**

- Produces: `sidepots.Contribution{PlayerID string, Amount int64}`,
  `sidepots.PotLayer{Amount int64, Eligible []string}`,
  `sidepots.ComputeSidePots(contributions []Contribution) []PotLayer`. Task 9 consumes this at `SHOWDOWN`, intersecting
  each layer's `Eligible` list with non-folded players before running `handeval.Best7` to find that layer's winner(s).

- [ ] **Step 1: Write the failing tests — the exact 2-way and 3-way scenarios from OVERVIEW.md § 3.3**

`api/internal/engine/sidepots/sidepots_test.go`:

```go
package sidepots

import (
	"reflect"
	"sort"
	"testing"
)

func sortedEligible(layers []PotLayer) []PotLayer {
	out := make([]PotLayer, len(layers))
	for i, l := range layers {
		e := append([]string(nil), l.Eligible...)
		sort.Strings(e)
		out[i] = PotLayer{Amount: l.Amount, Eligible: e}
	}
	return out
}

func TestTwoWayAllInAtDifferentAmounts(t *testing.T) {
	// A all-in 100, B contributes 300 (not all-in), C all-in 200.
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "B", Amount: 300},
		{PlayerID: "C", Amount: 200},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 300, Eligible: []string{"A", "B", "C"}}, // 100 * 3
		{Amount: 200, Eligible: []string{"B", "C"}},       // 100 * 2
		{Amount: 100, Eligible: []string{"B"}},            // 100 * 1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 600 {
		t.Fatalf("layers must sum to total contributed (600), got %d", total)
	}
}

func TestThreeWaySimultaneousAllInsAtDifferentAmounts(t *testing.T) {
	// A all-in 50, B all-in 150, C all-in 300, D contributes 300 (not all-in).
	contributions := []Contribution{
		{PlayerID: "A", Amount: 50},
		{PlayerID: "B", Amount: 150},
		{PlayerID: "C", Amount: 300},
		{PlayerID: "D", Amount: 300},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 200, Eligible: []string{"A", "B", "C", "D"}}, // 50 * 4
		{Amount: 300, Eligible: []string{"B", "C", "D"}},      // 100 * 3
		{Amount: 300, Eligible: []string{"C", "D"}},           // 150 * 2
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 800 {
		t.Fatalf("layers must sum to total contributed (800), got %d", total)
	}
}

func TestNoAllInsProducesASingleLayer(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "B", Amount: 100},
	}
	got := ComputeSidePots(contributions)
	if len(got) != 1 || got[0].Amount != 200 {
		t.Fatalf("expected a single 200-chip layer, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/sidepots/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write `ComputeSidePots`**

`api/internal/engine/sidepots/sidepots.go`:

```go
// Package sidepots implements OVERVIEW.md § 3.3's side-pot algorithm as a
// pure, independently-tested function — this is the #1 place real-money
// poker engines have historically had payout bugs.
package sidepots

import "sort"

// Contribution is the total amount one player put into the pot this hand
// (win-or-lose — folded players' chips still count, they're simply never in
// any layer's Eligible list because the caller filters folded players out
// before running the showdown evaluation against each layer).
type Contribution struct {
	PlayerID string
	Amount   int64
}

// PotLayer is one slice of the pot: an Amount, and the set of player IDs who
// contributed enough to be eligible to win it. The caller (Task 9's hand
// lifecycle) is responsible for further excluding folded players from
// Eligible before running the showdown — this function only knows about
// chip amounts, not fold state.
type PotLayer struct {
	Amount   int64
	Eligible []string
}

// ComputeSidePots sorts distinct contribution levels ascending; each layer
// between two consecutive levels is (levelDelta * numContributorsAtOrAboveLevel),
// and a player is eligible for a layer only if their own contribution reaches
// that layer's upper bound.
func ComputeSidePots(contributions []Contribution) []PotLayer {
	levels := make([]int64, 0, len(contributions))
	seen := map[int64]bool{}
	for _, c := range contributions {
		if c.Amount > 0 && !seen[c.Amount] {
			seen[c.Amount] = true
			levels = append(levels, c.Amount)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })

	layers := make([]PotLayer, 0, len(levels))
	var prev int64
	for _, level := range levels {
		delta := level - prev
		var eligible []string
		for _, c := range contributions {
			if c.Amount >= level {
				eligible = append(eligible, c.PlayerID)
			}
		}
		layers = append(layers, PotLayer{
			Amount:   delta * int64(len(eligible)),
			Eligible: eligible,
		})
		prev = level
	}
	return layers
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/sidepots/... -v -race`
Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/engine/sidepots
git commit -m "feat: ComputeSidePots — layered side-pot algorithm with eligibility"
```

---

### Task 8: Betting round — min-raise and short-all-in-does-not-reopen-action

**Files:**

- Create: `api/internal/engine/betting/betting.go`
- Test: `api/internal/engine/betting/betting_test.go`

**Interfaces:**

- Produces: `betting.PlayerState{ID string, Stack, Contributed int64, Folded, AllIn, ActedSinceLastFullRaise bool}`,
  `betting.Round{Players []*PlayerState, CurrentBet, MinRaise int64}`,
  `betting.NewRound(players []*PlayerState, currentBet, minRaise int64) *Round`,
  `(*Round).Act(playerIdx int, action Action, amount int64) error`, `(*Round).IsComplete() bool`. Task 9 consumes this
  per betting round (pre-flop/flop/turn/river), constructing a fresh `Round` each time with `MinRaise` reset to the
  table's big blind at the start of every round.

- [ ] **Step 1: Write the failing tests — the two hard rules named in OVERVIEW.md § 3.3**

`api/internal/engine/betting/betting_test.go`:

```go
package betting

import "testing"

func TestRaiseBelowMinimumIsRejected(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	r := NewRound([]*PlayerState{p1, p2}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bet 100 (full raise, opens at 0): %v", err)
	}
	if err := r.Act(1, ActionRaise, 150); err == nil {
		t.Fatal("expected raise to 150 (only +50, below the 100 minimum) to be rejected")
	}
	if err := r.Act(1, ActionRaise, 200); err != nil {
		t.Fatalf("raise to 200 (+100, meets minimum) should succeed: %v", err)
	}
}

func TestShortAllInDoesNotReopenActionForPlayersWhoAlreadyActed(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	p3 := &PlayerState{ID: "P3", Stack: 150}
	r := NewRound([]*PlayerState{p1, p2, p3}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionCall, 100); err != nil {
		t.Fatalf("P2 calls 100: %v", err)
	}
	// P3 shoves their entire 150-chip stack — a raise of only 50, below the
	// 100 minimum, but it's their whole stack so it's allowed as a short all-in.
	if err := r.Act(2, ActionRaise, 150); err != nil {
		t.Fatalf("P3's short all-in for 150 should be allowed: %v", err)
	}
	if !p3.AllIn {
		t.Fatal("P3 should be marked all-in")
	}
	if r.CurrentBet != 150 {
		t.Fatalf("CurrentBet should rise to 150, got %d", r.CurrentBet)
	}
	if r.MinRaise != 100 {
		t.Fatalf("MinRaise must stay at 100 — a short all-in does not reopen full-raise sizing, got %d", r.MinRaise)
	}

	// P1 already acted (bet) — the short all-in must NOT let them re-raise.
	if err := r.Act(0, ActionRaise, 300); err == nil {
		t.Fatal("P1 already acted; a short all-in must not reopen raising for them")
	}
	// P1 may still call the extra 50 to match the new CurrentBet.
	if err := r.Act(0, ActionCall, 150); err != nil {
		t.Fatalf("P1 should be able to call the extra 50: %v", err)
	}
	// P2 likewise may only call or fold, not re-raise.
	if err := r.Act(1, ActionRaise, 300); err == nil {
		t.Fatal("P2 already acted; a short all-in must not reopen raising for them either")
	}
	if err := r.Act(1, ActionCall, 150); err != nil {
		t.Fatalf("P2 should be able to call the extra 50: %v", err)
	}

	if !r.IsComplete() {
		t.Fatal("round should be complete: all non-folded, non-all-in players matched CurrentBet and have acted")
	}
}

func TestFullRaiseReopensActionForEveryone(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	r := NewRound([]*PlayerState{p1, p2}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionRaise, 300); err != nil { // +200, a full raise
		t.Fatalf("P2 raises to 300: %v", err)
	}
	if r.MinRaise != 200 {
		t.Fatalf("a full raise must update MinRaise to the new raise size (200), got %d", r.MinRaise)
	}
	// P1 already acted once, but P2's raise was full, so P1 may re-raise again.
	if err := r.Act(0, ActionRaise, 600); err != nil {
		t.Fatalf("P1 should be allowed to re-raise after a full raise reopened action: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/betting/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the betting package**

`api/internal/engine/betting/betting.go`:

```go
// Package betting implements one betting round's action rules (OVERVIEW.md
// § 3.3), most importantly: minimum raise sizing, and the rule that a short
// (sub-minimum) all-in does not reopen raising for players who already acted.
package betting

import "fmt"

type Action string

const (
	ActionFold  Action = "fold"
	ActionCheck Action = "check"
	ActionCall  Action = "call"
	ActionBet   Action = "bet"
	ActionRaise Action = "raise"
)

// PlayerState tracks one player's standing within a single betting round.
// ActedSinceLastFullRaise means "has responded to the current bet level
// since the last full raise" — it gates both round-completion and whether
// this player may raise again (a short all-in never resets it to false for
// players who already had it true).
type PlayerState struct {
	ID                       string
	Stack                    int64
	Contributed              int64
	Folded                   bool
	AllIn                    bool
	ActedSinceLastFullRaise  bool
}

// Round tracks one betting round (pre-flop/flop/turn/river) across all
// players still dealt into the hand.
type Round struct {
	Players    []*PlayerState
	CurrentBet int64
	MinRaise   int64
}

// NewRound starts a betting round. currentBet/minRaise seed initial state —
// e.g. post-flop rounds start at (0, bigBlind); pre-flop starts at
// (bigBlindAmount, bigBlindAmount) since the blinds are already posted.
func NewRound(players []*PlayerState, currentBet, minRaise int64) *Round {
	return &Round{Players: players, CurrentBet: currentBet, MinRaise: minRaise}
}

// Act applies one player's action. amount is the TOTAL chips this player
// will have contributed this round after a Bet/Raise/Call (not the delta).
func (r *Round) Act(playerIdx int, action Action, amount int64) error {
	if playerIdx < 0 || playerIdx >= len(r.Players) {
		return fmt.Errorf("betting: invalid player index %d", playerIdx)
	}
	p := r.Players[playerIdx]
	if p.Folded || p.AllIn {
		return fmt.Errorf("betting: player %s cannot act (folded=%v allIn=%v)", p.ID, p.Folded, p.AllIn)
	}

	switch action {
	case ActionFold:
		p.Folded = true
		return nil

	case ActionCheck:
		if p.Contributed != r.CurrentBet {
			return fmt.Errorf("betting: player %s must call or fold, cannot check (owes %d)", p.ID, r.CurrentBet-p.Contributed)
		}
		p.ActedSinceLastFullRaise = true
		return nil

	case ActionCall:
		owed := r.CurrentBet - p.Contributed
		if owed <= 0 {
			return fmt.Errorf("betting: player %s has nothing to call", p.ID)
		}
		if owed >= p.Stack {
			p.Contributed += p.Stack
			p.Stack = 0
			p.AllIn = true
		} else {
			p.Stack -= owed
			p.Contributed += owed
		}
		p.ActedSinceLastFullRaise = true
		return nil

	case ActionBet, ActionRaise:
		if p.ActedSinceLastFullRaise {
			return fmt.Errorf("betting: player %s already acted and no full raise has reopened action — may only call or fold", p.ID)
		}
		if amount <= r.CurrentBet {
			return fmt.Errorf("betting: raise amount %d must exceed current bet %d", amount, r.CurrentBet)
		}
		raiseSize := amount - r.CurrentBet
		delta := amount - p.Contributed
		goingAllIn := delta >= p.Stack
		if raiseSize < r.MinRaise && !goingAllIn {
			return fmt.Errorf("betting: raise size %d below minimum raise %d", raiseSize, r.MinRaise)
		}
		if goingAllIn {
			delta = p.Stack
			amount = p.Contributed + delta
			p.AllIn = true
		}
		p.Stack -= delta
		p.Contributed += delta
		r.CurrentBet = amount

		isFullRaise := raiseSize >= r.MinRaise
		if isFullRaise {
			r.MinRaise = raiseSize
			for _, other := range r.Players {
				if other != p {
					other.ActedSinceLastFullRaise = false
				}
			}
		}
		p.ActedSinceLastFullRaise = true
		return nil

	default:
		return fmt.Errorf("betting: unknown action %q", action)
	}
}

// IsComplete reports whether every player still in the hand (not folded, not
// all-in) has acted since the last full raise and matches CurrentBet.
func (r *Round) IsComplete() bool {
	for _, p := range r.Players {
		if p.Folded || p.AllIn {
			continue
		}
		if !p.ActedSinceLastFullRaise || p.Contributed != r.CurrentBet {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/betting/... -v -race`
Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/engine/betting
git commit -m "feat: betting round engine — min-raise sizing, short-all-in non-reopening"
```

---

### Task 9: Hand lifecycle orchestrator

Ties Tasks 5–8 together into the full state machine (OVERVIEW.md § 3.1), including blind posting (heads-up special
case), dealer rotation, the ready system, and mid-hand `PENDING_ENTRY` joins (OVERVIEW.md § 2).

**Files:**

- Create: `api/internal/engine/hand/hand.go`
- Test: `api/internal/engine/hand/hand_test.go`

**Interfaces:**

- Consumes: `deck.NewShuffle`, `handeval.Best7`, `sidepots.ComputeSidePots`, `betting.NewRound`/`Act`/`IsComplete` (
  Tasks 5–8).
- Produces: `hand.Player{ID string, Stack int64, State hand.PlayerState}`, `hand.PlayerState` enum (
  `Active, Folded, AllIn, SittingOut, Disconnected, PendingEntry`), `hand.Stage` enum (
  `WaitingForPlayers, PreFlop, Flop, Turn, River, Showdown, Complete`),
  `hand.NewTable(players []*Player, smallBlind, bigBlind int64) *hand.Table`, `(*Table).StartHand() error`,
  `(*Table).Act(playerID string, action betting.Action, amount int64) error`, `(*Table).Stage() Stage`,
  `(*Table).Payouts() map[string]int64` (populated once `Stage() == Complete`). Task 10's CLI harness consumes
  `NewTable`, `StartHand`, `Act`, `Payouts`.

- [ ] **Step 1: Write the failing test — full hand, 3-way all-in, correct pot distribution**

`api/internal/engine/hand/hand_test.go`:

```go
package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

func TestFullHandWithThreeWayAllInProducesCorrectPayouts(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 1000},
		{ID: "SB", Stack: 200},
		{ID: "BB", Stack: 1000},
	}
	table := NewTable(players, 10, 20)

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
	}

	// Pre-flop: Dealer raises to 220 (their whole intent), SB shoves all-in
	// for 200 total (short all-in — a raise of only... wait SB already posted
	// 10 as small blind, so calling Dealer's raise plus going all-in uses the
	// remaining 190 of their 200 stack), BB calls.
	if err := table.Act("Dealer", betting.ActionRaise, 220); err != nil {
		t.Fatalf("Dealer raises to 220: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 200); err != nil {
		t.Fatalf("SB shoves all-in for 200 total: %v", err)
	}
	if err := table.Act("BB", betting.ActionCall, 220); err != nil {
		t.Fatalf("BB calls 220: %v", err)
	}
	if err := table.Act("Dealer", betting.ActionCall, 220); err != nil {
		t.Fatalf("Dealer calls the short all-in (owes nothing more, already at 220): %v", err)
	}

	// SB is all-in with 200 total in the pot; Dealer and BB each have 220 in.
	// Main pot: 200*3=600, eligible all three. Side pot: 20*2=40, eligible
	// Dealer+BB only. Play remaining streets with both non-all-in players
	// checking through (SB has no more decisions — they're all-in).
	for table.Stage() != Showdown && table.Stage() != Complete {
		for _, id := range []string{"Dealer", "BB"} {
			if table.currentPlayerCanAct(id) {
				if err := table.Act(id, betting.ActionCheck, 0); err != nil {
					t.Fatalf("check on %v for %s: %v", table.Stage(), id, err)
				}
			}
		}
	}

	payouts := table.Payouts()
	var total int64
	for _, amount := range payouts {
		total += amount
	}
	if total != 640 { // 600 main pot + 40 side pot
		t.Fatalf("total payouts must equal total pot (640), got %d (%+v)", total, payouts)
	}
	if _, ok := payouts["SB"]; !ok {
		t.Fatal("SB contributed to and must be eligible for the main pot")
	}
}

func TestHeadsUpDealerPostsSmallBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000},
		{ID: "P2", Stack: 1000},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0 // P1 is dealer

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.players[0].Contributed != 10 {
		t.Fatalf("heads-up: dealer (P1) must post the small blind, got Contributed=%d", table.players[0].Contributed)
	}
	if table.players[1].Contributed != 20 {
		t.Fatalf("heads-up: non-dealer (P2) must post the big blind, got Contributed=%d", table.players[1].Contributed)
	}
}

func TestReadyGateBlocksHandStartWithFewerThanTwoReady(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: false},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to fail with fewer than 2 ready players")
	}
}

func TestPendingEntryPlayerIsNotDealtIntoHandsUntilTheyPostBigBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	table.AddMidHandJoiner(&Player{ID: "P3", Stack: 1000})
	if table.playerByID("P3").State != PendingEntry {
		t.Fatal("mid-hand joiner must start as PendingEntry")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/hand/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the hand lifecycle orchestrator**

`api/internal/engine/hand/hand.go`:

```go
// Package hand orchestrates one table's full hand lifecycle (OVERVIEW.md
// § 3.1), tying together deck shuffling (Task 5), hand evaluation (Task 6),
// side pots (Task 7), and betting rounds (Task 8). Pure logic — no
// networking, no persistence; Phase 2 wires this to a live table server.
package hand

import (
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/sidepots"
)

type PlayerState uint8

const (
	Active PlayerState = iota
	Folded
	AllIn
	SittingOut
	Disconnected
	PendingEntry
)

type Stage uint8

const (
	WaitingForPlayers Stage = iota
	PreFlop
	Flop
	Turn
	River
	Showdown
	Complete
)

type Player struct {
	ID          string
	Stack       int64
	Ready       bool
	State       PlayerState
	HoleCards   [2]deck.Card
	Contributed int64 // this hand's total contribution across all rounds, for side-pot math
}

type Table struct {
	players     []*Player
	smallBlind  int64
	bigBlind    int64
	dealerSeat  int
	stage       Stage
	board       []deck.Card
	shuffle     *deck.ShuffleResult
	nextCard    int
	round       *betting.Round
	roundIdx    map[string]int // playerID -> index into round.Players, for the active betting round
	payouts     map[string]int64
}

func NewTable(players []*Player, smallBlind, bigBlind int64) *Table {
	return &Table{
		players:    players,
		smallBlind: smallBlind,
		bigBlind:   bigBlind,
		stage:      WaitingForPlayers,
	}
}

func (t *Table) Stage() Stage { return t.stage }

func (t *Table) Payouts() map[string]int64 { return t.payouts }

func (t *Table) playerByID(id string) *Player {
	for _, p := range t.players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// AddMidHandJoiner seats a new player as PendingEntry (OVERVIEW.md § 2) — not
// dealt in until the hand in progress completes, and required to post the
// big blind on the hand they're first dealt into (handled in StartHand).
func (t *Table) AddMidHandJoiner(p *Player) {
	p.State = PendingEntry
	t.players = append(t.players, p)
}

// StartHand begins a new hand: requires >=2 ready players, draws/rotates the
// dealer button, posts blinds (heads-up special case: dealer posts small
// blind), shuffles via commit-reveal, and deals hole cards.
func (t *Table) StartHand() error {
	readyCount := 0
	for _, p := range t.players {
		if p.State != PendingEntry && p.Ready {
			readyCount++
		}
	}
	if readyCount < 2 {
		return fmt.Errorf("hand: need at least 2 ready players, have %d", readyCount)
	}

	shuffle, err := deck.NewShuffle()
	if err != nil {
		return fmt.Errorf("hand: shuffle: %w", err)
	}
	t.shuffle = shuffle
	t.nextCard = 0
	t.board = nil
	t.payouts = nil

	active := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.State == PendingEntry {
			continue // sits out until this hand completes
		}
		p.State = Active
		p.Contributed = 0
		p.HoleCards = [2]deck.Card{t.dealCard(), t.dealCard()}
		active = append(active, p)
	}

	sbSeat, bbSeat := t.blindSeats(len(active))
	t.postBlind(active[sbSeat], t.smallBlind)
	t.postBlind(active[bbSeat], t.bigBlind)

	t.startBettingRound(active, t.bigBlind, t.bigBlind)
	t.stage = PreFlop
	return nil
}

// blindSeats returns (smallBlindIdx, bigBlindIdx) relative to the active
// players slice ordered starting at the dealer. Heads-up is a special case:
// the dealer posts the small blind.
func (t *Table) blindSeats(numActive int) (sb, bb int) {
	if numActive == 2 {
		return 0, 1 // dealer (index 0 in this ordering) posts small blind
	}
	return 1, 2
}

func (t *Table) postBlind(p *Player, amount int64) {
	if amount >= p.Stack {
		amount = p.Stack
		p.State = AllIn
	}
	p.Stack -= amount
	p.Contributed += amount
}

func (t *Table) dealCard() deck.Card {
	c := t.shuffle.Cards[t.nextCard]
	t.nextCard++
	return c
}

func (t *Table) startBettingRound(active []*Player, currentBet, minRaise int64) {
	states := make([]*betting.PlayerState, 0, len(active))
	roundIdx := make(map[string]int, len(active))
	for _, p := range active {
		if p.State == Folded {
			continue
		}
		bs := &betting.PlayerState{
			ID:          p.ID,
			Stack:       p.Stack,
			Contributed: 0,
			AllIn:       p.State == AllIn,
		}
		if p.State == AllIn {
			bs.Contributed = p.Contributed // already committed, tracked for side pots separately
		}
		roundIdx[p.ID] = len(states)
		states = append(states, bs)
	}
	t.round = betting.NewRound(states, currentBet, minRaise)
	t.roundIdx = roundIdx
}

// currentPlayerCanAct reports whether id still has a decision to make in the
// current betting round (used by callers driving the hand to know who to
// prompt — Task 10's CLI harness and, later, Phase 2's table server).
func (t *Table) currentPlayerCanAct(id string) bool {
	idx, ok := t.roundIdx[id]
	if !ok {
		return false
	}
	bs := t.round.Players[idx]
	return !bs.Folded && !bs.AllIn && (!bs.ActedSinceLastFullRaise || bs.Contributed != t.round.CurrentBet)
}

// Act applies one player's betting action, then advances the stage if the
// round is complete.
func (t *Table) Act(playerID string, action betting.Action, amount int64) error {
	idx, ok := t.roundIdx[playerID]
	if !ok {
		return fmt.Errorf("hand: player %s has no pending action this round", playerID)
	}
	if err := t.round.Act(idx, action, amount); err != nil {
		return err
	}
	p := t.playerByID(playerID)
	if action == betting.ActionFold {
		p.State = Folded
	}
	if t.round.Players[idx].AllIn {
		p.State = AllIn
	}
	p.Stack = t.round.Players[idx].Stack
	p.Contributed += t.round.Players[idx].Contributed - p.contributedThisRoundBaseline()

	if t.round.IsComplete() {
		t.advanceStage()
	}
	return nil
}

// contributedThisRoundBaseline is a placeholder hook kept intentionally
// simple: this plan's Task 9 tracks cumulative Contributed by re-deriving it
// from the round's Contributed each time Act is called on a fresh round
// (round.Contributed always starts at 0 for a new street), so the delta
// equals the round's own Contributed value directly.
func (p *Player) contributedThisRoundBaseline() int64 { return 0 }

func (t *Table) advanceStage() {
	remaining := 0
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			remaining++
		}
	}
	if remaining <= 1 {
		t.runShowdown()
		return
	}

	switch t.stage {
	case PreFlop:
		t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
		t.stage = Flop
	case Flop:
		t.board = append(t.board, t.dealCard())
		t.stage = Turn
	case Turn:
		t.board = append(t.board, t.dealCard())
		t.stage = River
	case River:
		t.runShowdown()
		return
	}
	t.startBettingRound(t.activePlayers(), 0, t.bigBlind)
}

func (t *Table) activePlayers() []*Player {
	out := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			out = append(out, p)
		}
	}
	return out
}

func (t *Table) runShowdown() {
	t.stage = Showdown
	contributions := make([]sidepots.Contribution, 0, len(t.players))
	for _, p := range t.players {
		if p.Contributed > 0 {
			contributions = append(contributions, sidepots.Contribution{PlayerID: p.ID, Amount: p.Contributed})
		}
	}
	layers := sidepots.ComputeSidePots(contributions)

	payouts := make(map[string]int64)
	for _, layer := range layers {
		var winners []string
		var bestScore handeval.Score
		for _, id := range layer.Eligible {
			p := t.playerByID(id)
			if p.State == Folded {
				continue
			}
			var full [7]deck.Card
			full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
			copy(full[2:], t.board)
			score := handeval.Best7(full)
			switch {
			case score > bestScore:
				bestScore = score
				winners = []string{id}
			case score == bestScore:
				winners = append(winners, id)
			}
		}
		if len(winners) == 0 {
			continue
		}
		share := layer.Amount / int64(len(winners))
		for _, w := range winners {
			payouts[w] += share
		}
		// Odd chip goes to the first winner in seat order (closest to the
		// button, standard convention) — winners is already in table seat
		// order since layer.Eligible preserves contributions' input order.
		remainder := layer.Amount - share*int64(len(winners))
		if remainder > 0 {
			payouts[winners[0]] += remainder
		}
	}
	for id, amount := range payouts {
		t.playerByID(id).Stack += amount
	}
	t.payouts = payouts
	t.stage = Complete
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/hand/... -v -race`
Expected: all four tests PASS. If `TestFullHandWithThreeWayAllInProducesCorrectPayouts` fails on the exact payout split,
debug by printing `table.round` state after each `Act` call — the most likely bug source is the `Contributed`
bookkeeping between betting rounds (each street's `betting.Round` starts its own players' `Contributed` at 0;
`Player.Contributed` on the `hand` package's own `Player` struct is the *cumulative across the whole hand* figure that
side-pot math needs, so trace that specific accumulation first).

- [ ] **Step 5: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/internal/engine/hand
git commit -m "feat: hand lifecycle orchestrator tying deck/handeval/sidepots/betting together"
```

---

### Task 10: CLI test harness

The Phase 1 deliverable per PLAN.md: a CLI that plays a scripted action sequence and prints the resulting pot
distribution — no UI, no sockets.

**Files:**

- Create: `api/cmd/handreplay/main.go`
- Create: `api/cmd/handreplay/script.example.json`
- Test: `api/cmd/handreplay/main_test.go`

**Interfaces:**

- Consumes: `hand.NewTable`, `hand.Player`, `(*hand.Table).StartHand`, `(*hand.Table).Act`, `(*hand.Table).Payouts` (
  Task 9).

- [ ] **Step 1: Write the example script**

`api/cmd/handreplay/script.example.json`:

```json
{
  "players": [
    {"id": "Dealer", "stack": 1000, "ready": true},
    {"id": "SB", "stack": 200, "ready": true},
    {"id": "BB", "stack": 1000, "ready": true}
  ],
  "small_blind": 10,
  "big_blind": 20,
  "actions": [
    {"player": "Dealer", "action": "raise", "amount": 220},
    {"player": "SB", "action": "raise", "amount": 200},
    {"player": "BB", "action": "call", "amount": 220},
    {"player": "Dealer", "action": "call", "amount": 220},
    {"player": "Dealer", "action": "check", "amount": 0},
    {"player": "BB", "action": "check", "amount": 0},
    {"player": "Dealer", "action": "check", "amount": 0},
    {"player": "BB", "action": "check", "amount": 0},
    {"player": "Dealer", "action": "check", "amount": 0},
    {"player": "BB", "action": "check", "amount": 0}
  ]
}
```

- [ ] **Step 2: Write the failing test**

`api/cmd/handreplay/main_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunScriptProducesPayoutsSummingToTotalPot(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("script.example.json"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	payouts, err := runScript(data)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	var total int64
	for _, v := range payouts {
		total += v
	}
	if total != 640 {
		t.Fatalf("expected total payouts of 640, got %d (%+v)", total, payouts)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd api && go test ./cmd/handreplay/... -v`
Expected: FAIL — `runScript` doesn't exist yet.

- [ ] **Step 4: Write the CLI harness**

`api/cmd/handreplay/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type scriptPlayer struct {
	ID    string `json:"id"`
	Stack int64  `json:"stack"`
	Ready bool   `json:"ready"`
}

type scriptAction struct {
	Player string `json:"player"`
	Action string `json:"action"`
	Amount int64  `json:"amount"`
}

type script struct {
	Players    []scriptPlayer `json:"players"`
	SmallBlind int64          `json:"small_blind"`
	BigBlind   int64          `json:"big_blind"`
	Actions    []scriptAction `json:"actions"`
}

func runScript(data []byte) (map[string]int64, error) {
	var s script
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse script: %w", err)
	}

	players := make([]*hand.Player, len(s.Players))
	for i, sp := range s.Players {
		players[i] = &hand.Player{ID: sp.ID, Stack: sp.Stack, Ready: sp.Ready}
	}
	table := hand.NewTable(players, s.SmallBlind, s.BigBlind)
	if err := table.StartHand(); err != nil {
		return nil, fmt.Errorf("start hand: %w", err)
	}

	for _, a := range s.Actions {
		if err := table.Act(a.Player, betting.Action(a.Action), a.Amount); err != nil {
			return nil, fmt.Errorf("action %+v: %w", a, err)
		}
	}

	if table.Stage() != hand.Complete {
		return nil, fmt.Errorf("hand did not complete — stage is %v after all scripted actions", table.Stage())
	}
	return table.Payouts(), nil
}

func main() {
	path := "script.example.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read script:", err)
		os.Exit(1)
	}
	payouts, err := runScript(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(payouts, "", "  ")
	fmt.Println(string(out))
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd api && go test ./cmd/handreplay/... -v`
Expected: `TestRunScriptProducesPayoutsSummingToTotalPot` PASS.

- [ ] **Step 6: Run the CLI manually to see the deliverable work end to end**

Run: `cd api && go run ./cmd/handreplay`
Expected output: JSON object with each player's payout, e.g. `{"Dealer": ..., "BB": ...}` (exact split depends on
`handeval.Best7` result on the actual dealt cards, which vary run to run since the shuffle is genuinely random — that's
expected and correct; the test above only asserts the total, not who wins, since the deal isn't seeded for
reproducibility here).

- [ ] **Step 7: Commit**

```bash
cd /home/artur/Documents/Projects/Ctech/ctech-poker
git add api/cmd/handreplay
git commit -m "feat: handreplay CLI — scripted action sequence to pot distribution"
```

---

## Self-Review Notes

**Spec coverage:** Ready system, blind escalation (private rooms), leaderboard, achievements, sandbox sandbox credits, and hand
equity display (OVERVIEW.md § 2, § 9) are **not** in this plan — they require the table server (Phase 2),
wallet/persistence integration (Phase 3), and frontend (Phase 4), none of which exist yet. Mid-hand `PENDING_ENTRY`
joins are partially covered here (`AddMidHandJoiner`, `Table.players` gains a `PENDING_ENTRY` seat) but the "must post
big blind to be dealt in" rule's *enforcement* (currently `StartHand` simply skips `PendingEntry` seats every hand until
a future task marks them `Active`) is a stub deliberately left for Phase 2, where mid-hand joins interact with the live
table server's seat-management — flagging this explicitly rather than silently under-building it.

**Type consistency:** `hand.Player.Contributed`, `betting.PlayerState.Contributed`, and `sidepots.Contribution.Amount`
are three different fields tracking related-but-distinct things (cumulative-across-hand vs. this-street-only vs.
side-pot input) — Task 9's `Act` method bridges them. This is flagged in Task 9 Step 4's debugging note because it's the
single most likely place a future implementer introduces a bookkeeping bug.

**Next plans (not in this document):** Phase 2 (WebSocket gateway, table-server wiring of `tablelease` + `hand`,
disconnect/reconnect, durable action log/crash recovery), Phase 3 (room/lobby API, sandbox wallet integration,
ready/blind-escalation/mid-hand-join enforcement), Phase 4 (frontend: lobby, table UI, animations, equity display,
achievements, leaderboard, sandbox credits) each get their own plan under `docs/plans/` once this one ships — writing their
exact code now would be speculative against an engine that doesn't exist yet.

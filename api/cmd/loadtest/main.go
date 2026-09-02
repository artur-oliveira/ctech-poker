// Command loadtest drives synthetic WebSocket clients against a running poker
// API (local or a deployed prod-like env) to validate behaviour at a target
// concurrency profile before wider release (GitHub issue #123).
//
// It is a laptop / single-throwaway-instance tool: stdlib + existing repo deps
// (fasthttp/websocket, the generated protobuf types) only, no infra of its own.
//
// Each synthetic player:
//   - (optionally) buys in and readies over HTTP,
//   - opens a table WebSocket, sends the auth frame,
//   - plays hands with a weighted fold/check/call bot,
//   - periodically disconnects and reconnects (churn), and
//   - on socket loss, reconnects with backoff for the rest of the run.
//
// It reports p50/p95/p99 action-ack latency, hands completed, errors by code,
// and WS reconnects. See docs/plans/2026-09-02-load-soak-test-harness.md.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	fws "github.com/fasthttp/websocket"
	goproto "google.golang.org/protobuf/proto"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
)

type config struct {
	baseURL      string
	wsBaseURL    string
	tableIDs     []string
	tokens       []string
	playersTable int
	buyInAmount  int64
	autoBuyIn    bool
	autoReady    bool
	ramp         time.Duration
	duration     time.Duration
	churnEvery   time.Duration
	thinkMin     time.Duration
	thinkMax     time.Duration
	wFold        int
	wCheckCall   int
	insecureTLS  bool
	reportEvery  time.Duration
}

func main() {
	var (
		baseURL     = flag.String("url", "http://localhost:8003", "HTTP base URL of the poker API (scheme+host, no path)")
		wsURL       = flag.String("ws-url", "", "WebSocket base URL (default: derived from -url, http->ws)")
		tableIDsCSV = flag.String("table-ids", "", "comma-separated table/room IDs to drive (required)")
		tokenFile   = flag.String("token-file", "", "file with one player access token (JWT) per line")
		tokensCSV   = flag.String("tokens", "", "comma-separated player access tokens (alternative to -token-file)")
		players     = flag.Int("players-per-table", 4, "synthetic players seated per table")
		buyIn       = flag.Int64("buyin-amount", 1000, "buy-in amount per player (chips) for -auto-buyin")
		autoBuyIn   = flag.Bool("auto-buyin", false, "POST /v1.0/rooms/:id/join for each player before connecting")
		autoReady   = flag.Bool("auto-ready", true, "send a ready command after connecting")
		ramp        = flag.Duration("ramp", 10*time.Second, "spread client startup over this window")
		duration    = flag.Duration("duration", 60*time.Second, "total run duration (use hours for a soak)")
		churnEvery  = flag.Duration("churn-every", 0, "if >0, each client disconnects+reconnects on this interval (jittered)")
		thinkMin    = flag.Duration("think-min", 150*time.Millisecond, "minimum bot decision delay")
		thinkMax    = flag.Duration("think-max", 900*time.Millisecond, "maximum bot decision delay")
		wFold       = flag.Int("weight-fold", 1, "relative bot weight for folding when folding is legal")
		wCheckCall  = flag.Int("weight-check-call", 4, "relative bot weight for check/call")
		reportEvery = flag.Duration("report-every", 10*time.Second, "progress report interval")
	)
	flag.Parse()

	if *tableIDsCSV == "" {
		fmt.Fprintln(os.Stderr, "loadtest: -table-ids is required")
		os.Exit(2)
	}
	tokens, err := loadTokens(*tokenFile, *tokensCSV)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadtest: %v\n", err)
		os.Exit(2)
	}
	cfg := config{
		baseURL:      strings.TrimRight(*baseURL, "/"),
		wsBaseURL:    *wsURL,
		tableIDs:     splitCSV(*tableIDsCSV),
		tokens:       tokens,
		playersTable: *players,
		buyInAmount:  *buyIn,
		autoBuyIn:    *autoBuyIn,
		autoReady:    *autoReady,
		ramp:         *ramp,
		duration:     *duration,
		churnEvery:   *churnEvery,
		thinkMin:     *thinkMin,
		thinkMax:     *thinkMax,
		wFold:        *wFold,
		wCheckCall:   *wCheckCall,
		reportEvery:  *reportEvery,
	}
	if cfg.wsBaseURL == "" {
		cfg.wsBaseURL = strings.Replace(strings.Replace(cfg.baseURL, "https://", "wss://", 1), "http://", "ws://", 1)
	}

	need := len(cfg.tableIDs) * cfg.playersTable
	if len(cfg.tokens) < need {
		fmt.Fprintf(os.Stderr, "loadtest: need %d tokens (%d tables x %d players), have %d\n",
			need, len(cfg.tableIDs), cfg.playersTable, len(cfg.tokens))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	m := newMetrics()
	fmt.Printf("loadtest: %d tables x %d players = %d clients, duration %s, ramp %s\n",
		len(cfg.tableIDs), cfg.playersTable, need, cfg.duration, cfg.ramp)

	var wg sync.WaitGroup
	tokenIdx := 0
	rampGap := time.Duration(0)
	if need > 0 {
		rampGap = cfg.ramp / time.Duration(need)
	}
	for _, tableID := range cfg.tableIDs {
		for p := 0; p < cfg.playersTable; p++ {
			cl := &client{cfg: cfg, tableID: tableID, token: cfg.tokens[tokenIdx], m: m}
			tokenIdx++
			delay := time.Duration(tokenIdx) * rampGap
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-time.After(delay):
				case <-runCtx.Done():
					return
				}
				cl.run(runCtx)
			}()
		}
	}

	reportDone := make(chan struct{})
	go func() {
		t := time.NewTicker(cfg.reportEvery)
		defer t.Stop()
		start := time.Now()
		for {
			select {
			case <-t.C:
				m.printProgress(time.Since(start))
			case <-runCtx.Done():
				close(reportDone)
				return
			}
		}
	}()

	wg.Wait()
	<-reportDone
	m.printFinal(cfg)
	if m.failed() {
		os.Exit(1)
	}
}

// ---- token loading -------------------------------------------------------

func loadTokens(file, csv string) ([]string, error) {
	if csv != "" {
		return splitCSV(csv), nil
	}
	if file == "" {
		return nil, fmt.Errorf("one of -token-file or -tokens is required")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- metrics -----------------------------------------------------------

type metrics struct {
	mu            sync.Mutex
	ackLatencies  []time.Duration
	handsComplete map[string]struct{}
	errorsByCode  map[string]int64
	actionsSent   atomic.Int64
	acksRecv      atomic.Int64
	reconnects    atomic.Int64
	connectFail   atomic.Int64
	buyInFail     atomic.Int64
}

func newMetrics() *metrics {
	return &metrics{handsComplete: map[string]struct{}{}, errorsByCode: map[string]int64{}}
}

func (m *metrics) recordAck(d time.Duration) {
	m.acksRecv.Add(1)
	m.mu.Lock()
	m.ackLatencies = append(m.ackLatencies, d)
	m.mu.Unlock()
}

func (m *metrics) recordError(code string) {
	if code == "" {
		code = "unknown"
	}
	m.mu.Lock()
	m.errorsByCode[code]++
	m.mu.Unlock()
}

func (m *metrics) recordHandComplete(handID string) {
	if handID == "" {
		return
	}
	m.mu.Lock()
	m.handsComplete[handID] = struct{}{}
	m.mu.Unlock()
}

func (m *metrics) snapshotLatencies() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]time.Duration, len(m.ackLatencies))
	copy(out, m.ackLatencies)
	return out
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p / 100 * float64(len(sorted)-1))
	return sorted[idx]
}

func (m *metrics) printProgress(elapsed time.Duration) {
	lat := m.snapshotLatencies()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	m.mu.Lock()
	hands := len(m.handsComplete)
	m.mu.Unlock()
	fmt.Printf("[%6.0fs] actions=%d acks=%d hands=%d reconnects=%d p50=%s p95=%s p99=%s\n",
		elapsed.Seconds(), m.actionsSent.Load(), m.acksRecv.Load(), hands, m.reconnects.Load(),
		percentile(lat, 50), percentile(lat, 95), percentile(lat, 99))
}

// failed applies the pass/fail thresholds from the runbook.
func (m *metrics) failed() bool {
	lat := m.snapshotLatencies()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	var errs int64
	m.mu.Lock()
	for _, c := range m.errorsByCode {
		errs += c
	}
	m.mu.Unlock()
	total := m.actionsSent.Load()
	errRate := 0.0
	if total > 0 {
		errRate = float64(errs) / float64(total)
	}
	// Thresholds (see runbook): p99 action-ack < 750ms, error rate < 1%.
	return percentile(lat, 99) > 750*time.Millisecond || errRate > 0.01 || m.acksRecv.Load() == 0
}

func (m *metrics) printFinal(cfg config) {
	lat := m.snapshotLatencies()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Println("\n==================== load test results ====================")
	fmt.Printf("tables:              %d\n", len(cfg.tableIDs))
	fmt.Printf("players per table:   %d\n", cfg.playersTable)
	fmt.Printf("actions sent:        %d\n", m.actionsSent.Load())
	fmt.Printf("action acks:         %d\n", m.acksRecv.Load())
	fmt.Printf("hands completed:     %d\n", len(m.handsComplete))
	fmt.Printf("ws reconnects:       %d\n", m.reconnects.Load())
	fmt.Printf("connect failures:    %d\n", m.connectFail.Load())
	fmt.Printf("buy-in failures:     %d\n", m.buyInFail.Load())
	if len(lat) > 0 {
		fmt.Printf("action-ack latency:  p50=%s  p95=%s  p99=%s  max=%s\n",
			percentile(lat, 50), percentile(lat, 95), percentile(lat, 99), lat[len(lat)-1])
	}
	var errs int64
	for _, c := range m.errorsByCode {
		errs += c
	}
	fmt.Printf("errors:              %d total\n", errs)
	codes := make([]string, 0, len(m.errorsByCode))
	for c := range m.errorsByCode {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		fmt.Printf("  %-24s %d\n", c, m.errorsByCode[c])
	}
	errRate := 0.0
	if t := m.actionsSent.Load(); t > 0 {
		errRate = float64(errs) / float64(t)
	}
	fmt.Printf("error rate:          %.3f%%\n", errRate*100)
	fmt.Println("==========================================================")
	if m.failedLocked() {
		fmt.Println("RESULT: FAIL (see thresholds in the runbook)")
	} else {
		fmt.Println("RESULT: PASS")
	}
}

func (m *metrics) failedLocked() bool {
	lat := make([]time.Duration, len(m.ackLatencies))
	copy(lat, m.ackLatencies)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	var errs int64
	for _, c := range m.errorsByCode {
		errs += c
	}
	errRate := 0.0
	if t := m.actionsSent.Load(); t > 0 {
		errRate = float64(errs) / float64(t)
	}
	return percentile(lat, 99) > 750*time.Millisecond || errRate > 0.01 || m.acksRecv.Load() == 0
}

// ---- synthetic client -------------------------------------------------

type client struct {
	cfg     config
	tableID string
	token   string
	m       *metrics
	rng     *rand.Rand

	playerID string // learned from the first "connected"/"state" frame is not exposed; kept for churn logs
}

func (c *client) run(ctx context.Context) {
	c.rng = rand.New(rand.NewSource(time.Now().UnixNano() ^ int64(len(c.token))))

	if c.cfg.autoBuyIn {
		if err := c.httpBuyIn(ctx); err != nil {
			c.m.buyInFail.Add(1)
		}
	}

	backoff := time.Second
	for ctx.Err() == nil {
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.m.reconnects.Add(1)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < 15*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		c.m.reconnects.Add(1)
	}
}

func (c *client) httpBuyIn(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{
		"amount":          c.cfg.buyInAmount,
		"idempotency_key": fmt.Sprintf("loadtest-%s-%d", c.tableID, time.Now().UnixNano()),
	})
	url := fmt.Sprintf("%s/v1.0/rooms/%s/join", c.cfg.baseURL, c.tableID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("buyin http %d", resp.StatusCode)
	}
	return nil
}

// session opens one WebSocket and plays until the socket dies, the context
// ends, or (when churn is configured) the churn timer fires.
func (c *client) session(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1.0/tables/%s/ws", c.cfg.wsBaseURL, c.tableID)
	dialer := *fws.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		c.m.connectFail.Add(1)
		return err
	}
	defer conn.Close()

	// Auth frame: protobuf ClientMessage with the token.
	authFrame, _ := goproto.Marshal(&pokerproto.ClientMessage{Type: "auth", Token: c.token})
	if err := conn.WriteMessage(fws.BinaryMessage, authFrame); err != nil {
		return err
	}

	var writeMu sync.Mutex
	send := func(m *pokerproto.ClientMessage) error {
		data, err := goproto.Marshal(m)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteMessage(fws.BinaryMessage, data)
	}

	sessCtx, sessCancel := context.WithCancel(ctx)
	defer sessCancel()
	if c.cfg.churnEvery > 0 {
		jitter := time.Duration(c.rng.Int63n(int64(c.cfg.churnEvery)))
		go func() {
			select {
			case <-time.After(c.cfg.churnEvery + jitter):
				sessCancel()
			case <-sessCtx.Done():
			}
		}()
	}
	go func() {
		<-sessCtx.Done()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	}()

	if c.cfg.autoReady {
		_ = send(&pokerproto.ClientMessage{Type: "ready", Ready: true, ActionId: c.actionID("ready")})
	}

	pending := map[string]time.Time{} // action_id -> sent time
	var pendMu sync.Mutex

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if sessCtx.Err() != nil {
				return nil // clean churn / shutdown
			}
			return err
		}
		var sm pokerproto.ServerMessage
		if err := goproto.Unmarshal(data, &sm); err != nil {
			continue
		}
		switch sm.Type {
		case "action_ack":
			pendMu.Lock()
			if t, ok := pending[sm.ActionId]; ok {
				c.m.recordAck(time.Since(t))
				delete(pending, sm.ActionId)
			}
			pendMu.Unlock()
		case "error":
			c.m.recordError(sm.Code)
			pendMu.Lock()
			delete(pending, sm.ActionId)
			pendMu.Unlock()
		case "state":
			snap := sm.Snapshot
			if snap == nil {
				continue
			}
			if snap.GetStage() == "complete" {
				c.m.recordHandComplete(snap.GetHandId())
			}
			// The gateway masks per-viewer identity; a synthetic client learns
			// it is "on the clock" only when CurrentPlayerId matches one of its
			// own seats. We can't read our sub from here, so act whenever the
			// snapshot says it's our turn AND we hold legal actions (the server
			// authoritatively rejects a wrong-turn act, counted as an error).
			la := snap.GetLegalActions()
			if snap.GetCurrentPlayerId() == "" || la == nil || len(la.GetActions()) == 0 {
				continue
			}
			if !c.isMySeat(snap) {
				continue
			}
			go c.actAfterThink(sessCtx, send, snap, &pending, &pendMu)
		}
	}
}

// isMySeat reports whether the current player is plausibly one of this
// client's seats. Without the viewer's own id in the snapshot we approximate:
// a synthetic client owns exactly one identity per table, and the gateway
// only ever delivers "your turn" legal actions for the viewer's own seat —
// so a non-nil LegalActions block with a current player is our cue.
func (c *client) isMySeat(snap *pokerproto.TableSnapshot) bool {
	// LegalActions is populated by ViewFor only for the viewer's own turn.
	return snap.GetLegalActions() != nil && snap.GetCurrentPlayerId() != ""
}

func (c *client) actAfterThink(ctx context.Context, send func(*pokerproto.ClientMessage) error,
	snap *pokerproto.TableSnapshot, pending *map[string]time.Time, pendMu *sync.Mutex) {

	think := c.cfg.thinkMin
	if d := c.cfg.thinkMax - c.cfg.thinkMin; d > 0 {
		think += time.Duration(c.rng.Int63n(int64(d)))
	}
	select {
	case <-time.After(think):
	case <-ctx.Done():
		return
	}

	action, amount := c.pickAction(snap.GetLegalActions())
	if action == "" {
		return
	}
	id := c.actionID(action)
	pendMu.Lock()
	(*pending)[id] = time.Now()
	pendMu.Unlock()
	c.m.actionsSent.Add(1)
	_ = send(&pokerproto.ClientMessage{
		Type:                    "act",
		Action:                  action,
		Amount:                  amount,
		ActionId:                id,
		ExpectedSnapshotVersion: snap.GetSnapshotVersion(),
		ExpectedHandId:          snap.GetHandId(),
	})
}

func (c *client) pickAction(la *pokerproto.LegalActions) (string, int64) {
	acts := map[string]bool{}
	for _, a := range la.GetActions() {
		acts[a] = true
	}
	// Weighted fold vs check/call. Raise only occasionally, to keep pots moving.
	if acts["check"] && c.rng.Intn(20) != 0 {
		return "check", 0
	}
	total := c.cfg.wFold + c.cfg.wCheckCall
	if acts["fold"] && total > 0 && c.rng.Intn(total) < c.cfg.wFold && !acts["check"] {
		return "fold", 0
	}
	if acts["call"] {
		return "call", la.GetCallAmount()
	}
	if acts["check"] {
		return "check", 0
	}
	if acts["fold"] {
		return "fold", 0
	}
	return "", 0
}

func (c *client) actionID(kind string) string {
	return fmt.Sprintf("lt-%s-%s-%d", c.tableID, kind, time.Now().UnixNano())
}

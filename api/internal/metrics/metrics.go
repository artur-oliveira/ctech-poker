// Package metrics is the one way this service emits a CloudWatch metric.
//
// # Why EMF and not a metrics SDK
//
// Seven performance issues (#204, #207, #218, #220, #221, #222, #233) shipped
// their cost reduction pinned by a test and left their production-signal
// acceptance criterion unmet, because there was no metric sink at all and none
// of them wanted to leave a seventh ad-hoc collector behind (#279). Of the
// three candidates, CloudWatch's embedded metric format is the one that adds
// nothing to the runtime: cmd/server already logs JSON with slog to stdout,
// the ctech-ec2-agent logs-tail service already ships /var/log/app/app.log to
// the app log group, and CloudWatch Logs extracts metrics from any ingested
// log event whose JSON carries an `_aws` node. No new SDK, no PutMetricData
// call on a hot path, no agent config, no OTel collector to operate. The cost
// is that a metric is only as reliable as the log pipeline behind it — EMF
// guarantees at-least-once delivery, so treat counters as approximate.
//
// # How to emit one
//
// One function, Record. Everything is a distribution: CloudWatch derives Sum,
// SampleCount, Minimum, Maximum and percentiles from the values array, so a
// counter is Record(name, Count, dims, 1) read back with the Sum statistic,
// and a latency is Record(name, Milliseconds, dims, elapsed) read back with
// p95. There is deliberately no separate counter/gauge/histogram API.
//
// # Never emit one line per event
//
// The symptom of the 2026-09-02 rearm storm was 5,779 WARN lines for a single
// table, and #207's breaker answers it by logging state transitions rather
// than attempts. A metric on a high-volume event has the same hazard, so
// Record does not write a log line: it accumulates into a bucket keyed by
// (name, unit, dimensions) and one EMF line is emitted per bucket per flush —
// every flushInterval, or as soon as a bucket reaches maxValuesPerMetric
// samples (EMF's own array limit). The storm above would produce ~58 lines
// instead of 5,779, with an exact Sum either way.
//
// # Dimensions are money
//
// Every distinct dimension combination is a separate custom metric on the
// bill. Dimension values must come from a bounded set — an action name, an
// outcome, a step name. Never a table id, hand id or player id: those belong
// in the log line next to the metric, not in Dims. maxSeries is the backstop
// if that rule is broken anyway.
package metrics

import (
	"encoding/json"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Unit is a CloudWatch metric unit. Only the ones this service has a use for
// are named; the EMF spec's full list is in the MetricDatum reference.
type Unit string

const (
	Count        Unit = "Count"
	Milliseconds Unit = "Milliseconds"
	Bytes        Unit = "Bytes"
	None         Unit = "None"
)

const (
	// defaultNamespace is overridden by METRICS_NAMESPACE. The environment is
	// carried as a dimension rather than folded into the namespace so one
	// alarm definition can be pointed at any stage.
	defaultNamespace = "CTechPoker"

	// maxValuesPerMetric is the EMF limit on a numeric array metric target
	// (100). A bucket that reaches it is flushed immediately instead of
	// dropping samples, so Sum and SampleCount stay exact no matter the rate.
	maxValuesPerMetric = 100

	// flushInterval matches CloudWatch's standard 1-minute storage
	// resolution: emitting more often would not buy any more resolution.
	flushInterval = time.Minute

	// maxSeries bounds how many distinct (name, unit, dimensions) buckets may
	// exist between flushes. Reaching it means a dimension carries unbounded
	// values, which is a billing incident, so the excess is dropped and said
	// out loud once per flush rather than silently emitted.
	maxSeries = 256
)

type seriesKey struct {
	name string
	unit Unit
	// dims is the dimension set flattened to "k=v\x00k=v" with keys sorted,
	// so it is comparable and usable as a map key.
	dims string
}

type bucket struct {
	dims   map[string]string
	values []float64
}

type registry struct {
	mu      sync.Mutex
	series  map[seriesKey]*bucket
	dropped int

	namespace string
	env       string
	nowFunc   func() time.Time
	emit      func(payload map[string]any)
}

var (
	defaultRegistry = newRegistry()
	startOnce       sync.Once
)

func newRegistry() *registry {
	namespace := os.Getenv("METRICS_NAMESPACE")
	if namespace == "" {
		namespace = defaultNamespace
	}
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "dev"
	}
	return &registry{
		series:    make(map[seriesKey]*bucket),
		namespace: namespace,
		env:       env,
		nowFunc:   time.Now,
		emit:      emitLine,
	}
}

// Dims is a metric's dimension set. Keys and values must both come from a
// bounded set — see the package comment.
type Dims map[string]string

// Record adds one observation of name to the current window. It never blocks
// on I/O and never writes a log line of its own; see the package comment for
// why aggregation rather than one line per event is the point.
func Record(name string, unit Unit, dims Dims, value float64) {
	startOnce.Do(func() { go defaultRegistry.flushLoop() })
	defaultRegistry.record(name, unit, dims, value)
}

// Flush emits every bucket accumulated so far. The flush loop calls it once a
// minute; tests call it directly.
func Flush() { defaultRegistry.flush() }

func (r *registry) record(name string, unit Unit, dims Dims, value float64) {
	key := seriesKey{name: name, unit: unit, dims: flatten(dims)}

	r.mu.Lock()
	b := r.series[key]
	if b == nil {
		if len(r.series) >= maxSeries {
			r.dropped++
			r.mu.Unlock()
			return
		}
		b = &bucket{dims: copyDims(dims)}
		r.series[key] = b
	}
	b.values = append(b.values, value)
	var ready *bucket
	if len(b.values) >= maxValuesPerMetric {
		ready = &bucket{dims: b.dims, values: b.values}
		delete(r.series, key)
	}
	r.mu.Unlock()

	if ready != nil {
		r.emit(r.payload(key, ready))
	}
}

func (r *registry) flush() {
	r.mu.Lock()
	series := r.series
	dropped := r.dropped
	r.series = make(map[seriesKey]*bucket)
	r.dropped = 0
	r.mu.Unlock()

	for key, b := range series {
		r.emit(r.payload(key, b))
	}
	if dropped > 0 {
		// Not a metric: a metric would need a dimension set of its own, and
		// the thing worth seeing is which build started emitting unbounded
		// dimensions.
		slog.Error("metrics: dimension cardinality over the series cap, samples dropped",
			"dropped", dropped, "cap", maxSeries)
	}
}

func (r *registry) flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for range ticker.C {
		r.flush()
	}
}

// payload builds one EMF document. Environment is always a dimension so the
// same metric name from dev, stage and prod stays three separate series.
func (r *registry) payload(key seriesKey, b *bucket) map[string]any {
	dimensionNames := []string{environmentDimension}
	out := map[string]any{environmentDimension: r.env}
	for name, value := range b.dims {
		dimensionNames = append(dimensionNames, name)
		out[name] = value
	}
	sort.Strings(dimensionNames)

	// A one-element array would be valid EMF too, but a bare number is what
	// every EMF example shows and what reads best in the log.
	if len(b.values) == 1 {
		out[key.name] = b.values[0]
	} else {
		out[key.name] = b.values
	}
	out["_aws"] = map[string]any{
		"Timestamp": r.nowFunc().UnixMilli(),
		"CloudWatchMetrics": []map[string]any{{
			"Namespace":  r.namespace,
			"Dimensions": [][]string{dimensionNames},
			"Metrics":    []map[string]any{{"Name": key.name, "Unit": string(key.unit)}},
		}},
	}
	return out
}

const environmentDimension = "Environment"

// emitDisabled stops the last-mile write below, without touching any
// Record() caller. Every distinct (name, unit, dimensions) EMF line becomes a
// billed CloudWatch custom metric ($0.30/series/month); DynamoConsumedCapacity
// alone (Table x Operation, ~29 tables x up to 5 ops) was projected at ~100-150
// series on top of the fixed ~36 from the rest of this package. Flip back to
// false once a cheaper sink (self-hosted Prometheus/VictoriaMetrics, or a
// capped dimension set) is in place.
const emitDisabled = true

// emitLine writes the EMF document as its own line on the service's normal
// JSON log stream. It bypasses slog: slog would nest the payload under an
// attribute key or, at best, interleave `time`/`level`/`msg` members with the
// EMF target members, and EMF target members must sit on the root node.
// Marshalling by hand keeps the line exactly the document CloudWatch parses.
func emitLine(payload map[string]any) {
	if emitDisabled {
		return
	}
	line, err := json.Marshal(payload)
	if err != nil {
		slog.Error("metrics: could not marshal EMF payload", "err", err)
		return
	}
	// One Write, so two goroutines flushing at once cannot interleave halves
	// of a line — os.Stdout writes are not otherwise synchronised.
	os.Stdout.Write(append(line, '\n'))
}

func flatten(dims Dims) string {
	if len(dims) == 0 {
		return ""
	}
	names := make([]string, 0, len(dims))
	for name := range dims {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(dims[name])
		b.WriteByte(0)
	}
	return b.String()
}

func copyDims(dims Dims) map[string]string {
	out := make(map[string]string, len(dims))
	for name, value := range dims {
		out[name] = value
	}
	return out
}

// Package metrics emits CloudWatch Embedded Metric Format (EMF) JSON lines
// to stdout — auto-extracted by CloudWatch into metrics with zero new infra.
package metrics

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

// EmitTableMetric writes an EMF metric line for (name, value) with dimensions to stdout.
func EmitTableMetric(env, name string, value float64, dims map[string]string) {
	EmitTableMetricTo(os.Stdout, env, name, value, dims)
}

// EmitTableMetricTo writes an EMF metric line to w.
func EmitTableMetricTo(w io.Writer, env, name string, value float64, dims map[string]string) {
	namespace := "CtechPoker/" + env
	if env == "" {
		namespace = "CtechPoker"
	}

	dimKeys := make([]string, 0, len(dims))
	fields := map[string]any{name: value}
	for k, v := range dims {
		dimKeys = append(dimKeys, k)
		fields[k] = v
	}

	fields["_aws"] = map[string]any{
		"Timestamp": timeNowMillis(),
		"CloudWatchMetrics": []map[string]any{
			{
				"Namespace":  namespace,
				"Dimensions": [][]string{dimKeys},
				"Metrics":    []map[string]string{{"Name": name}},
			},
		},
	}

	line, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_, _ = w.Write(append(line, '\n'))
}

func timeNowMillis() int64 {
	return timeNowFunc().UnixMilli()
}

var timeNowFunc = func() time.Time { return time.Now() }

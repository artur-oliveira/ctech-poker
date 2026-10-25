// Package opsalert wires go-common alerts into poker's failure paths: a human
// must know about a failed refund, a quarantined cash-out and a stuck account
// deletion. Alerts go to the shared SNS topic ctech-{env}-alerts only; there
// are no CloudWatch alarms or metrics (ctech-skills infra.md).
package opsalert

import (
	"context"
	"log/slog"
	"time"

	"gopkg.aoctech.app/api-commons/alerts"
)

// New never fails: an unconfigured or broken topic yields Nop, because a job
// that cannot page anybody must still do its work.
func New(ctx context.Context, region, topicARN, env string) alerts.Publisher {
	p, err := alerts.New(ctx, region, topicARN, "poker", env)
	if err != nil {
		slog.ErrorContext(ctx, "alerting unavailable", "error", err)
		return alerts.Nop{}
	}
	return p
}

// timeout bounds the SNS publish so a slow topic cannot hold the caller.
const timeout = 3 * time.Second

// Send publishes one alert, detached from ctx cancellation (a Lambda about to
// return still gets to report) and bounded by timeout.
func Send(ctx context.Context, p alerts.Publisher, a alerts.Alert) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	p.Alert(ctx, a)
}

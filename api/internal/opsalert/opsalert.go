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
	"gopkg.aoctech.app/api-commons/erasure"
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

// AlertingPurge wraps an erasure purge so a failure also tells an operator: an
// account-deletion request that cannot finish is a legal deadline (LGPD). The
// error is returned unchanged, so redelivery behaves as before. Same shape as
// ctech-dfe's services.AlertingPurge.
func AlertingPurge(purge erasure.PurgeFunc, p alerts.Publisher) erasure.PurgeFunc {
	return func(ctx context.Context, m erasure.Message) (erasure.Ack, error) {
		ack, err := purge(ctx, m)
		if err != nil {
			Send(ctx, p, alerts.Alert{
				Job:     "erasure-purge-failed",
				Summary: "An account-deletion purge failed; the message stays on the queue and will be redelivered.",
				Err:     err,
				Detail:  "request_id: " + m.RequestID,
			})
		}
		return ack, err
	}
}

// Package v1 wires the poker HTTP routes onto a Fiber app.
package v1

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/health"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// RegisterHealth mounts the liveness probe (/v1.0/health) and the detailed health
// check (/v1.0/health-check) from go-common health. The ALB target group probes
// the detailed one and accepts 200 and 207, so a degraded (warn) instance keeps
// serving traffic while a 503 takes it out of rotation.
//
// DynamoDB is load-bearing: without the authoritative table state the API
// cannot safely serve a game, so a failed DescribeTable is the only thing that
// makes the probe 503. CPU and memory only degrade it to warn/207.
func RegisterHealth(router fiber.Router, cfg *config.Config, db *dynamodb.Client) {
	// A nil *dynamodb.Client wrapped in the interface would be non-nil.
	var describer health.TableDescriber
	if db != nil {
		describer = db
	}
	h := health.New(health.Config{
		ServiceID:   "CTech Poker",
		Description: "Health check details for CTech Poker API",
		Version:     "/v1.0",
		ReleaseID:   cfg.AppVersion,
	}).Add(
		health.DynamoDB(describer, dynamo.TableName(cfg.Env, "poker_table_state"), true),
		health.CPU(),
		health.Memory(),
	)
	router.Get("/health", h.Liveness())
	router.Get("/health-check", h.Report())
}

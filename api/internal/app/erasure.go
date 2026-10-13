package app

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/userpurge"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}

func newEligibility(sessions *sessionlog.Store, pending *reconcile.PendingStore) *userpurge.Eligibility {
	return userpurge.NewEligibility(sessions, pending)
}

func registerErasureRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, elig *userpurge.Eligibility) {
	v1.RegisterErasure(app.Group("/v1.0"), verifier, cfg.ErasureAccountClientID, elig.Blockers)
}

package app

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}

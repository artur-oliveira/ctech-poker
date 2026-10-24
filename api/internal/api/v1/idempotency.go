package v1

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/idempotency"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// newIdempotency is the `Idempotency-Key` middleware of the routes that move
// chips or money (purchases and refunds, the daily-reward spin, paid hand
// reveals, joining a table). Mount it AFTER auth: the key is scoped to the
// player. A request without the header is unchanged (Required is false until
// the UI and CLI send one), so the middleware only ever touches the store for a
// request that opted in. Fail-closed (the default): with a key and no store, a
// retry gets 503 instead of running unprotected. nil without DynamoDB (route
// tests); optional() turns that into a pass-through.
func newIdempotency(cfg *config.Config, db *dynamodb.Client) fiber.Handler {
	if db == nil {
		return nil
	}
	return idempotency.New(idempotency.Config{
		Store: idempotency.NewDynamoStore(dynamo.NewBase(db, cfg.Env, "poker_idempotency")),
		Scope: func(c fiber.Ctx) (string, bool) {
			id, ok := c.Locals(localsUserID).(string)
			return id, ok && id != ""
		},
	})
}

func optional(h fiber.Handler) fiber.Handler {
	if h == nil {
		return func(c fiber.Ctx) error { return c.Next() }
	}
	return h
}

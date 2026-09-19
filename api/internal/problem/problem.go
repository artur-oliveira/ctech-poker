// Package problem emits RFC 9457 Problem Details consistently across the API.
package problem

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/observability"
	fiberobs "gopkg.aoctech.app/api-commons/observability/fiber"
	common "gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

const ContentType = "application/problem+json"

type Problem struct{ common.Problem }

func (p *Problem) Send(c fiber.Ctx) error {
	body, err := json.Marshal(p)
	if err != nil {
		observability.Error(c.Context(), "problem response marshal failed", err,
			"status", p.Status, "problem_type", p.Type, "method", c.Method(), "path", c.Path())
		return err
	}
	fiberobs.LogHTTPError(c, p.Status, p.Type, p.Cause())
	c.Status(p.Status)
	c.Set(fiber.HeaderContentType, ContentType)
	return c.Send(body)
}

func wrap(p *common.Problem) *Problem { return &Problem{Problem: *p} }
func (p *Problem) WithCause(err error) *Problem {
	p.Problem.WithCause(err)
	return p
}

// WithNextAction attaches structured recovery guidance (#319) — see
// common.Problem.WithNextAction. Re-declared here (rather than relying on
// method promotion) so it keeps returning *Problem for fluent chaining.
func (p *Problem) WithNextAction(action string, retryAfterSeconds int) *Problem {
	p.Problem.WithNextAction(action, retryAfterSeconds)
	return p
}
func New(status int, typ, title, detail string) *Problem {
	return wrap(common.New(status, typ, title, detail))
}
func BadRequest(detail string) *Problem { return wrap(common.BadRequest(detail)) }

// Unauthorized carries NextActionReauthenticate (#319): an invalid/expired
// token is fixed by refreshing it or signing in again, not by retrying the
// same request or waiting.
func Unauthorized(detail string) *Problem {
	return wrap(common.Unauthorized(detail).WithNextAction(common.NextActionReauthenticate, 0))
}
func Forbidden(detail string) *Problem { return wrap(common.Forbidden(detail)) }
func NotFound(detail string) *Problem  { return wrap(common.NotFound(detail)) }
func Conflict(detail string) *Problem  { return wrap(common.Conflict(detail)) }

// TableFull carries NextActionRetry (#319): the seat that was lost may free up
// again, so retrying (e.g. against another table in the lobby) is the client's
// correct move rather than treating it as terminal.
func TableFull() *Problem {
	return New(http.StatusConflict, "/problems/table-full", "Table Full", "the last available seat was taken").
		WithNextAction(common.NextActionRetry, 0)
}
// TooManyRequests carries retry_after_seconds set to the rate limiter's own
// fixed window (#319), so the client knows how long to back off instead of
// parsing a textual detail or guessing.
func TooManyRequests(retryAfterSeconds int) *Problem {
	return wrap(common.TooManyRequestsAfter("too many requests, slow down", retryAfterSeconds))
}

func InternalServer(detail string, c fiber.Ctx, err error) *Problem {
	return wrap(common.InternalServer(detail)).WithCause(err)
}

func NotImplemented(detail string) *Problem {
	return &Problem{Problem: *common.New(http.StatusNotImplemented, "/problems/not-implemented", "Not Implemented", detail)}
}

func FromError(err error, c fiber.Ctx) *Problem {
	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		switch fiberErr.Code {
		case http.StatusBadRequest:
			return BadRequest(fiberErr.Message).WithCause(fiberErr)
		case http.StatusUnauthorized:
			return Unauthorized(fiberErr.Message).WithCause(fiberErr)
		case http.StatusForbidden:
			return Forbidden(fiberErr.Message).WithCause(fiberErr)
		case http.StatusNotFound:
			return NotFound(fiberErr.Message).WithCause(fiberErr)
		case http.StatusConflict:
			return Conflict(fiberErr.Message).WithCause(fiberErr)
		default:
			if fiberErr.Code >= 400 && fiberErr.Code < 500 {
				return New(fiberErr.Code, "/problems/http-error", http.StatusText(fiberErr.Code), fiberErr.Message).WithCause(fiberErr)
			}
		}
	}
	return InternalServer("an unexpected error occurred", c, err)
}

// FromWalletError passes ctech-wallet's own problem+json straight through
// (Status/Type/Title/Detail) when err wraps a *walletclient.Error, instead of
// letting a caller's generic fallback stringify the whole wrapped Go error
// chain (e.g. "buyin: debit: walletclient: Insufficient Balance: ...") into
// the response detail. The wrapping prefixes are for server logs/traces only
// — never meant to reach the client. ok is false when err isn't a wallet
// error, so the caller can fall back to its own generic problem.
func FromWalletError(err error) (p *Problem, ok bool) {
	var werr *walletclient.Error
	if errors.As(err, &werr) {
		p := New(werr.Status, werr.Type, werr.Title, werr.Detail)
		// #319: propagate ctech-wallet's own next_action/retry_after_seconds
		// verbatim when it already set them. Neither is a guess made up here
		// — an absent NextAction leaves the field unset (omitempty), same as
		// every other constructor that has no answer for it.
		if werr.NextAction != "" {
			p.WithNextAction(werr.NextAction, werr.RetryAfterSeconds)
		}
		return p, true
	}
	return nil, false
}

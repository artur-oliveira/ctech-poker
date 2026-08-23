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
func New(status int, typ, title, detail string) *Problem {
	return wrap(common.New(status, typ, title, detail))
}
func BadRequest(detail string) *Problem   { return wrap(common.BadRequest(detail)) }
func Unauthorized(detail string) *Problem { return wrap(common.Unauthorized(detail)) }
func Forbidden(detail string) *Problem    { return wrap(common.Forbidden(detail)) }
func NotFound(detail string) *Problem     { return wrap(common.NotFound(detail)) }
func Conflict(detail string) *Problem     { return wrap(common.Conflict(detail)) }
func TableFull() *Problem {
	return New(http.StatusConflict, "/problems/table-full", "Table Full", "the last available seat was taken")
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

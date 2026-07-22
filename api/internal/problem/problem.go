// Package problem emits RFC 9457 Problem Details consistently across the API.
package problem

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	common "gopkg.aoctech.app/api-commons/problem"
)

const ContentType = "application/problem+json"

type Problem struct{ common.Problem }

func (p *Problem) Send(c fiber.Ctx) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	c.Status(p.Status)
	c.Set(fiber.HeaderContentType, ContentType)
	return c.Send(body)
}

func wrap(p *common.Problem) *Problem { return &Problem{Problem: *p} }
func New(status int, typ, title, detail string) *Problem {
	return wrap(common.New(status, typ, title, detail))
}
func BadRequest(detail string) *Problem   { return wrap(common.BadRequest(detail)) }
func Unauthorized(detail string) *Problem { return wrap(common.Unauthorized(detail)) }
func Forbidden(detail string) *Problem    { return wrap(common.Forbidden(detail)) }
func NotFound(detail string) *Problem     { return wrap(common.NotFound(detail)) }
func Conflict(detail string) *Problem     { return wrap(common.Conflict(detail)) }
func InternalServer(detail string, c fiber.Ctx, err error) *Problem {
	slog.Error("unhandled error", "request_id", requestid.FromContext(c), "path", c.Path(), "err", err)
	return wrap(common.InternalServer(detail))
}

func NotImplemented(detail string) *Problem {
	return &Problem{Problem: *common.New(http.StatusNotImplemented, "/problems/not-implemented", "Not Implemented", detail)}
}

func FromError(err error, c fiber.Ctx) *Problem {
	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		switch fiberErr.Code {
		case http.StatusBadRequest:
			return BadRequest(fiberErr.Message)
		case http.StatusUnauthorized:
			return Unauthorized(fiberErr.Message)
		case http.StatusForbidden:
			return Forbidden(fiberErr.Message)
		case http.StatusNotFound:
			return NotFound(fiberErr.Message)
		case http.StatusConflict:
			return Conflict(fiberErr.Message)
		default:
			if fiberErr.Code >= 400 && fiberErr.Code < 500 {
				return New(fiberErr.Code, "/problems/http-error", http.StatusText(fiberErr.Code), fiberErr.Message)
			}
		}
	}
	return InternalServer("an unexpected error occurred", c, err)
}

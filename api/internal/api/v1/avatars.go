package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/avatar"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// Public avatar reads.
//
// Until the Cloudflare migration these were served by a CloudFront behaviour
// that rewrote /avatars/* onto the bucket's av/ prefix over OAC. The frontend
// is a static Cloudflare deployment now and there is no distribution in front
// of the app, so the API serves the bytes itself and AVATAR_BASE_URL points
// here (/ctech/{env}/poker/avatar-base-url).
//
// Unauthenticated on purpose: every client that renders a table needs these
// URLs, they already appear in profile payloads read by any authenticated
// caller, and the URL itself is the only capability. What is NOT public is the
// bucket — the response is the object's bytes, never a redirect to a presigned
// URL, so an avatar URL can never be traded for direct S3 access.
func RegisterAvatars(router fiber.Router, avatars *avatar.Service, readLimiter *RateLimiter) {
	h := &avatarHandlers{avatars: avatars}
	router.Get("/avatars/:userId/:file", rateLimit(readLimiter, ipKey("avatar-read")), h.read)
}

type avatarHandlers struct {
	avatars *avatar.Service
}

func (h *avatarHandlers) read(c fiber.Ctx) error {
	version, ok := avatarVersion(c.Params("file"))
	if !ok {
		return problem.NotFound("avatar not found").Send(c)
	}
	object, err := h.avatars.Get(c.Context(), c.Params("userId"), version)
	if err != nil {
		if errors.Is(err, avatar.ErrNotFound) || errors.Is(err, avatar.ErrInvalidKey) {
			return problem.NotFound("avatar not found").Send(c)
		}
		// A real storage failure is not a missing avatar. It renders the same
		// broken image either way, but only one of the two is worth paging on.
		slog.Warn("avatar: read failed", "err", err)
		return problem.New(http.StatusBadGateway, "/problems/avatar-unavailable", "Avatar unavailable", "avatar storage is unavailable").Send(c)
	}
	// Keys are versioned and never rewritten, so immutable is honest and no
	// revalidation round trip is ever needed. The ETag is S3's MD5 of the
	// object (a strong validator) and is there for caches that ignore that.
	c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	c.Set(fiber.HeaderContentType, avatar.PublishedType)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	if object.ETag != "" {
		c.Set(fiber.HeaderETag, object.ETag)
	}
	// No defer Close: fasthttp writes the body after this handler returns and
	// closes the stream itself once it does (Response.closeBodyStream closes
	// any io.Closer). Closing here would race the write.
	return c.SendStream(object.Body, int(object.ContentLength))
}

// avatarVersion parses the "{version}.jpg" half of a public avatar URL. Digits
// only and bounded, because it becomes part of an S3 key.
func avatarVersion(file string) (int, bool) {
	digits, found := strings.CutSuffix(file, ".jpg")
	if !found || digits == "" || len(digits) > 9 {
		return 0, false
	}
	version, err := strconv.Atoi(digits)
	if err != nil || version < 1 {
		return 0, false
	}
	return version, true
}

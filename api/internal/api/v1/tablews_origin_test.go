package v1

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func newOriginTestCtx(origin string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	if origin != "" {
		ctx.Request.Header.Set("Origin", origin)
	}
	return ctx
}

// TestWsAllowedOrigin covers the #44 fix: once an allow-list is configured,
// a missing Origin header must be rejected, not treated as "nothing to
// check". Dev (no allow-list configured) is unaffected.
func TestWsAllowedOrigin(t *testing.T) {
	allowed := []string{"https://poker.aoctech.app", "https://staging.poker.aoctech.app"}

	tests := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{
			name:    "dev: no allow-list configured, no Origin header",
			allowed: nil,
			origin:  "",
			want:    true,
		},
		{
			name:    "dev: no allow-list configured, any Origin header",
			allowed: nil,
			origin:  "https://evil.example.com",
			want:    true,
		},
		{
			name:    "prod: allow-list configured, missing Origin header is rejected",
			allowed: allowed,
			origin:  "",
			want:    false,
		},
		{
			name:    "prod: allow-list configured, unlisted Origin is rejected",
			allowed: allowed,
			origin:  "https://evil.example.com",
			want:    false,
		},
		{
			name:    "prod: allow-list configured, listed Origin succeeds",
			allowed: allowed,
			origin:  "https://poker.aoctech.app",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newOriginTestCtx(tt.origin)
			got := wsAllowedOrigin(ctx, tt.allowed)
			if got != tt.want {
				t.Errorf("wsAllowedOrigin(origin=%q, allowed=%v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

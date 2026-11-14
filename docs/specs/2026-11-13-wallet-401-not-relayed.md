# A wallet 401/403 is never relayed to the player

**Symptom (HAR, 2026-11-13):** `POST /v1.0/rooms/join-or-create` answered `401 /problems/unauthorized
"invalid credentials"` on every attempt, while `POST accounts-api /token` returned 200 and `GET /players/me`
worked with the same fresh token. The UI's 401 interceptor refreshed and retried, got 401 again, and the
player could not buy in.

**Root cause:** the 401 did not come from poker's JWT check (the join limiter's `ratelimit-*` headers were on the
response, so `authMiddleware` had already passed). It came from `ctech-wallet`: poker's own M2M
(`client_credentials`) token was rejected, and `walletProblem` copied the wallet's status verbatim. A service
credential failure was reported as an expired *player* session.

**Fix:** `walletProblem` (`api/internal/api/v1/problems.go`) maps wallet 401/403 to `503 /problems/unavailable`
(retryable, no wallet detail leaked) and logs an ERROR with the wallet status. Covers every handler that uses it
(buy-in, join-or-create, purchases, reveal, daily reward).

**Upstream root cause (found via AWS logs, same day):** ctech-account runs two processes behind nginx; after the
03:15Z key rotation one never reloaded, so `/.well-known/jwks.json` alternated between two key sets and verifiers
that cached the stale one (ctech-wallet) rejected M2M tokens signed with the new kid. Fixed in ctech-account #131
(per-tick rotator deadline) and ctech-go-common v1.40.1 (`jwtverify` keeps recently seen keys 15 min); this change
bumps `api-commons` to v1.40.1. The account process was restarted by hand. Other consumers (wallet, billing, dfe,
account) still need the bump.

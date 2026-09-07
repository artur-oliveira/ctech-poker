# `GET /v1.0/leaderboard` is public again

**Status:** implemented · **Date:** 2026-09-06

## Problem

`https://poker-api.aoctech.app/v1.0/leaderboard?mode=sandbox` returned 401 to anonymous callers. The
ranking is a **public page** — it is meant to be linkable and readable without an account. The B9
authz pass had mounted it behind `authMiddleware` together with `/leaderboard/me` and the hand
history route, and a test (`TestLeaderboardRequiresAuth`) pinned that.

## Fix

`RegisterLeaderboard` splits the two routes:

- `GET /v1.0/leaderboard` — **no auth**, with an IP rate limiter (`120/min`) in its place. Same
  shape `RegisterAvatars` already uses for its public reads.
- `GET /v1.0/leaderboard/me` — unchanged, still behind `auth`: it ranks the JWT's own `sub` and has
  nobody to rank without a user token.

The public route exposes only what a signed-in player could already read (display name, hands
played/won, achievement points, win rate) and takes no caller-supplied player id, so there is no
IDOR surface. Scope enforcement (`enforceReadOnlyScope`, which is what `poker:leaderboard:read`
gates) lives inside `authMiddleware`, so it now applies to `/leaderboard/me` only — the public board
is public for every caller, scoped token or not.

`TestLeaderboardRequiresAuth` is replaced by `TestLeaderboardTopIsPublic`; `/leaderboard/me`'s own
401 stays pinned by the pre-existing `TestLeaderboardMeRequiresAuth`.

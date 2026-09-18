# #322 — botcheck adaptive challenge: security self-review

This is an AI-agent self-review, written to satisfy #322's "revisão de segurança
documentada antes do merge" acceptance criterion as far as an agent reasonably
can. It is **not** a substitute for a human security sign-off — flagging that
explicitly, since #322 calls this module "a defesa anti-bot" and says a
badly-designed contestation flow "vira bypass." A human reviewer with security
ownership should confirm the points below before this ships to prod.

## What changed

1. `botcheck.DecideChallengeLevel(RiskSignal) ChallengeLevel` — a pure function
   grading an existing risk signal (the ws gateway's own decision-latency bot
   score) into `ChallengeStandard` / `ChallengeEscalated`. It has no network
   access and cannot itself accept or reject a Turnstile token.
2. `tablews.go` wiring: when a connection's risk score crosses the existing
   `>= 16` threshold (unchanged), the level is computed and stored. On a
   *successful* `Verify` (unchanged Turnstile call, unchanged action/hostname
   check), an escalated connection's risk budget resets to 8 instead of 0, so
   its next challenge arrives sooner. A *failed* `Verify` is completely
   unaffected by the level — `checker.Verify`'s fail-closed result and error
   path are untouched by this change.
3. `botcheck.ContestStore` (`poker_botcheck_contests`) — an append-only
   DynamoDB store for a player's claim that a block was a false positive, plus
   `POST/GET /players/me/bot-challenge/contests`. `Record` never calls
   Turnstile, never reads a token, and never changes anything `Verify`
   accepted or rejected. Filing a contest cannot unblock a player by itself —
   there is no code path from a stored `Contest` back into `Verify`'s
   decision, `challengeRequired`, or `botRiskScore`.

## Fail-closed review

- **Same action/hostname on the standard path.** `turnstileAction` and
  `expectedHostname` are unchanged constants/fields; `DecideChallengeLevel`
  cannot influence them — it is a separate, pure function called only to
  decide the size of the *next* risk budget, never inside `Verify`.
- **No new bypass surface.** Filing a contest performs one DynamoDB
  `PutItem` and nothing else. It does not set `challengeRequired = false`,
  does not zero `botRiskScore`, and does not touch the WS connection at all —
  `RegisterBotCheckContest` is a plain HTTP handler with no reference to any
  `Actor` or table connection.
- **Player identity.** Both new endpoints read `playerID` from
  `c.Locals(localsUserID)` (the verified JWT `sub`), never a client-supplied
  field — same rule as every other self-service endpoint in this repo
  (`playernotes`, `chatprefs`).
- **`secret == ""` still disables everything.** `Service.Enabled()` is
  unchanged; `DecideChallengeLevel` and `ContestStore` are both independent of
  it and add no new "half-enabled" state.
- **Rate limiting.** `POST /players/me/bot-challenge/contests` is not yet
  behind a dedicated rate limiter — it is authenticated (so already keyed to
  one player), but an abusive player could still write many contest rows.
  Left open deliberately (#322 does not ask for a moderation UI, and every
  other write here is `PutItem`-cheap), but worth a limiter
  (`internal/api/v1/ratelimit.go`, same pattern as `purchaseLimiter`) if this
  sees real traffic before a review screen exists.

## Explicitly out of scope (per #322 itself)

- Reading IP-level attempt counts from the existing rate limiter into
  `RiskSignal.RecentAttempts` — only `ActionRiskScore` is wired today. The
  thresholds (`highRiskAttempts`, `highRiskActionScore`) are heuristic, not
  validated against real false-positive data (#322 says this validation is
  explicitly not measured in this repo).
- A moderation/review screen for `ContestStatusPending` rows — `List` exists
  so one can be built, but nothing here reviews or resolves a contest.
- Changing the Turnstile widget contract (e.g. a stricter managed challenge
  mode) to reflect `ChallengeEscalated` client-side — the escalation is
  currently server-internal only (a smaller risk budget after a pass).

## Reviewer checklist

- [ ] Confirm `Verify`'s fail-closed behavior is genuinely untouched (diff
      `internal/botcheck/service.go` — no lines inside `Verify` changed).
- [ ] Confirm no code path reaches `challengeRequired = false` or
      `botRiskScore = 0` from `ContestStore`/`RegisterBotCheckContest`.
- [ ] Decide whether `POST .../contests` needs a rate limiter before prod
      traffic.
- [ ] Sign off here (name, date) once satisfied.

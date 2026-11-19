# Poker Scheduled Web Push Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Poker users get a streak reminder at a time they choose and a "come play today" push at the hour they usually play, both through Web Push.

**Architecture:** `pushsvc` (go-common `v1.44.0`) provides subscriptions, reminders, sender and invoker. Poker adds a preferences API, a 15-minute `push-tick` Lambda (EventBridge Scheduler) that applies the Poker rules and asks `push-sender` to send, and a notifications screen in the UI.

**Tech Stack:** Go (Fiber v3, aws-sdk-go-v2), go-common `pushsvc`, CDK TypeScript (existing `cdk/`), Next.js static export, `@aoctech/push-client`.

**Spec:** `docs/specs/2026-11-18-scheduled-web-push.md`. Prerequisite plan: `ctech-go-common/docs/plans/2026-11-18-pushsvc.md` (tagged `v1.44.0`).

## Global Constraints

- Load `ctech-dev-standards` first. Routes under `/v1.0/`. No new KMS key (table encryption key setting follows the existing Poker tables), no SQS, no `Scan`, no decrypt-in-loop: VAPID is read once per cold start.
- Time zone `America/Sao_Paulo`; slots of 15 minutes (`pushsvc.SlotOf`). Reminder kinds: `streak`, `play`.
- Defaults at device activation: `play` enabled; `streak` disabled until the user picks a time.
- At most one send per kind per day (`MarkSent`); `play` is skipped when the user already played today or when `streak` is enabled and its condition holds today.
- Typical hour: median start hour of the last 14 sessions; fewer than 5 sessions gives `19:00`. Recomputed after each `play` send.
- Payload carries no chip amounts. `navigate` is `/`. Permission is requested only on a click.
- Poker module is on go-common `v1.40.1`: bump to `v1.44.0` and set `FAMILY_GOCOMMON_MINOR=v1.44` in `scripts/standards-check.sh` usage (CI env) if the gate reads it.
- Sender and tick run inert without VAPID parameters (no failure).

## Review Focus

- User claims the daily bonus at 19:59, reminder slot 20:00: no streak push.
- User with `streak` and `play` both due the same day: only the streak push goes.
- Player with zero sessions (new): `play` slot `19:00`, no crash on an empty `sessionlog`.
- Two ticks overlapping, or the catch-up of the previous slot: one send.
- Push disabled in the browser (410): subscription deleted, reminders stay but nothing is sent when no device remains (skip the job, do not invoke).
- Streak already lost (`StreakLost`): no "your streak ends today" message.

---

### Task 1: Dependency, table, reminders API

**Files:** Modify `api/go.mod`; Create `api/internal/pokerpush/service.go`, `api/internal/pokerpush/service_test.go`, `api/internal/api/v1/push.go`, `api/internal/api/v1/push_test.go`; Modify the router wiring in `api/internal/app` (where `RegisterDailyReward` is registered) and `api/internal/config`.

**Interfaces:**
- Produces: `package pokerpush`: `const KindStreak = "streak"`, `KindPlay = "play"`, `DefaultPlaySlot = 76` (19:00); `type Preferences struct{StreakReminder struct{Enabled bool; Time string}; ComePlay struct{Enabled bool}}` (JSON `streak_reminder{enabled,time}`, `come_play{enabled}`); `type Service struct{...}`; `NewService(reminders *pushsvc.ReminderStore) *Service`; `Get(ctx, sub) (Preferences, error)`; `Set(ctx, sub string, p Preferences) (Preferences, error)` (validates `HH:MM` via `pushsvc.ParseHHMM`; `streak` enabled requires a time; preserves `last_sent_day`); `SeedOnSubscribe(ctx, sub) error` (creates `play` enabled at `DefaultPlaySlot` if absent; `streak` row absent).
- Routes: `GET|PUT /v1.0/push/preferences`, `POST /v1.0/push/test` (invokes `push-sender` with a job for the caller), plus `pushsvc.Mount` for config/subscriptions. Auth middleware and `localsUserID` as `RegisterDailyReward`.

- [ ] **Step 1:** Tests: `TestSetStreakRequiresTime`, `TestSetRejectsBadTime` (`"24:00"`, `""`), `TestSeedOnSubscribeEnablesPlayOnly`, `TestSeedOnSubscribeKeepsExistingChoice`, `TestPutPreferencesRoundTrip` (HTTP, 200 and body), `TestPreferencesRequireAuth` (401).
- [ ] **Step 2:** `cd api && go test ./internal/pokerpush ./internal/api/v1 -run 'Preferences|Seed|SetStreak|SetRejects' -v` → FAIL.
- [ ] **Step 3:** Implement; `go get gopkg.aoctech.app/api-commons@v1.44.0 && go mod tidy`. Subscribe hook: after `pushsvc.Service.Subscribe` succeeds call `SeedOnSubscribe`.
- [ ] **Step 4:** Same command → PASS; `go test ./...`.
- [ ] **Step 5:** Commit `feat(push): preferences API on pushsvc`.

### Task 2: Reminder rules

**Files:** Create `api/internal/pokerpush/rules.go`, `api/internal/pokerpush/rules_test.go`.

**Interfaces:**
- Consumes: `dailyreward.Service.Status(ctx, playerID) (Status, error)` (`ClaimedToday`, `StreakAtRisk`, `CurrentStreak`); `sessionlog.Store` (read of recent session start times, see Step 3); `pokerpush.Service`.
- Produces: `type Rules struct{...}`; `func NewRules(status StatusReader, sessions SessionReader, prefs *Service) *Rules`; `func (r *Rules) Decide(ctx, now time.Time, rem pushsvc.Reminder) (*pushsvc.Job, error)` (nil job means "do not send"); `func TypicalSlot(starts []time.Time) int` (median hour of start, rounded down to the 15-minute slot; `DefaultPlaySlot` below 5 sessions); `type SessionReader interface{ RecentStarts(ctx, playerID string, n int) ([]time.Time, error); PlayedOn(ctx, playerID, day string) (bool, error) }`.
- Texts (`pt-BR`): streak `"Sua ofensiva de {N} dias termina hoje"` / body `"Resgate o bônus diário para mantê-la."`; play `"A mesa está esperando"` / `"Venha jogar hoje."`. Tags `streak-{day}`, `play-{day}`; `Urgency: normal`; `TTLSeconds`: seconds until 23:59 BRT.

- [ ] **Step 1:** Tests (table-driven, fake readers): `TestStreakSendsWhenAtRisk`, `TestStreakSkipsWhenClaimedToday`, `TestStreakSkipsWhenNoStreak`, `TestStreakSkipsWhenStreakLost`, `TestPlaySkipsWhenPlayedToday`, `TestPlayYieldsToStreakWhenStreakDue`, `TestPlaySendsWhenStreakDisabled`, `TestTypicalSlotMedian` (starts at 20:10, 20:40, 21:05, 20:20, 19:55 → 20:20 → slot 81), `TestTypicalSlotDefaultsBelowFiveSessions`, `TestTextsCarryNoChipAmounts`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. Check `api/internal/sessionlog/store.go` for what already answers "sessions of a player, newest first" and "played today"; if absent, add `RecentStarts`/`PlayedOn` there as one `Query` each (no `Scan`) with an integration test (`-tags integration`). **Step 4:** PASS. **Step 5:** Commit.

### Task 3: Tick and sender Lambdas

**Files:** Create `api/cmd/push-tick/main.go`, `api/cmd/push-tick/tick.go`, `api/cmd/push-tick/tick_test.go`, `api/cmd/push-sender/main.go`.

**Interfaces:**
- Consumes: `pushsvc.SlotsToProcess`, `ReminderStore.ListSlot/MarkSent/Put`, `pushsvc.Invoker.Send`, `pokerpush.Rules.Decide`, `pushsvc.Store.List` (to skip a user with no device).
- Produces: `func (t *Tick) Run(ctx, now time.Time) error` (for each slot, page through `ListSlot`; for each reminder: skip if no device; `Decide`; if a job, `MarkSent` first and invoke only when it returned true; after a `play` send recompute `TypicalSlot` and `Put` the new slot when it differs by one slot or more). `push-sender/main.go` is `lambda.Start(pushsvc.Lambda(...))` (about 20 lines, env `TABLE_PREFIX`, `PUSH_VAPID_*_SSM_PATH`, `PUSH_SUBJECT_SSM_PATH`, `ALERTS_TOPIC_ARN`).

- [ ] **Step 1:** Tests with fakes: `TestRunSendsOncePerDayWhenTickedTwice`, `TestRunProcessesPreviousSlot`, `TestRunSkipsUserWithoutDevice`, `TestRunMarksBeforeInvoking` (invoker failure leaves it marked and is logged, never double-sent), `TestRunMovesPlaySlotAfterSend`, `TestRunKeepsGoingAfterOneUserFails`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement; per-user failures are logged with `sub_hash` and counted, the run returns an error only if the table read itself fails. Concurrency bounded to 8. **Step 4:** PASS `-race`. **Step 5:** Commit.

### Task 4: Infrastructure (CDK)

> **Decision needed before this task:** `ctech-skills/references/infra.md` says CDK is frozen and new Lambda sets go in Terraform, but Poker has no Terraform workspace and its sibling jobs (`reconcile-stack.ts`, `tablecleanup-stack.ts`) are CDK. This plan follows those stacks. If the user picks Terraform instead, only this task changes.

**Files:** Create `cdk/lib/push-stack.ts`, `cdk/test/push-stack.test.ts`; Modify `cdk/lib/dynamodb-stack.ts` (table `{prefix}_push` with GSIs `endpoint-index` and `slot-index`), `cdk/lib/constants.ts` (names), `cdk/bin/*.ts` (instantiate `PushStack` like `TableCleanupStack`).

**Interfaces:** Produces Lambdas `push-tick` and `push-sender` (PROVIDED_AL2023, ARM_64, 256 MB / 128 MB, timeouts 2 min / 30 s, `localGoBundling`), `scheduler.CfnSchedule` `cron(0/15 * * * ? *)` with `scheduleExpressionTimezone: 'America/Sao_Paulo'`, flexible window OFF, retry policy and DLQ as `TableCleanupSchedule`; roles with least privilege (tick: table read/write, `lambda:InvokeFunction` on sender; sender: table read/delete on subscription rows, SSM `GetParameter` on three `/ctech-poker/{env}/push/*` parameters, `sns:Publish` on the alerts topic); the API role gets `lambda:InvokeFunction` on the sender and table access.

- [ ] **Step 1:** CDK assertions: `TestPushTableHasBothIndexes`, `TestScheduleIsEveryFifteenMinutesInBRT`, `TestSenderRoleCannotWriteReminderRows` (no `PutItem`), `TestNoKmsKeyAdded` (resource count of `AWS::KMS::Key` unchanged). **Step 2:** `cd cdk && npx jest test/push-stack.test.ts` → FAIL. **Step 3:** Implement. **Step 4:** PASS, `npx cdk synth` clean. **Step 5:** Commit.

### Task 5: Account erasure and export

**Files:** Modify `api/internal/userpurge/*`, `api/internal/userexport/*` and their tests.

- [ ] **Step 1:** Tests `TestPurgeDeletesPushRows` (subscriptions, count row and both reminder rows of the user gone; other user untouched), `TestExportIncludesReminderPreferencesNotEndpoints` (preferences yes, endpoint/keys never). **Step 2:** FAIL. **Step 3:** Use `Store.List`+`Delete` and `ReminderStore.DeleteAll`; follow how `userpurge` handles `dailyreward` rows. **Step 4:** PASS. **Step 5:** Commit.

### Task 6: UI

**Files:** Modify `ui/package.json` (`@aoctech/push-client`); Create `ui/src/lib/providers/PushProvider.tsx`, `ui/src/lib/push/{callbacks,logout,device}.ts`, `ui/src/lib/api/push.ts`, `ui/src/components/NotificationSettings.tsx`, `ui/public/sw.js` (copy step as DFe's `scripts/`), plus tests under `ui/src/__tests__`; Modify the root layout (provider) and the existing settings/profile screen (mount the component).

**Interfaces:** Consumes `usePushSubscription`, `registerServiceWorker`, `onNavigateMessage` from `@aoctech/push-client`. Produces `usePush()`; `getPushPreferences(): Promise<PushPreferences>`, `putPushPreferences(p): Promise<PushPreferences>` over the existing axios client; `NotificationSettings` (button "Ativar neste dispositivo", switch "Lembrete de ofensiva" with a time input `type="time"` step 900, switch "Venha jogar hoje" default on, install hint for iOS). Copy and structure from `ctech-dfe/ui/src/lib/providers/PushProvider.tsx` and `ctech-dfe/ui/src/lib/push/*`; extract nothing new to `@aoctech/ui` in this plan.

- [ ] **Step 1:** Vitest: `renders nothing when /push/config says disabled`, `asks permission only on button click`, `saving the streak switch without a time shows an error and does not call the API`, `time input uses 15-minute steps`, `logout unsubscribes this device` (as DFe `push.test.ts`). **Step 2:** `cd ui && npx vitest run src/__tests__/lib/push` → FAIL. **Step 3:** Implement honoring the ui rules (static export, no hand-rolled OAuth, no magic strings, reduced motion). **Step 4:** PASS, `npm run lint`, bundle budget check. **Step 5:** Commit.

### Task 7: Turn it on

- [ ] **Step 1:** Print the keygen commands (`pushsvc/cmd/push-keygen`), operator writes the SSM parameters for the environment. **Step 2:** Deploy; confirm the named Deploy job succeeded. **Step 3:** From the UI, activate a device, set the streak time two slots ahead, confirm one push arrives and a second tick sends nothing. **Step 4:** Add Poker to the "push" section of `ctech-skills/references` (spec phase F5) and `docs/README.md` index entry.

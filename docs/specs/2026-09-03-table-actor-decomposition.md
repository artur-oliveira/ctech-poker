# Table actor source decomposition (issue #52)

Date: 2026-09-03

`api/internal/table/actor.go` previously mixed the actor loop, persistence,
presence, timers, player commands, hooks, and snapshot decoration in one
3,179-line file. The actor remains one `Actor` type with one command goroutine;
this change only moves same-package methods into focused source files:

- `actor.go`: state, construction, mailbox, panic boundary, and command routing
- `actor_activity.go`: chat/reactions/preselection and mutation snapshots
- `actor_loading.go`: authoritative loads and timer restoration
- `actor_commit.go`: action commits, replay frames, deadlines, and retries
- `actor_hooks.go`: fleet hand-hook claiming and manager callbacks
- `actor_presence.go`: connect/disconnect, kick/AFK handling, and idle exits
- `actor_player_actions.go`: sit-out, reveal, rabbit-hunt, and exit commands
- `actor_seating.go`: timeout actions and join/leave settlement
- `actor_timers.go`: turn, next-hand, winner-card, and runout timers
- `actor_views.go`: queued auto-actions, broadcasts, fleet presence/streaks,
  equity decoration, and test/config setters

No exported signature, command ordering, timer duration, persistence path, or
hook behavior changes. Keeping the files in package `table` intentionally
preserves direct access to actor-owned state while reducing `actor.go` to well
below the 900-line acceptance target.

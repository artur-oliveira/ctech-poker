# Testing policy — ui/

**Every new feature ships with the tests that cover it. No exceptions.**

Coverage is not a report we look at afterwards; it is a gate. `vitest.config.ts` enforces
**90% lines, 90% statements, 90% functions and 90% branches** over `src/**/*.{ts,tsx}`. A
change that pushes any of those under 90 fails `npx vitest run --coverage` and therefore CI.

## The rule

When you add or change behaviour, the same change adds the tests that exercise it — including
the branches that are easy to skip:

- the **error path** (request rejected, socket refused the frame, clipboard unavailable);
- the **empty / missing-data path** (no rows yet, `undefined` optional field, unknown enum value
  arriving from a newer server);
- the **disabled / blocked path** (not connected, cooling down, insufficient balance, not the
  viewer's turn);
- both sides of every user-visible conditional (owned vs. locked, expired vs. valid, reduced
  motion vs. animated, vertical vs. oval stage).

If a branch is genuinely unreachable, delete it rather than leaving it uncovered.

### Why the threshold is where it is

The gap the 80/70 thresholds allowed was mostly *type-shaped*: uncovered branches were the
`?? fallback`, the `as` cast and the optional field that no test ever supplied, which is exactly
where a wrong assumption about an API shape survives `tsc` and reaches production. Holding all
four metrics at 90 means a new optional field, status string or nullable prop has to be
exercised at least once before it ships.

## Lowering a threshold is not an option

`vitest.config.ts` thresholds may be **raised**, never lowered to make a change land. If a
change cannot reach 90, the missing test is the deliverable — not a smaller number.

Legitimate exclusions (files with no behaviour to assert) live in `coverage.exclude` in
`vitest.config.ts`: generated protobuf code (`src/lib/api/proto/**`), the mock runtime
(`src/dev/**`), the test setup, and the root layouts/providers that only wire children. Adding
a file there needs a reason in the PR description; "hard to test" is not one.

## How to write them here

- **Test through the UI, not the implementation.** Query by role, label and text
  (`getByRole('button', {name: 'Estornar'})`), not by class name, and assert what the player
  sees. Class-name queries are acceptable only for decorative, non-semantic markup that has no
  accessible name (`.hand-outcome-chips-delta`, `.street-progress`).
- **Reuse the mock runtime.** `src/dev/mockRuntime.ts` exports `snapshotForScenario` and
  `MOCK_PLAYER_ID`; prefer them over hand-built table snapshots so tests exercise the same
  shapes the backend sends.
- **Realtime hooks:** mock `@aoctech/ws-client`'s `useWebSocket`, capture the `options` it was
  given, and drive the hook by calling `options.onMessage(...)` inside `act`. See
  `src/lib/hooks/useTableRealtime.test.tsx`. For the in-memory engine path, mock
  `@/lib/mockConfig` with `USE_MOCK: true` (spread the original module — `MOCK_PLAYER_ID` and
  `MockScenario` still have to exist) as in `useTableRealtimeMock.test.tsx`.
- **Pages:** mock `next/navigation`, `@tanstack/react-query` and the heavy child components,
  capturing their props in a `vi.hoisted` object. That is what makes a page's callbacks
  (`onFavoriteReactionsChangeAction`, `onSaved`, …) directly assertable. See
  `src/app/table/page.test.tsx`.
- **Timers:** `vi.useFakeTimers()` inside a `try/finally` that restores real timers, and use
  `vi.advanceTimersByTimeAsync` when the code awaits a promise between ticks.
- **`window.matchMedia` is a shared mock** from `src/test/setup.ts`. If a test overrides it
  (reduced motion, compact layout, vertical stage), restore the non-matching implementation in
  an `afterEach` — otherwise later tests in the same file silently change behaviour.

## Running it

```bash
npx vitest run              # tests only
npx vitest run --coverage   # tests + the 90% gate
npx tsc --noEmit
npx eslint src --max-warnings 0
npm run build
```

All four must pass with zero errors and zero warnings before a change is done.

To see what a change left uncovered, `npx vitest run --coverage` prints a per-file table with
the uncovered line ranges; `coverage/index.html` has the same data with the branches
highlighted.

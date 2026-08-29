# Social UX polish + "Sem pressa" achievement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the unread social badge actionable everywhere, let a table invite be answered from its toast, let a friend who opted in be joined at a public table, and add the "Sem pressa" time-bank achievement.

**Architecture:** Four independent slices over the existing stack. Slices 1–2 are frontend-only. Slice 3 adds an opt-in profile flag, teaches `presence` to carry a room id, and gates its exposure at the `/social/friends` handler. Slice 4 returns the milliseconds `Actor.consumeTimeBank` already charges, persists them on the action-log entry that is being committed anyway, and sums them in the existing `onHandComplete` hook.

**Tech Stack:** Go 1.25 (Fiber v3, DynamoDB, Valkey), Next.js 16 / React 19 with TanStack Query, Vitest + Testing Library, protobuf over WebSocket.

**Spec:** `docs/specs/2026-08-27-social-ux-and-no-rush-achievement.md`

## Global Constraints

- All player-facing copy is Brazilian Portuguese. Existing tone: short, direct, no exclamation marks except where already present.
- Time-bank achievement thresholds are **milliseconds**: `60_000`, `3_600_000`, `86_400_000`, `604_800_000`, `2_592_000_000` (stars 1–5).
- Achievement key: `no_rush`. Metric name: `time_bank_ms_consumed`. Label: `Sem pressa`.
- `room_id` is exposed on a friend row **only** when all five gates hold: friend's `TablePublic` is true, presence is `in_table` with a non-empty room id, room `Visibility == "public"`, room `Status` is `waiting` or `active`, and `SeatsTaken < MaxSeats`.
- `TablePublic` defaults to false. No migration, no backfill.
- Go tests: `cd api && go test ./...`. UI tests: `cd ui && npx vitest run <path>`.
- Never add a `Co-Authored-By` trailer to a commit.

---

## File Structure

**Slice 1 — badge destination**
- Create `ui/src/lib/hooks/useSocialUnread.ts` — one hook, the shared unread count.
- Modify `ui/src/components/social/PeopleNavBadge.tsx` — consume the hook.
- Modify `ui/src/components/AppPageChrome.tsx` — route the Pessoas link by count.
- Modify `ui/src/app/people/page.tsx` — seed the tab from `?tab=`.

**Slice 2 — actionable toast**
- Modify `ui/src/lib/notify.ts` — optional actions on a notification.
- Modify `ui/src/components/Notifier.tsx` — render them.
- Modify `ui/src/lib/hooks/useLobbyRealtime.ts` — invite toast with Entrar/Recusar.

**Slice 3 — join a friend's table**
- Modify `api/internal/player/model.go`, `store.go`, `service.go` — `TablePublic`.
- Modify `api/internal/api/v1/player.go` — PATCH field + response field.
- Modify `api/internal/presence/{model,store,memory,valkey,service}.go` — room id.
- Modify `api/internal/sessionlog/store.go` — `FindLatestOpenSession` returns the table id.
- Modify `api/internal/buyin/service.go` — pass the room id.
- Modify `api/internal/api/v1/social.go` + `router.go` — the five gates.
- Modify `ui/src/lib/api/{player,social}.ts`, `ui/src/components/lobby/ProfileShowcaseDialog.tsx`, `ui/src/components/social/PeopleList.tsx`, `ui/src/app/people/page.tsx` (copy).

**Slice 4 — "Sem pressa"**
- Modify `api/internal/tablestore/store.go` — `TimeBankMs` on the log entry.
- Modify `api/internal/table/actor.go` — `consumeTimeBank` returns the charge.
- Modify `api/internal/achievements/{catalog,service}.go` — key, tiers, award.
- Modify `api/internal/app/app.go` — sum per player in `onHandComplete`.
- Modify `ui/src/lib/utils.ts`, `ui/src/lib/achievements.ts`, `ui/src/components/achievements/AchievementCard.tsx`, `ui/src/app/achievements/page.tsx`.

---

## Task 1: Shared unread hook + badge destination

**Files:**
- Create: `ui/src/lib/hooks/useSocialUnread.ts`
- Modify: `ui/src/components/social/PeopleNavBadge.tsx`
- Modify: `ui/src/components/AppPageChrome.tsx`
- Test: `ui/src/components/AppPageChrome.test.tsx`

**Interfaces:**
- Consumes: `SOCIAL_KEYS.summary` and `getSocialSummary` (already exist in `@/lib/social` and `@/lib/api/social`).
- Produces: `useSocialUnread(): number` from `@/lib/hooks/useSocialUnread`.

- [x] **Step 1: Write the failing test**

Append to `ui/src/components/AppPageChrome.test.tsx` (reuse whatever render helper and QueryClient wrapper the file already defines; the snippet below assumes a `renderWithClient(ui, client)` helper — if the file names it differently, use that name):

```tsx
import {QueryClient} from '@tanstack/react-query';
import {SOCIAL_KEYS} from '@/lib/social';

it('sends the Pessoas link to the activity tab while there are unread events', () => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  client.setQueryData(SOCIAL_KEYS.summary, {unread_count: 2});
  renderWithClient(<AppPageNav authed current="lobby"/>, client);
  for (const link of screen.getAllByRole('link', {name: /Pessoas/})) {
    expect(link).toHaveAttribute('href', '/people?tab=activity');
  }
});

it('sends the Pessoas link to the plain page with nothing unread', () => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  client.setQueryData(SOCIAL_KEYS.summary, {unread_count: 0});
  renderWithClient(<AppPageNav authed current="lobby"/>, client);
  for (const link of screen.getAllByRole('link', {name: /Pessoas/})) {
    expect(link).toHaveAttribute('href', '/people');
  }
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/components/AppPageChrome.test.tsx`
Expected: FAIL — the href is `/people` in both cases.

- [x] **Step 3: Create the hook**

`ui/src/lib/hooks/useSocialUnread.ts`:

```ts
'use client';
import {useQuery} from '@tanstack/react-query';
import {getSocialSummary} from '@/lib/api/social';
import {SOCIAL_KEYS} from '@/lib/social';

/** Unread social inbox events. Shared by the nav badge and by the nav link,
 * which points at the Atividades tab precisely when there is something there
 * to read — the badge is the only thing that clears it. */
export function useSocialUnread(): number {
  const {data} = useQuery({queryKey: SOCIAL_KEYS.summary, queryFn: getSocialSummary});
  return data?.unread_count ?? 0;
}
```

- [x] **Step 4: Consume the hook in the badge**

Replace the body of `ui/src/components/social/PeopleNavBadge.tsx`:

```tsx
'use client';
import {useSocialUnread} from '@/lib/hooks/useSocialUnread';

/** Unread social activity, mirrored from the same counter the socket pushes.
 * The number is spelled out for assistive tech: the dot alone would make the
 * badge color-only information. */
export function PeopleNavBadge() {
  const count = useSocialUnread();
  if (count <= 0) return null;
  return <>
    <span className="app-nav-people-badge" aria-hidden="true">{count > 9 ? '9+' : count}</span>
    <span className="sr-only"> — {count} {count === 1 ? 'novidade' : 'novidades'} em Pessoas</span>
  </>;
}
```

- [x] **Step 5: Route the link by the count**

In `ui/src/components/AppPageChrome.tsx`, import the hook:

```tsx
import {useSocialUnread} from '@/lib/hooks/useSocialUnread';
```

Add this helper next to `routeBadgeClass`:

```tsx
// The badge counts inbox events, and only the Atividades tab marks them read.
// Landing on Amigos (the page default) left the badge stuck with no obvious
// way to clear it.
function routeHref(route: MainRoute, href: string, unread: number) {
  return route === 'people' && unread > 0 ? '/people?tab=activity' : href;
}
```

In `AppPageNav`, add `const unread = useSocialUnread();` at the top of the component and change the desktop link to `href={routeHref(route, href, unread)}`. Do the same inside `AppTabBar` (add its own `const unread = useSocialUnread();` — the query is shared, so this costs no extra request).

- [x] **Step 6: Run tests to verify they pass**

Run: `cd ui && npx vitest run src/components/AppPageChrome.test.tsx src/components/social/socialComponents.test.tsx`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add ui/src/lib/hooks/useSocialUnread.ts ui/src/components/social/PeopleNavBadge.tsx \
        ui/src/components/AppPageChrome.tsx ui/src/components/AppPageChrome.test.tsx
git commit -m "feat(ui): point the Pessoas badge at the activity tab"
```

---

## Task 2: `/people` honours `?tab=`

**Files:**
- Modify: `ui/src/app/people/page.tsx`
- Test: `ui/src/app/people/page.test.tsx`

**Interfaces:**
- Consumes: `routeHref` from Task 1 produces `/people?tab=activity`.
- Produces: `/people?tab=<friends|requests|recent|blocked|activity>` opens that tab.

- [x] **Step 1: Write the failing test**

Append to `ui/src/app/people/page.test.tsx`. The file already mocks `next/navigation`; extend that mock so `useSearchParams` is controllable:

```tsx
const searchParams = {value: new URLSearchParams()};
vi.mock('next/navigation', async importOriginal => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useSearchParams: () => searchParams.value,
  useRouter: () => ({push: vi.fn(), replace: vi.fn()}),
}));

it('opens the activity tab when the url asks for it', async () => {
  searchParams.value = new URLSearchParams('tab=activity');
  renderPeople();
  expect(await screen.findByRole('radio', {name: 'Atividades'})).toBeChecked();
});

it('falls back to friends for an unknown tab', async () => {
  searchParams.value = new URLSearchParams('tab=garbage');
  renderPeople();
  expect(await screen.findByRole('radio', {name: 'Amigos'})).toBeChecked();
});
```

If the existing file already mocks `next/navigation` with a fixed object, merge the `useSearchParams` entry into that mock rather than adding a second `vi.mock` for the same module. If `FilterGroup` renders its options as buttons rather than radios, assert on `aria-pressed`/`aria-current` as that component does — check `ui/src/components/FilterGroup.tsx` and match it.

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/app/people/page.test.tsx`
Expected: FAIL — Amigos is selected in both cases.

- [x] **Step 3: Seed the tab from the query string**

In `ui/src/app/people/page.tsx`, import `useSearchParams` from `next/navigation` and replace the tab state:

```tsx
const params = useSearchParams();
// The unread badge links here with ?tab=activity: that is the only tab that
// marks inbox events read.
const [tab, setTab] = useState<PeopleTab>(() => {
  const requested = params.get('tab');
  return TABS.some(option => option.value === requested) ? requested as PeopleTab : 'friends';
});
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd ui && npx vitest run src/app/people/page.test.tsx`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add ui/src/app/people/page.tsx ui/src/app/people/page.test.tsx
git commit -m "feat(ui): open the people tab named in the query string"
```

---

## Task 3: Notifications can carry actions

**Files:**
- Modify: `ui/src/lib/notify.ts`
- Modify: `ui/src/components/Notifier.tsx`
- Test: `ui/src/components/Notifier.test.tsx`

**Interfaces:**
- Produces: `NotificationAction = {label: string; run: () => void | Promise<void>}` and `pushNotification(message: string, variant?: NotificationVariant, actions?: NotificationAction[]): void`, both exported from `@/lib/notify`.

- [x] **Step 1: Write the failing test**

Append to `ui/src/components/Notifier.test.tsx`:

```tsx
it('runs a notification action and dismisses the toast', async () => {
  const run = vi.fn();
  render(<Notifier/>);
  act(() => pushNotification('Convite de mesa', 'info', [{label: 'Entrar', run}]));
  await userEvent.click(await screen.findByRole('button', {name: 'Entrar'}));
  expect(run).toHaveBeenCalledOnce();
  await waitFor(() => expect(screen.queryByText('Convite de mesa')).not.toBeInTheDocument());
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/components/Notifier.test.tsx`
Expected: FAIL — `pushNotification` takes two arguments and no button renders.

- [x] **Step 3: Add actions to the notification model**

In `ui/src/lib/notify.ts`:

```ts
export interface NotificationAction {
  label: string
  run: () => void | Promise<void>
}

export interface AppNotification {
  id: string
  message: string
  variant: NotificationVariant
  // Optional inline buttons. An ignored toast still auto-dismisses on the
  // normal timer: every action here also exists on a durable surface.
  actions?: NotificationAction[]
}

export function pushNotification(message: string, variant: NotificationVariant = 'error',
                                 actions?: NotificationAction[]): void {
  const now = Date.now();
  if (now - (recent.get(message) || 0) < DEDUPE_MS) return;
  recent.set(message, now);
  const id = `${now}-${nextID++}`;
  items = [...items, {id, message, variant, actions}].slice(-MAX_VISIBLE);
  listeners.forEach(f => f(items));
  setTimeout(() => dismissNotification(id), AUTO_DISMISS_MS);
}
```

- [x] **Step 4: Render the actions**

In `ui/src/components/Notifier.tsx`, inside the toast, after the `<p>`:

```tsx
{n.actions?.length ? <div className="api-toast-actions">
  {n.actions.map(action => <button key={action.label} type="button" onClick={async () => {
    dismissNotification(n.id);
    await action.run();
  }}>{action.label}</button>)}
</div> : null}
```

- [x] **Step 5: Style the row**

In the stylesheet that already defines `.api-toast` (find it with `grep -rn "api-toast" ui/src --include=*.css`), add:

```css
.api-toast-actions {
  display: flex;
  gap: 0.5rem;
}

.api-toast-actions button {
  border: 1px solid currentColor;
  border-radius: 0.5rem;
  padding: 0.25rem 0.75rem;
  font: inherit;
  cursor: pointer;
}
```

- [x] **Step 6: Run test to verify it passes**

Run: `cd ui && npx vitest run src/components/Notifier.test.tsx`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add ui/src/lib/notify.ts ui/src/components/Notifier.tsx ui/src/components/Notifier.test.tsx ui/src/**/*.css
git commit -m "feat(ui): allow toasts to carry inline actions"
```

---

## Task 4: Answer a table invite from its toast

**Files:**
- Modify: `ui/src/lib/hooks/useLobbyRealtime.ts`
- Test: `ui/src/lib/hooks/useLobbyRealtime.test.ts` (create it if the repo has no test for this hook — check with `ls ui/src/lib/hooks/`)

**Interfaces:**
- Consumes: `pushNotification(message, variant, actions)` from Task 3; `acceptTableInvite(eventId)` / `declineTableInvite(eventId)` from `@/lib/api/social`.

- [x] **Step 1: Write the failing test**

```ts
import {renderHook} from '@testing-library/react';
import {acceptTableInvite} from '@/lib/api/social';
import {pushNotification} from '@/lib/notify';

vi.mock('@/lib/api/social', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api/social')>()),
  acceptTableInvite: vi.fn().mockResolvedValue(undefined),
  declineTableInvite: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('@/lib/notify', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/notify')>()),
  pushNotification: vi.fn(),
}));

it('offers accept and decline on a table invite push', async () => {
  const push = vi.mocked(pushNotification);
  // receiveForTest is the exported message handler; if useLobbyRealtime keeps
  // it internal, export it as `export function receiveLobbyMessage(...)` and
  // call it from the hook, so it can be driven directly here.
  receiveLobbyMessage({
    type: 'social_event',
    social_event: {event_id: 'ev-1', type: 'table_invite', actor_id: 'p2', room_id: 'room-9'},
  }, deps);
  const actions = push.mock.calls.at(-1)![2]!;
  expect(actions.map(a => a.label)).toEqual(['Entrar', 'Recusar']);
  await actions[0].run();
  expect(acceptTableInvite).toHaveBeenCalledWith('ev-1');
});
```

If extracting a testable handler is more churn than it is worth in this file, assert the same behavior through the existing lobby page test instead — the requirement is that a `table_invite` push produces a two-action toast whose first action calls `acceptTableInvite` with the event id.

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/lib/hooks/useLobbyRealtime.test.ts`
Expected: FAIL — the toast is pushed with two arguments.

- [x] **Step 3: Build the invite toast**

In `ui/src/lib/hooks/useLobbyRealtime.ts`, replace the `social_event` branch:

```ts
} else if (message.type === 'social_event') {
  void queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
  const event = message.social_event;
  const eventType = event?.type as SocialEventType | undefined;
  const copy = eventType ? SOCIAL_EVENT_COPY[eventType] || 'Nova atividade em Pessoas.'
    : 'Nova atividade em Pessoas.';
  // Accepting or declining from here also marks the inbox event read (both
  // server handlers call notifyUnread), so the badge clears without opening
  // the list. Everything else the server revalidates on accept: expiry,
  // friendship, room status and capacity.
  if (eventType === 'table_invite' && event?.event_id && event.room_id) {
    const eventId = event.event_id;
    const roomId = event.room_id;
    pushNotification(copy, 'info', [
      {
        label: 'Entrar', run: async () => {
          await acceptTableInvite(eventId);
          await queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
          router.push(`/table?id=${roomId}`);
        }
      },
      {
        label: 'Recusar', run: async () => {
          await declineTableInvite(eventId);
          await queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
        }
      },
    ]);
  } else {
    pushNotification(copy, 'info', [
      {label: 'Ver atividades', run: () => router.push('/people?tab=activity')},
    ]);
  }
}
```

Add `import {useRouter} from 'next/navigation';` and `const router = useRouter();` inside the hook (before the `receive` callback), extend `receive`'s dependency array with `router`, and import `acceptTableInvite` / `declineTableInvite` from `@/lib/api/social`.

- [x] **Step 4: Run tests to verify they pass**

Run: `cd ui && npx vitest run src/lib/hooks src/app/lobby`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add ui/src/lib/hooks/useLobbyRealtime.ts ui/src/lib/hooks/useLobbyRealtime.test.ts
git commit -m "feat(ui): accept or decline a table invite from its toast"
```

---

## Task 5: `TablePublic` opt-in on the player profile

**Files:**
- Modify: `api/internal/player/model.go:30`, `api/internal/player/store.go:334-351`, `api/internal/player/service.go:246-268`
- Modify: `api/internal/api/v1/player.go:29`, `:230-252`, `:420-432`
- Test: `api/internal/player/service_test.go`

**Interfaces:**
- Produces: `player.PlayerProfile.TablePublic bool`; `Service.SetShowcase(ctx, userID string, public, playstylePublic, tablePublic bool, featured []string)`; PATCH `/v1.0/players/me` accepts `table_public`; `GET` responses include `"table_public"`.

Note the signature change: `SetShowcase` gains a third bool. Update both the service and the store, and every existing caller and test (`grep -rn "SetShowcase" api/`).

- [x] **Step 1: Write the failing test**

Append to `api/internal/player/service_test.go`:

```go
func TestSetShowcaseStoresTablePublic(t *testing.T) {
	svc, _ := newTestService(t) // reuse whatever constructor the file already uses
	ctx := context.Background()
	if _, err := svc.SetShowcase(ctx, "u1", true, false, true, nil); err != nil {
		t.Fatal(err)
	}
	profile, err := svc.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !profile.TablePublic {
		t.Fatal("expected table_public to persist")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/player/ -run TestSetShowcaseStoresTablePublic`
Expected: FAIL — compile error, `SetShowcase` takes five arguments and `TablePublic` is undefined.

- [x] **Step 3: Add the field and thread it through**

`api/internal/player/model.go`, after `PlaystylePublic`:

```go
	// TablePublic lets friends see which PUBLIC room this player is sitting
	// in, so they can join it. Off by default: presence is otherwise
	// room-blind by design (see presence/model.go), and a private room is
	// never exposed even with this on.
	TablePublic bool `dynamodbav:"table_public,omitempty" json:"table_public"`
```

`api/internal/player/store.go`, in `SetShowcase`: take `tablePublic bool` after `playstylePublic` and add `"table_public": tablePublic` to the update map.

`api/internal/player/service.go`, in `SetShowcase`: take `tablePublic bool` after `playstylePublic` and pass it to `s.store.SetShowcase`.

- [x] **Step 4: Wire the HTTP layer**

`api/internal/api/v1/player.go`: add `TablePublic *bool \`json:"table_public"\`` to the PATCH request struct next to `PlaystylePublic`; extend the `if` guard to `req.ShowcasePublic != nil || req.PlaystylePublic != nil || req.TablePublic != nil || req.FeaturedAchievements != nil`; read `tablePublic := current.TablePublic`, override when `req.TablePublic != nil`, and pass it to `SetShowcase`. Add `"table_public": profile.TablePublic,` to `playerResponse`.

- [x] **Step 5: Run tests to verify they pass**

Run: `cd api && go build ./... && go test ./internal/player/ ./internal/api/v1/`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add api/internal/player api/internal/api/v1/player.go
git commit -m "feat(api): add the table_public profile opt-in"
```

---

## Task 6: Presence carries the room id

**Files:**
- Modify: `api/internal/presence/model.go`, `store.go`, `memory.go`, `valkey.go`, `service.go`
- Modify: `api/internal/sessionlog/store.go:201-222`
- Modify: `api/internal/buyin/service.go:83`, `:135`, `:345`, `:761`
- Test: `api/internal/presence/service_test.go`

**Interfaces:**
- Produces:
  - `presence.PlayerPresence{PlayerID string; Status Status; RoomID string}`
  - `Store.SetInTable(ctx context.Context, playerID, roomID string) (changed bool, err error)` — empty `roomID` means "not in a table"
  - `Store.GetMany(ctx, playerIDs []string) (map[string]PlayerPresence, error)`
  - `Service.GetMany` with the same new return type
  - `Service.SetInTable(ctx, playerID, roomID string) error`
  - `sessionlog.Store.FindLatestOpenSession(ctx, playerID) (tableID string, err error)` — empty means no open session
- Consumes: nothing from earlier tasks.

- [x] **Step 1: Write the failing test**

Append to `api/internal/presence/service_test.go` (and change `fakeSessions` to the new shape at the top of the file):

```go
type fakeSessions string

func (f fakeSessions) FindLatestOpenSession(context.Context, string) (string, error) {
	return string(f), nil
}

func TestRoomIDSurvivesSetAndReconcile(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fakeFriends{}, fakeSessions("room-9"), func(context.Context, string, string, Status) {})
	ctx := context.Background()
	if err := svc.Open(ctx, "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetMany(ctx, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["p1"].Status != StatusInTable || got["p1"].RoomID != "room-9" {
		t.Fatalf("want in_table at room-9, got %+v", got["p1"])
	}
	if err := svc.SetInTable(ctx, "p1", ""); err != nil {
		t.Fatal(err)
	}
	got, err = svc.GetMany(ctx, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["p1"].Status != StatusOnline || got["p1"].RoomID != "" {
		t.Fatalf("want online with no room, got %+v", got["p1"])
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/presence/ -run TestRoomIDSurvivesSetAndReconcile`
Expected: FAIL — compile error, `GetMany` returns `map[string]Status`.

- [x] **Step 3: Update the model and the interfaces**

`api/internal/presence/model.go` — replace the package comment's absolute claim and extend the struct:

```go
// Package presence defines ephemeral friend-visible availability. A room
// identifier is carried only for players who opted in (player.TablePublic)
// and is exposed only by the social API, which additionally requires the room
// to be public and joinable — see api/v1/social.go.
package presence

type PlayerPresence struct {
	PlayerID string
	Status   Status
	// RoomID is the room this player is sitting in, or "" when it is unknown
	// (offline, not in a table, or a pre-existing key written before rooms
	// were tracked). Never published without the gates in api/v1/social.go.
	RoomID string
}
```

`store.go`:

```go
	SetInTable(ctx context.Context, playerID, roomID string) (changed bool, err error)
	GetMany(ctx context.Context, playerIDs []string) (map[string]PlayerPresence, error)
```

`SessionSource` in `service.go`:

```go
type SessionSource interface {
	FindLatestOpenSession(ctx context.Context, playerID string) (string, error)
}
```

- [x] **Step 4: Update the memory store**

In `memory.go`, change `inTable map[string]bool` to `inTable map[string]string`, and:

```go
func (s *MemoryStore) SetInTable(_ context.Context, playerID, roomID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.inTable[playerID] != roomID
	if roomID == "" {
		delete(s.inTable, playerID)
	} else {
		s.inTable[playerID] = roomID
	}
	return changed, nil
}

func (s *MemoryStore) GetMany(_ context.Context, playerIDs []string) (map[string]PlayerPresence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]PlayerPresence, len(playerIDs))
	for _, playerID := range playerIDs {
		entry := PlayerPresence{PlayerID: playerID, Status: StatusOffline}
		if s.activeLocked(playerID) > 0 {
			entry.Status = StatusOnline
			if room := s.inTable[playerID]; room != "" {
				entry.Status, entry.RoomID = StatusInTable, room
			}
		}
		result[playerID] = entry
	}
	return result, nil
}
```

Update `NewMemoryStore` to `make(map[string]string)`.

- [x] **Step 5: Update the Valkey store**

In `valkey.go`, the table key now holds the room id instead of `'1'`:

```go
const setTableStateScript = `
local before = redis.call('GET', KEYS[1])
if ARGV[1] == '' then
  redis.call('DEL', KEYS[1])
else
  redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
end
local after = redis.call('GET', KEYS[1])
if before == after then return 0 end
return 1`

// readStatusScript returns "<status>|<room id>". A key written before rooms
// were tracked holds '1'; it still reads as in_table, just with no room, so
// it produces no join button and expires on its own TTL.
const readStatusScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
  return 'offline|'
end
local room = redis.call('GET', KEYS[2])
if room then
  if room == '1' then return 'in_table|' end
  return 'in_table|' .. room
end
return 'online|'`
```

```go
func (s *ValkeyStore) SetInTable(ctx context.Context, playerID, roomID string) (bool, error) {
	changed, err := s.client.Do(ctx, s.client.B().Eval().Script(setTableStateScript).Numkeys(1).
		Key(tableKey(playerID)).Arg(roomID, strconv.FormatInt(int64(tableStateTTL.Seconds()), 10)).Build()).ToInt64()
	return changed == 1, err
}

func (s *ValkeyStore) GetMany(ctx context.Context, playerIDs []string) (map[string]PlayerPresence, error) {
	result := make(map[string]PlayerPresence, len(playerIDs))
	commands := make([]valkey.Completed, 0, len(playerIDs))
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	for _, playerID := range playerIDs {
		commands = append(commands, s.client.B().Eval().Script(readStatusScript).Numkeys(2).
			Key(connectionKey(playerID), tableKey(playerID)).Arg(now).Build())
	}
	for i, response := range s.client.DoMulti(ctx, commands...) {
		raw, err := response.ToString()
		if err != nil {
			return nil, fmt.Errorf("presence: read %s: %w", playerIDs[i], err)
		}
		status, room, _ := strings.Cut(raw, "|")
		result[playerIDs[i]] = PlayerPresence{PlayerID: playerIDs[i], Status: Status(status), RoomID: room}
	}
	return result, nil
}
```

Add `"strings"` to the imports.

- [x] **Step 6: Update the service**

In `service.go`:

```go
func (s *Service) SetInTable(ctx context.Context, playerID, roomID string) error {
	changed, err := s.store.SetInTable(ctx, playerID, roomID)
	if err == nil && changed {
		entries, statusErr := s.store.GetMany(ctx, []string{playerID})
		if statusErr != nil {
			return statusErr
		}
		if entries[playerID].Status != StatusOffline {
			s.broadcast(ctx, playerID, entries[playerID].Status)
		}
	}
	return err
}

func (s *Service) Reconcile(ctx context.Context, playerID string) error {
	if s.sessions == nil {
		return nil
	}
	roomID, err := s.sessions.FindLatestOpenSession(ctx, playerID)
	if err != nil {
		return err
	}
	return s.SetInTable(ctx, playerID, roomID)
}

func (s *Service) GetMany(ctx context.Context, playerIDs []string) (map[string]PlayerPresence, error) {
	return s.store.GetMany(ctx, playerIDs)
}
```

In `Open`, replace the reconciliation block:

```go
	if s.sessions != nil {
		if roomID, sessionErr := s.sessions.FindLatestOpenSession(ctx, playerID); sessionErr != nil {
			slog.Warn("presence: session reconciliation failed", "player", playerID, "err", sessionErr)
		} else if _, setErr := s.store.SetInTable(ctx, playerID, roomID); setErr != nil {
			return setErr
		}
	}
```

In `broadcastCurrent`, read `entries[playerID].Status`.

- [x] **Step 7: Update sessionlog and buyin**

`api/internal/sessionlog/store.go`:

```go
// FindLatestOpenSession returns the table id of the player's newest unclosed
// session, or "" when there is none. It reconciles friend-visible in_table
// presence after a process restart or a WebSocket reconnect; whether the id is
// ever published is decided by api/v1/social.go's gates, not here.
func (s *Store) FindLatestOpenSession(ctx context.Context, playerID string) (string, error) {
```

Inside, return `item.TableID, nil` on the hit, `"", nil` when exhausted, and `"", err` on failure.

`api/internal/buyin/service.go`: change both `Reconcile`/`SetInTable` interface declarations (lines ~83 and ~135) to `SetInTable(context.Context, string, string) error`, call `s.presence.SetInTable(ctx, playerID, roomID)` at line ~345, and leave the `Reconcile` call at ~761 as is (it now restores the room id by itself).

- [x] **Step 8: Fix remaining callers and run the suite**

Run: `cd api && go build ./... && go test ./...`
Expected: PASS. Compile errors point at every other caller of `SetInTable`, `GetMany` or `FindLatestOpenSession` — fix each to the new signature (`presence.Status` reads become `.Status`).

- [x] **Step 9: Commit**

```bash
git add api/internal/presence api/internal/sessionlog/store.go api/internal/buyin/service.go
git commit -m "feat(api): carry the room id through presence"
```

---

## Task 7: `/social/friends` publishes a joinable room id

**Files:**
- Modify: `api/internal/api/v1/social.go:53-70`, `:86-100`, `hydrate`
- Modify: `api/internal/api/v1/router.go:128-135`
- Test: `api/internal/api/v1/social_test.go` (or the existing social handler test file — find it with `ls api/internal/api/v1/ | grep social`)

**Interfaces:**
- Consumes: `presence.PlayerPresence.RoomID` (Task 6), `player.PlayerProfile.TablePublic` (Task 5).
- Produces: `socialPlayerResponse.RoomID string \`json:"room_id,omitempty"\`` on `GET /v1.0/social/friends`.

- [x] **Step 1: Write the failing test**

```go
func TestFriendsRoomIDGates(t *testing.T) {
	cases := []struct {
		name       string
		tablePublic bool
		room       *roomstore.Room
		wantRoomID string
	}{
		{"opted out", false, &roomstore.Room{ID: "r1", Visibility: "public", Status: "waiting", MaxSeats: 6, SeatsTaken: 2}, ""},
		{"private room", true, &roomstore.Room{ID: "r1", Visibility: "private", Status: "waiting", MaxSeats: 6, SeatsTaken: 2}, ""},
		{"closed room", true, &roomstore.Room{ID: "r1", Visibility: "public", Status: "closed", MaxSeats: 6, SeatsTaken: 2}, ""},
		{"full room", true, &roomstore.Room{ID: "r1", Visibility: "public", Status: "waiting", MaxSeats: 6, SeatsTaken: 6}, ""},
		{"missing room", true, nil, ""},
		{"joinable", true, &roomstore.Room{ID: "r1", Visibility: "public", Status: "active", MaxSeats: 6, SeatsTaken: 2}, "r1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build the handler the way the existing tests in this file do,
			// with: a friend edge for "friend-1"; a profile for "friend-1"
			// whose TablePublic is tc.tablePublic; presence returning
			// {Status: in_table, RoomID: "r1"}; and a room store returning
			// tc.room for "r1".
			body := getFriends(t, handler)
			if body.Items[0].RoomID != tc.wantRoomID {
				t.Fatalf("want room_id %q, got %q", tc.wantRoomID, body.Items[0].RoomID)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v1/ -run TestFriendsRoomIDGates`
Expected: FAIL — `socialPlayerResponse` has no `RoomID`.

- [x] **Step 3: Accept a room store in the social handlers**

In `api/internal/api/v1/social.go`, add `rooms *roomstore.Store` to `socialHandlers`, and in `RegisterSocial`'s `extras` switch add:

```go
		case *roomstore.Store:
			roomsStore = value
```

declaring `var roomsStore *roomstore.Store` alongside the other extras and passing it into the struct literal. In `router.go`, add `rooms` to the `RegisterSocial` call's trailing arguments: `}, presenceSvc, recentSvc, rooms)`.

- [x] **Step 4: Add the field and the gates**

Add to `socialPlayerResponse`:

```go
	// RoomID is present only for a friend who opted in (player.TablePublic)
	// and is sitting in a joinable PUBLIC room. Every other case omits it —
	// see joinableRoomIDs.
	RoomID string `json:"room_id,omitempty"`
```

Add the helper:

```go
// joinableRoomIDs resolves which of the given presences may be published as a
// joinable room. All five gates must hold; any failure (including a room read
// error) drops the id silently, because a missing join button is always the
// safe direction.
func (h *socialHandlers) joinableRoomIDs(ctx context.Context, presences map[string]presence.PlayerPresence,
	profiles map[string]player.PlayerProfile) map[string]string {
	if h.rooms == nil {
		return nil
	}
	wanted := make(map[string]string) // playerID -> roomID
	rooms := make(map[string]bool)    // distinct room ids to read
	for playerID, entry := range presences {
		profile, ok := profiles[playerID]
		if !ok || !profile.TablePublic || entry.Status != presence.StatusInTable || entry.RoomID == "" {
			continue
		}
		wanted[playerID] = entry.RoomID
		rooms[entry.RoomID] = true
	}
	joinable := make(map[string]bool, len(rooms))
	for roomID := range rooms {
		room, err := h.rooms.Get(ctx, roomID)
		if err != nil {
			slog.Warn("social: room lookup for friend presence failed", "room", roomID, "err", err)
			continue
		}
		joinable[roomID] = room != nil && room.Visibility == "public" &&
			(room.Status == "waiting" || room.Status == "active") && room.SeatsTaken < room.MaxSeats
	}
	result := make(map[string]string, len(wanted))
	for playerID, roomID := range wanted {
		if joinable[roomID] {
			result[playerID] = roomID
		}
	}
	return result
}
```

In `hydrate`, after presence is loaded:

```go
	statuses := map[string]presence.PlayerPresence{}
	var joinable map[string]string
	if includePresence && h.presence != nil {
		statuses, err = h.presence.GetMany(c.Context(), ids)
		if err != nil {
			return nil, err
		}
		joinable = h.joinableRoomIDs(c.Context(), statuses, profiles)
	}
```

and in the per-edge loop:

```go
		if includePresence && h.presence != nil {
			status := statuses[edges[i].OtherPlayerID].Status
			response.Presence = &status
			response.RoomID = joinable[edges[i].OtherPlayerID]
		}
```

- [x] **Step 5: Run tests to verify they pass**

Run: `cd api && go test ./internal/api/v1/`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add api/internal/api/v1/social.go api/internal/api/v1/router.go api/internal/api/v1/social_test.go
git commit -m "feat(api): publish a joinable room id for opted-in friends"
```

---

## Task 8: Join button and the opt-in toggle

**Files:**
- Modify: `ui/src/lib/api/social.ts` (`SocialPlayer`), `ui/src/lib/api/player.ts` (`PlayerProfile`, update body)
- Modify: `ui/src/components/social/PeopleList.tsx`
- Modify: `ui/src/components/lobby/ProfileShowcaseDialog.tsx`
- Modify: `ui/src/app/people/page.tsx` (header copy), `ui/src/lib/social.ts` (presence comment)
- Modify: `ui/src/dev/mockRuntime.ts` (`table_public` on the mock profile, `room_id` on a mock friend)
- Test: `ui/src/components/social/socialComponents.test.tsx`

**Interfaces:**
- Consumes: `room_id` on a friend row (Task 7), `table_public` on the profile (Task 5).

- [x] **Step 1: Write the failing test**

Append to `ui/src/components/social/socialComponents.test.tsx`:

```tsx
it('offers to join a friend at a joinable public table', () => {
  renderPeopleList('friends', [{
    player_id: 'p2', name: 'Ana', relationship: 'friend', muted: false, blocked: false,
    presence: 'in_table', room_id: 'room-9',
  }]);
  expect(screen.getByRole('link', {name: /Entrar na mesa/})).toHaveAttribute('href', '/table?id=room-9');
});

it('does not offer to join without a room id', () => {
  renderPeopleList('friends', [{
    player_id: 'p2', name: 'Ana', relationship: 'friend', muted: false, blocked: false, presence: 'in_table',
  }]);
  expect(screen.queryByRole('link', {name: /Entrar na mesa/})).not.toBeInTheDocument();
});
```

Use whatever render helper the file already provides for `PeopleList`; if there is none, render `<PeopleList variant="friends" items={items} actions={{run: vi.fn(), pending: null}}/>` directly.

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/components/social/socialComponents.test.tsx`
Expected: FAIL — no such link, and `room_id` is not on the type.

- [x] **Step 3: Extend the types**

`ui/src/lib/api/social.ts`, in `SocialPlayer`:

```ts
  /** Present only for a friend who opted in and is sitting at a joinable
   * public table. The join flow revalidates everything; this is a shortcut. */
  room_id?: string;
```

`ui/src/lib/api/player.ts`: add `table_public: boolean;` to `PlayerProfile` and `table_public?: boolean;` to the update body type.

- [x] **Step 4: Render the join link**

In `ui/src/components/social/PeopleList.tsx`, inside the `friends` variant's action area:

```tsx
{player.room_id && <Link href={`/table?id=${player.room_id}`} className="social-actions-item">
  Entrar na mesa
</Link>}
```

Import `Link` from `next/link` if it is not imported yet.

- [x] **Step 5: Add the profile toggle**

In `ui/src/components/lobby/ProfileShowcaseDialog.tsx`'s `ShowcaseEditor`, add `const [isTablePublic, setIsTablePublic] = useState(me.table_public);`, include `table_public: isTablePublic` in the `updateMe` payload, add `me.table_public` to the remount `key` on line ~103, and add a privacy row next to the existing ones:

```tsx
<div className="showcase-privacy-row">
  <span><b>Mesa visível para amigos</b><small>Amigos podem entrar na sua mesa quando ela for pública.</small></span>
  <Switch checked={isTablePublic} onCheckedChange={setIsTablePublic} aria-label="Mesa visível para amigos"/>
</div>
```

- [x] **Step 6: Update the copy that promised the opposite**

`ui/src/app/people/page.tsx`, the `AppPageHeader` description:

```
description="A amizade é sempre mútua. A presença aparece só entre amigos, e sua mesa só fica visível se você ativar isso no perfil — mesas privadas nunca aparecem."
```

`ui/src/lib/social.ts`, above `presenceLabel`:

```ts
// Presence never carries blinds or balance, and never a private table. A
// public table shows up only for a friend who turned that on in their profile,
// and it travels as a separate room_id field — never inside the status label.
```

- [x] **Step 7: Update the mock runtime**

In `ui/src/dev/mockRuntime.ts`: add `table_public: false` to `mockProfile`, accept it in the PATCH handler (`if (typeof body.table_public === 'boolean') mockProfile.table_public = body.table_public;`), and give one mocked friend a `room_id` so the button can be exercised by hand.

- [x] **Step 8: Run tests to verify they pass**

Run: `cd ui && npx vitest run` and `cd ui && npx tsc --noEmit`
Expected: PASS

- [x] **Step 9: Commit**

```bash
git add ui/src
git commit -m "feat(ui): join a friend at their public table"
```

---

## Task 9: Capture consumed time-bank milliseconds

**Files:**
- Modify: `api/internal/tablestore/store.go:33-51`
- Modify: `api/internal/table/actor.go:999`, `:1180-1195`, `:1927`
- Test: `api/internal/table/actor_test.go` (or the existing time-bank test file — `api/internal/engine/hand/timebank_test.go` covers the engine; the actor-level test belongs with the actor)

**Interfaces:**
- Produces: `tablestore.ActionLogEntry.TimeBankMs int64` and `func (a *Actor) consumeTimeBank(playerID string) int64` returning the milliseconds charged (0 when nothing was).

- [x] **Step 1: Write the failing test**

```go
func TestConsumeTimeBankReturnsChargedMillis(t *testing.T) {
	// Build an actor the way the neighbouring actor tests do, with the time
	// bank enabled and a turn timeout of at least five seconds.
	a := newTestActor(t)
	a.turnDeadlineFor = "p1"
	a.turnBaseDeadline = timeNowFunc().Add(-2 * time.Second)

	charged := a.consumeTimeBank("p1")
	if charged < 1900 || charged > 2100 {
		t.Fatalf("want roughly 2000ms charged, got %d", charged)
	}

	a.turnBaseDeadline = timeNowFunc().Add(2 * time.Second)
	if again := a.consumeTimeBank("p1"); again != 0 {
		t.Fatalf("want 0 charged inside the deadline, got %d", again)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/table/ -run TestConsumeTimeBankReturnsChargedMillis`
Expected: FAIL — `consumeTimeBank` returns nothing.

- [x] **Step 3: Return the charge**

`api/internal/table/actor.go`:

```go
// consumeTimeBank charges only the part of a decision made after the normal
// room clock expired, and returns the milliseconds charged (0 when nothing
// was) so the caller can record them on the action-log entry it is about to
// commit. The total deadline and the durable balance are committed in the
// same conditionally-written table state, so a losing multi-server attempt is
// discarded and recomputed after reload.
func (a *Actor) consumeTimeBank(playerID string) int64 {
	if !a.timeBankEnabled || a.turnTimeout < 5*time.Second || playerID == "" || playerID != a.turnDeadlineFor || a.turnBaseDeadline.IsZero() {
		return 0
	}
	elapsed := timeNowFunc().Sub(a.turnBaseDeadline).Milliseconds()
	if elapsed <= 0 {
		return 0
	}
	before := a.cached.TimeBankForActor(playerID)
	after := a.cached.ConsumeTimeBankForActor(playerID, elapsed)
	slog.Info("table time bank consumed",
		"table", a.id, "hand", a.handID, "stage", a.cached.ViewFor("").Stage,
		"turn_player", a.turnDeadlineFor, "charged_player", playerID,
		"bank_before_ms", before, "bank_elapsed_ms", elapsed, "bank_after_ms", after,
		"base_deadline_unix_ms", a.turnBaseDeadline.UnixMilli(),
		"action_deadline_unix_ms", a.turnDeadline.UnixMilli())
	// The bank can run out mid-decision: charge what was actually deducted,
	// never the raw elapsed time.
	return before - after
}
```

- [x] **Step 4: Persist it on the entry**

`api/internal/tablestore/store.go`, in `ActionLogEntry`:

```go
	// TimeBankMs is the time-bank milliseconds this action consumed. Written
	// by Actor.consumeTimeBank; read once per hand by app.go's onHandComplete
	// to award the no_rush achievement. Older rows omit it and read as zero.
	TimeBankMs int64 `dynamodbav:"time_bank_ms,omitempty"`
```

In `actor.go` at the act call site (~line 999), replace `a.consumeTimeBank(c.PlayerID)` with `timeBankMs := a.consumeTimeBank(c.PlayerID)` and add `TimeBankMs: timeBankMs,` to the `entry` literal.

At the disconnect sit-out call site (~line 1927):

```go
		timeBankMs := a.consumeTimeBank(c.PlayerID)
		a.cached.SitOutForActor(c.PlayerID)
		if err := a.commit(ctx, "", &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, Action: "disconnect_sit_out", TimeBankMs: timeBankMs,
		}); err != nil {
```

- [x] **Step 5: Run tests to verify they pass**

Run: `cd api && go test ./internal/table/ ./internal/tablestore/`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add api/internal/table/actor.go api/internal/tablestore/store.go api/internal/table/actor_test.go
git commit -m "feat(api): record consumed time-bank millis on the action log"
```

---

## Task 10: Award the "Sem pressa" achievement

**Files:**
- Modify: `api/internal/achievements/catalog.go:18-54`, `:150-160`
- Modify: `api/internal/achievements/service.go:95-103`, `:117-160`
- Modify: `api/internal/app/app.go:530-556`
- Test: `api/internal/achievements/service_test.go`

**Interfaces:**
- Consumes: `tablestore.ActionLogEntry.TimeBankMs` (Task 9).
- Produces: `achievements.KeyNoRush = "no_rush"`; `achievements.HandMetric.TimeBankMs int64`.

- [x] **Step 1: Write the failing test**

Append to `api/internal/achievements/service_test.go`:

```go
func TestNoRushAwardsOnFirstMinute(t *testing.T) {
	svc, _ := newTestService(t) // reuse the constructor the file already uses
	outcome := hand.HandOutcome{Participants: []string{"p1"}, Payouts: map[string]int64{}}
	unlocks, err := svc.RecordHand(context.Background(), "t1", "sandbox", outcome,
		[]HandMetric{{PlayerID: "p1", TimeBankMs: 60_000}})
	if err != nil {
		t.Fatal(err)
	}
	var stars int
	for _, unlock := range unlocks {
		if unlock.Key == KeyNoRush {
			stars = unlock.Stars
		}
	}
	if stars != 1 {
		t.Fatalf("want one star for no_rush, got %d", stars)
	}
}

func TestNoRushIgnoresZero(t *testing.T) {
	svc, _ := newTestService(t)
	outcome := hand.HandOutcome{Participants: []string{"p1"}, Payouts: map[string]int64{}}
	unlocks, err := svc.RecordHand(context.Background(), "t1", "sandbox", outcome,
		[]HandMetric{{PlayerID: "p1", TimeBankMs: 0}})
	if err != nil {
		t.Fatal(err)
	}
	for _, unlock := range unlocks {
		if unlock.Key == KeyNoRush {
			t.Fatal("no_rush must not unlock without consumed time")
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/achievements/ -run TestNoRush`
Expected: FAIL — `KeyNoRush` and `HandMetric.TimeBankMs` are undefined.

- [x] **Step 3: Add the catalog entry**

`api/internal/achievements/catalog.go` — add to the key block:

```go
	KeyNoRush = "no_rush"
```

and to `Catalog`:

```go
	// Thresholds are MILLISECONDS of consumed time bank: 1 minute, 1 hour,
	// 1 day, 1 week, 30 days. The frontend renders them as those durations.
	{Key: KeyNoRush, Metric: "time_bank_ms_consumed", Tiers: []Tier{
		{1, 60_000}, {2, 3_600_000}, {3, 86_400_000}, {4, 604_800_000}, {5, 2_592_000_000}}},
```

- [x] **Step 4: Award it**

`api/internal/achievements/service.go` — add to `HandMetric`:

```go
	// TimeBankMs is this player's time-bank milliseconds consumed during the
	// hand, summed from the action log by app.go's onHandComplete. Drives
	// KeyNoRush below. Zero means either "decided in time" or "no action log
	// to read", and both correctly award nothing.
	TimeBankMs int64
```

In `RecordHand`, right after the `handsTotals` loop:

```go
	if len(metricSets) > 0 {
		for _, metric := range metricSets[0] {
			if metric.TimeBankMs <= 0 {
				continue
			}
			if err := bumpBy(metric.PlayerID, KeyNoRush, int(metric.TimeBankMs)); err != nil {
				return nil, err
			}
		}
	}
```

- [x] **Step 5: Sum the milliseconds in the hand hook**

`api/internal/app/app.go`, in `onHandComplete`, next to the `peeked` loop:

```go
		// Time bank is charged per action, so one hand can carry several
		// charges for the same player.
		timeBankMs := make(map[string]int64)
		for _, entry := range actions {
			if entry.Action == "peek_cards" {
				peeked[entry.PlayerID] = true
			}
			timeBankMs[entry.PlayerID] += entry.TimeBankMs
		}
```

(delete the old single-purpose `peeked` loop) and extend the metric mapping:

```go
			achievementMetrics[i] = achievements.HandMetric{
				PlayerID:   metric.PlayerID,
				VPIP:       metric.VPIP,
				ThreeBet:   metric.ThreeBet,
				Peeked:     peeked[metric.PlayerID],
				TimeBankMs: timeBankMs[metric.PlayerID],
			}
```

- [x] **Step 6: Run tests to verify they pass**

Run: `cd api && go build ./... && go test ./internal/achievements/ ./internal/app/`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add api/internal/achievements api/internal/app/app.go
git commit -m "feat(api): award the no_rush achievement for consumed time bank"
```

---

## Task 11: Present "Sem pressa" as durations

**Files:**
- Modify: `ui/src/lib/utils.ts` (`ACHIEVEMENT_LABELS`)
- Modify: `ui/src/lib/achievements.ts` (description, example, new formatter)
- Modify: `ui/src/components/achievements/AchievementCard.tsx`
- Modify: `ui/src/app/achievements/page.tsx:142`
- Test: `ui/src/components/achievements/AchievementCard.test.tsx`

**Interfaces:**
- Consumes: the `no_rush` catalog entry from Task 10.
- Produces: `achievementValueFormat(key: string): (value: number) => string` exported from `@/lib/achievements`.

- [x] **Step 1: Write the failing test**

Append to `ui/src/components/achievements/AchievementCard.test.tsx`:

```tsx
const noRush = {
  key: 'no_rush',
  metric: 'time_bank_ms_consumed',
  tiers: [
    {stars: 1, threshold: 60_000}, {stars: 2, threshold: 3_600_000}, {stars: 3, threshold: 86_400_000},
    {stars: 4, threshold: 604_800_000}, {stars: 5, threshold: 2_592_000_000},
  ],
};

it('labels no_rush tiers as durations', () => {
  render(<AchievementCard achievement={noRush}/>);
  expect(screen.getByLabelText('Nível 1: 1 minuto')).toBeInTheDocument();
  expect(screen.getByLabelText('Nível 3: 1 dia')).toBeInTheDocument();
  expect(screen.getByLabelText('Nível 5: 1 mês')).toBeInTheDocument();
});

it('still renders plain counts for other achievements', () => {
  render(<AchievementCard achievement={{key: 'wins', metric: 'hand_won', tiers: [{stars: 1, threshold: 1000}]}}/>);
  expect(screen.getByLabelText('Nível 1: 1.000')).toBeInTheDocument();
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd ui && npx vitest run src/components/achievements/AchievementCard.test.tsx`
Expected: FAIL — the label reads `Nível 1: 60.000`.

- [x] **Step 3: Add the label, description and example**

`ui/src/lib/utils.ts`, in `ACHIEVEMENT_LABELS`: `no_rush: "Sem pressa",`

`ui/src/lib/achievements.ts`, in `DESCRIPTIONS`:

```ts
  no_rush: 'Deixou o relógio correr e usou seu tempo extra para decidir.',
```

and in `EXAMPLES`: `no_rush: ['QS', 'JS'],`

- [x] **Step 4: Add the formatter**

Append to `ui/src/lib/achievements.ts`:

```ts
// Most achievements count events; no_rush counts milliseconds, so its raw
// thresholds ("2.592.000.000") are unreadable. The unit lives here rather
// than in the catalog because it is purely a presentation concern.
const DURATION_UNITS: {ms: number; one: string; many: string}[] = [
  {ms: 2_592_000_000, one: 'mês', many: 'meses'},
  {ms: 604_800_000, one: 'semana', many: 'semanas'},
  {ms: 86_400_000, one: 'dia', many: 'dias'},
  {ms: 3_600_000, one: 'hora', many: 'horas'},
  {ms: 60_000, one: 'minuto', many: 'minutos'},
];

function formatDurationMs(value: number): string {
  for (const unit of DURATION_UNITS) {
    if (value >= unit.ms) {
      const count = Math.floor(value / unit.ms);
      return `${count.toLocaleString('pt-BR')} ${count === 1 ? unit.one : unit.many}`;
    }
  }
  const seconds = Math.max(0, Math.floor(value / 1000));
  return `${seconds.toLocaleString('pt-BR')} ${seconds === 1 ? 'segundo' : 'segundos'}`;
}

const DURATION_KEYS = new Set(['no_rush']);

/** How this achievement's counts and thresholds are written out. */
export function achievementValueFormat(key: string): (value: number) => string {
  return DURATION_KEYS.has(key) ? formatDurationMs : value => value.toLocaleString('pt-BR');
}
```

- [x] **Step 5: Route the card's four renders through it**

In `ui/src/components/achievements/AchievementCard.tsx`, import `achievementValueFormat`, add `const formatValue = achievementValueFormat(achievement.key);` next to `const example = …`, and replace each `…toLocaleString('pt-BR')` on a **count or threshold** with `formatValue(…)`:

- the stars container's `aria-label`: `` `${progress.starsFilled} de 5 estrelas, ${formatValue(progress.count)} registrados` ``
- each star's `aria-label`: `` `Nível ${tier.stars}: ${formatValue(tier.threshold)}` ``
- the tooltip: `` `${formatValue(progress.count)}/${formatValue(previewTier.threshold)}` `` and `formatValue(previewTier.threshold)`
- the progress `<strong>`: `` `${formatValue(progress.count)}/${formatValue(progress.nextTier!.threshold)}` ``
- the locked ladder: `achievement.tiers.map(t => formatValue(t.threshold)).join(' · ')`

Leave `progress.starsFilled`/`achievement.tiers.length` and the progress-bar math alone — they are counts of stars and percentages, not metric values.

- [x] **Step 6: Fix the "faltam" line**

`ui/src/app/achievements/page.tsx` line ~142:

```tsx
<small>Faltam {achievementValueFormat(nextMilestone.item.key)(nextMilestone.progress.nextTier!.threshold - nextMilestone.progress.count)} para o nível {nextMilestone.progress.nextTier!.stars}.</small>
```

Import `achievementValueFormat` and use whatever the surrounding code calls the catalog item that `nextMilestone` was built from (check the `nextMilestone` construction around line 60-70 and use its actual property name for the achievement key).

- [x] **Step 7: Run tests to verify they pass**

Run: `cd ui && npx vitest run src/components/achievements src/app/achievements && npx tsc --noEmit`
Expected: PASS

- [x] **Step 8: Commit**

```bash
git add ui/src/lib/utils.ts ui/src/lib/achievements.ts ui/src/components/achievements ui/src/app/achievements
git commit -m "feat(ui): render no_rush thresholds as durations"
```

---

## Task 12: Full verification and documentation

**Files:**
- Modify: `docs/specs/2026-08-27-social-ux-and-no-rush-achievement.md` (status line)
- Modify: `api/CLAUDE.md` and/or `ui/CLAUDE.md` if either documents presence's room-blindness or the achievement catalog

- [x] **Step 1: Run everything**

Run:
```bash
cd api && go build ./... && go vet ./... && go test ./...
cd ../ui && npx tsc --noEmit && npx vitest run && npm run lint
```
Expected: all PASS

- [x] **Step 2: Check the docs that state the old guarantee**

Run: `grep -rn "nunca revela\|room-blind\|No table or room identifier" --include=*.md --include=*.go .`
Update every hit that now overstates the guarantee: presence still never reveals a private table, and a public one only with the player's opt-in.

- [x] **Step 3: Mark the spec implemented**

Change the spec's Status section to `**Implemented 2026-08-27.**` plus a line for anything that shipped differently from the design.

- [x] **Step 4: Commit**

```bash
git add docs api/CLAUDE.md ui/CLAUDE.md
git commit -m "docs: mark the social UX and no_rush spec implemented"
```

---

## Self-review notes

- Spec §1 → Tasks 1–2. §2 → Tasks 3–4. §3 → Tasks 5–8. §4 → Tasks 9–11. Verification → Task 12.
- Signature changes that ripple: `presence.Store.SetInTable`/`GetMany` and `sessionlog.FindLatestOpenSession` (Task 6, step 8 sweeps the callers), and `player.Service.SetShowcase`/`Store.SetShowcase` (Task 5).
- Names used consistently across tasks: `useSocialUnread`, `NotificationAction`, `PlayerPresence.RoomID`, `TablePublic`/`table_public`, `joinableRoomIDs`, `KeyNoRush`/`no_rush`, `HandMetric.TimeBankMs`, `ActionLogEntry.TimeBankMs`, `achievementValueFormat`.

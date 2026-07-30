# Sandbox credit purchases (PIX via ctech-wallet) — design

Date: 2026-07-30

## Goal

Let a poker player buy sandbox credits with real PIX money, using ctech-wallet's
existing (mostly M2M) sandbox-purchase endpoints. Poker tracks its own copy of
purchase history (so users can see it and request refunds), receives a wallet
webhook on status change, and pushes a live update over the existing general
websocket so the frontend can refresh balance without polling.

## Architecture

```
Poker FE (/store)          Poker BE (Go)                Wallet BE
     |                          |                            |
     |--GET skus-------------->|--M2M GET /skus----------->|  (NEW wallet route)
     |<--pack list--------------|<--catalog-------------------|
     |--POST purchase{sku}---->|--M2M POST purchase-------->|
     |<--pix qr+copia+expiry----|<--purchase+pix+expires_at---|  (NEW field)
     |  [poll GET :id every 5s while modal open]
     |                          |
     |                          |<====== webhook (HMAC) =====|  wallet notifies on status change
     |                          |--M2M GET purchase (reverify, never trust webhook body)
     |                          |  update row, ws broadcast "user#"+player_id
     |<==== ws sandbox_purchase_update ====|
     |--invalidate balance + purchases queries, toast
```

Two repos touched: `ctech-wallet` (small additive endpoint changes only),
`ctech-poker` (new package, new table, new routes, ws message, frontend page).

## Gaps found in ctech-wallet, closed as small additive changes

1. No M2M route to list SKUs — only an internal `wallet.ListSKUs()` Go func.
   Adding `GET /v1.0/wallet/sandbox-purchase/skus` (same scope group as the
   existing sandbox-purchase routes).
2. No `expires_at` in the purchase response — only an internal TTL used for
   the sweep, never surfaced. The frontend needs to show a countdown, so
   adding `expires_at` (RFC3339) to `m2mPurchaseSandbox` / `m2mGetSandboxPurchase`
   responses, computed from the existing `sandboxPurchaseTTLMinutes`.

Both are additive, no behavior change to existing callers.

## ctech-wallet changes

- `GET /v1.0/wallet/sandbox-purchase/skus` (M2M, `ScopeWalletSandboxPurchase`) → `wallet.ListSKUs()`.
- Add `expires_at` to the purchase DTOs in `api/internal/api/v1/dto.go`, sourced
  from the pending-until time already computed in `services/sandbox_purchase.go`.

## ctech-poker backend (Go)

- `internal/walletclient`: new methods `ListSandboxSKUs`, `PurchaseSandbox`,
  `GetSandboxPurchase`, `RefundSandboxPurchase`. New `TokenManager` scoped to
  `internal:wallet:sandbox-purchase` (grant already added to `PokerClientID`).
  Same breaker/retry wrapper as existing methods (`client.go:207-250`).
- New package `internal/sandboxpurchase`: service + DynamoDB store, mirrors
  `internal/dailyreward/store.go` (dynamo.Base, conditional put for idempotent
  create, `UpdateItem` on webhook).
- New table `poker_sandbox_purchases`: pk `player_id`, sk `purchase_id`
  (= wallet's purchase_id). Fields: `sku`, `price_cents`, `base_credits`,
  `bonus_percent`, `total_credits`, `status`, `pix_copia_e_cola`,
  `qr_code_base64`, `expires_at`, `created_at`, `updated_at`. No TTL — this is
  purchase history, not ephemeral state.
- Routes (session-authenticated, under `/v1.0/wallet/sandbox-purchase`):
  - `GET /skus` — proxy to wallet, no cache (low traffic, not worth it).
  - `POST /` `{sku}` — poker mints an idempotency key (uuid), calls wallet,
    stores a pending row, returns the pix payload.
  - `GET /` — list mine, DynamoDB query by `player_id`.
  - `GET /:id` — re-GET wallet (source of truth), refresh local row, return.
    Used by the frontend's safety-net poll.
  - `POST /:id/refund` — direct passthrough to wallet, update local row with
    the result.
- Webhook: `POST /v1.0/webhooks/wallet`. Verifies `X-Wallet-Signature`
  (HMAC-SHA256, `hmac.Equal`) against `WALLET_WEBHOOK_HMAC_SECRET` (new env
  var; value generated with `openssl rand -hex 32`, same value registered in
  wallet's SSM M2M-clients param). Never trusts the payload for money: re-GETs
  the purchase, updates the row conditionally (idempotent against wallet's
  retries), broadcasts ws on status change, ACKs 200 only after a successful
  reverify (5xx on failure so wallet retries).
- Proto (`proto/poker.proto`): add one field `purchase_id` to `ServerMessage`
  (field 19). New `type = "sandbox_purchase_update"` reuses existing
  `player_id` (target), `amount` (credits_granted), `code` (status:
  confirmed/refunded/expired/failed) — no other new fields needed.
- CDK: add the table to `dynamodb-stack.ts`; wire `WALLET_WEBHOOK_HMAC_SECRET`
  the same way `PokerClientSecret` is wired (SSM secure string → task env).

## ctech-poker frontend (Next.js)

Build via `/impeccable` once this architecture is implemented.

- `lib/api/wallet.ts`: `listSkus`, `createPurchase`, `listPurchases`,
  `getPurchase`, `refundPurchase`.
- `lib/api/dailyReward.ts` (new — no frontend exists yet for the already-live
  backend feature): `getCooldown`, `spin` against `/v1.0/sandbox-credits/`.
- New route `app/store/page.tsx` (flat, matches existing `app/achievements`,
  `app/leaderboard` convention). Two tabs via inline `FilterGroup` (mirrors
  `achievements/page.tsx:128-141` — no new wrapper component, this pair isn't
  reused elsewhere):
  - **"Recompensas"** — daily reward widget: cooldown timer, spin button.
  - **"Compras"** — SKU grid + purchase history list, refund button where
    `status === 'confirmed'`.
- Purchase modal: QR (`qr_code_base64`), copy-to-clipboard copia-e-cola,
  countdown from `expires_at`. Safety-net poll every 5s on `GET /:id` while
  the modal is open — websocket is the primary path, poll only covers a
  missed/dropped ws frame. (ponytail: fixed-interval poll, upgrade to
  backoff/visibility-aware polling if it matters later.)
- `useLobbyRealtime.ts`: new branch for `type === 'sandbox_purchase_update'`
  → invalidate `['wallet','balance']` + new `['wallet','sandbox-purchases']`
  query, toast per status. Mirrors the existing `payment_received` branch
  (`useLobbyRealtime.ts:59-61`) exactly.

## Error handling

- Wallet unreachable on create → existing breaker trips, 503 to frontend, no
  local row written.
- Webhook HMAC mismatch → 401, logged, dropped.
- Webhook replay → conditional update no-ops if status unchanged, still 200.
- Refund rejected by wallet (already debited) → passthrough error surfaces as
  a toast.
- Missed webhook entirely → covered by the 5s safety-net poll while the modal
  is open, and by `GET /:id` re-verify whenever the store/history page opens.

## Testing

- Go: `walletclient` new methods — fake-HTTP-server tests mirroring
  `client_test.go` / `gamewallet_test.go`. `sandboxpurchase` store — mirrors
  `dailyreward` store tests. Webhook handler — valid/invalid HMAC, replay
  idempotency, broadcast-on-confirm (fake registry).
- Frontend: `useLobbyRealtime.test.tsx` — add `sandbox_purchase_update` case
  (mirrors the existing `payment_received` test at line 77). Basic render test
  for `app/store/page.tsx`.

## Manual provisioning (outside this change, tracked as a blocker)

- Poker's OAuth client grant for `internal:wallet:sandbox-purchase` — already
  done.
- Poker's `client_id` + webhook URL + HMAC secret registered in wallet's SSM
  M2M-clients param — pending, user will do this later.

# Poker owns the games' rules and sales (P4)

Two independent moves, each safe on its own, each with its own rollback.

1. **Responsible gambling** (consent, personal limits, self-exclusion) lives in Poker (`internal/responsible`, table `poker_responsible`). The real-money buy-in enforces it. Real money is off in production (`/ctech/prod/poker/real-money-enabled` absent), so nothing changes for players until it is turned on, which also needs the legal sign-off reference.
2. **Sales** of chip packs and premium goods go through Billing invoices (`internal/purchasegw`) instead of ctech-wallet's Pix sale. Switched by `PURCHASES_VIA` (`wallet` default, `billing`).

## Prerequisites (done 2026-10-15 in production)

- Wallet routes `/internal/wallet/holds*` and `/internal/wallet/responsible/export` deployed (ctech-wallet #118).
- Scopes `internal:wallet:hold`, `internal:wallet:responsible-export` in ctech-account's catalogue (`seedscopes`), and granted with `internal:account:kyc` to Poker's M2M client (`createclient -grant`).
- Poker's Billing client `poker-billing` created (`createclient`, scopes `billing:customers:read|write billing:invoices:read|write`); `client_id`/secret in SSM `/ctech/prod/poker/billing-client-id|secret`, plus `billing-url` and `billing-event-secret` (random, SecureString).
- Billing tenant (`api/tenants/ctech.json`): owner `poker`, credential `poker-billing`, endpoint `whe_poker` (`invoice.paid` to `https://poker.internal.aoctech.app/v1.0/webhooks/billing`), applied with `api/cmd/seed/run.sh prod`.

## 1. Responsible gambling

1. Deploy Poker (creates `poker_responsible`; the CDK deploy runs first).
2. `scripts/import-responsible.sh prod --dry-run`: counts users and carried exclusions. Compare with the Wallet export (the numbers must match what `GET /v1.0/internal/wallet/responsible/export` returns).
3. `scripts/import-responsible.sh prod`: writes. Repeatable; a player already present is left alone, so a second run changes nothing, and nothing a player changed in Poker since is overwritten.
4. Wallet still checks consent and exclusion on its own holds, so both sides agree while real money is off. Before turning real money on: re-run the import once more (state may have changed in the Wallet UI meanwhile) and then stop using the Wallet's pages for it.
5. Rollback: nothing to undo while real money is off. The import only adds rows.

Account deletion: an active self-exclusion is carried (keyed by the CPF keyed hash, `SELFEXCL#<hash>` rows in `poker_responsible`) before the player's rows are erased. If ctech-account cannot be reached the purge step fails and retries; it never erases an exclusion it could not carry.

## 2. Sales through Billing

1. Deploy Poker with `PURCHASES_VIA=wallet` (the default): no Billing call is made, the Billing webhook route is not mounted.
2. Non-prod (when a stage exists): set `PURCHASES_VIA` to `billing` in `cdk/lib/constants.ts`, deploy, buy the R$ 1,00 pack with Inter's simulated payment, see the chips credited and a deck unlocked.
3. **Production, owner first (live money).** Set `prod: 'billing'` in `cdk/lib/constants.ts`, deploy; the owner buys the smallest pack and one good. Check: the invoice is `PAID` in Billing, the Sandbox balance rose by the pack's credits once, the `invoice.paid` delivery shows `delivered` in Billing's console.
4. Leave both rails' history for a week; watch for `ALARM poker sale` log lines (a paid invoice that does not match the catalogue price) in `/ctech-poker/prod/app`.
5. Rollback: `prod: 'wallet'`. Sales opened through Billing and still unpaid are paid at Billing and credited by the next read of the purchase; do not switch back while one is in flight (list open Poker invoices in Billing first).

Buying needs a CPF on the player's KYC (Billing opens a Pix only for a customer with one). A player without it gets `422 kyc-required`.

A refund of a chip pack works while the player still has the credits (the wallet debit is refused otherwise and no money moves); the wallet's stricter "no debit since the credit" rule has no equivalent through the generic debit route.

## What this costs

No new infrastructure beyond one DynamoDB table (on demand) and a handful of SSM parameters.

# Runbook — account deletion (poker participant)

## One-time enablement (per environment)
1. Deploy ctech-account's stack first (it creates `<env>-account-user-erasure` and its SSM ARN), then poker's.
2. In ctech-account, create a dedicated confidential client for poker's acks, grant it only
   `internal:account:erasure-ack`, and store its id/secret in SSM `/ctech/<env>/poker/erasure-ack-client-id` and
   `/ctech/<env>/poker/erasure-ack-client-secret` (SecureString). Instance refresh to load them.
3. Add poker to ctech-account's `/ctech-account/<env>/erasure-participants` JSON:
   `{"service":"poker","url":"https://poker-api[-<env>].aoctech.app/v1.0","audience":"<poker SERVICE_AUDIENCE>","client_id":"<ERASURE_ACK_CLIENT_ID>"}`.
4. Check poker's `ERASURE_ACCOUNT_CLIENT_ID` equals ctech-account's `SELF_CLIENT_ID` (default `accounts`).

## DLQ alarm (`<env>-poker-user-erasure-dlq-depth`)
A message failed 5 times. Read the app log for `erasure: message left for redelivery` with its error, fix the cause,
then redrive the DLQ to the queue (`aws sqs start-message-move-task --source-arn <dlq-arn> --profile ctech`). The
purge is idempotent: redriving is always safe.

## Restoring a DynamoDB table from PITR/backup (mandatory, saga protocol §8)
A restore brings erased users back. After restoring any `<env>_poker_*` table, list purged requests from
ctech-account (see its README "Account deletion" restore runbook) whose `purged_at` is after the restore point, and
have ctech-account re-publish `user.erase` for each (admin redrive). Do not restore `<env>_poker_erasure_state` to an
older point: it would drop tombstones.

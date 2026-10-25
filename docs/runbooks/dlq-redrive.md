# DLQ redrive: poker

Queues (CDK; `{env}` = `prod`):

| DLQ | Fed by | Source in CDK | Redrive target |
|---|---|---|---|
| `{env}-poker-user-erasure-dlq` | `{env}-poker-user-erasure` after 5 receives | `cdk/lib/api-stack.ts` | back to the source queue (default) |
| `{env}-poker-reconcile-dlq` | EventBridge Scheduler `{env}-poker-reconcile` (reconcile Lambda) | `cdk/lib/reconcile-stack.ts` | none: no source queue; reconcile is idempotent and runs every few minutes, so inspect and purge |
| `{env}-poker-tablecleanup-dlq` | Scheduler `{env}-poker-tablecleanup` | `cdk/lib/tablecleanup-stack.ts` | same as reconcile |
| `{env}-poker-action-log-archiver-dlq` | DynamoDB stream records the archiver Lambda failed 3 times | `cdk/lib/archiver-stack.ts` | none: records carry the stream metadata, not a queue payload; replay by invoking the archiver or reading the action log table |

## Alert

None fires on DLQ depth: the family declined CloudWatch alarms (poker #427). A failed erasure purge alerts to `ctech-{env}-alerts` (`erasure-purge-failed`, each redelivery) and also shows as a deletion in `purging` in ctech-account. The `reconcile` and `tablecleanup` Lambdas alert on a quarantined cash-out or a failed stale-table refund (`poker-reconcile`, `poker-tablecleanup-refund`, `poker-tablecleanup-settlement`); for the archiver DLQ, read the depth after a Lambda error in its log group.

## Inspect

```bash
export AWS_PROFILE=ctech AWS_REGION=us-east-1
Q=$(aws sqs get-queue-url --queue-name <dlq-name> --query QueueUrl --output text)
aws sqs get-queue-attributes --queue-url "$Q" --attribute-names ApproximateNumberOfMessages
aws sqs receive-message --queue-url "$Q" --max-number-of-messages 10 --visibility-timeout 0 \
  --message-attribute-names All
```

`--visibility-timeout 0` peeks without hiding the message. Messages are kept 14 days; read the cause in the
service log group (CloudWatch Logs) before redriving, or the message will just come back.

## Redrive

Fix the cause first, then move the messages back to their source queue:

```bash
aws sqs start-message-move-task --source-arn arn:aws:sqs:us-east-1:868899309401:<dlq-name>
aws sqs list-message-move-tasks --source-arn arn:aws:sqs:us-east-1:868899309401:<dlq-name>
```

Without `--destination-arn` the messages go back to the queue that dead-lettered them. Use `--max-number-of-messages-per-second`
to throttle. Cancel with `aws sqs cancel-message-move-task --task-handle <handle>`.
For a DLQ filled by something that is not a queue (Scheduler, Lambda stream failure) there is no source queue:
pass `--destination-arn` of a queue that can take the payload, or replay by hand and `aws sqs purge-queue` after.

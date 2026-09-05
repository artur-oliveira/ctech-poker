import * as cdk from 'aws-cdk-lib';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import * as cloudwatchActions from 'aws-cdk-lib/aws-cloudwatch-actions';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as sns from 'aws-cdk-lib/aws-sns';
import * as sqs from 'aws-cdk-lib/aws-sqs';
import {Construct} from 'constructs';
import {ALERTS_TOPIC_ARN} from './constants';

/**
 * Wire a scheduled/stream Lambda and its dead-letter queue to the existing
 * shared alert topic (`ctech-prod-alerts`, email subscription already
 * provisioned out of band — we import it, never create a topic or a
 * subscription here). Two alarms per Lambda, both notifying on ALARM and OK:
 *
 *  - DLQ depth: `ApproximateNumberOfMessagesVisible >= 1` — any message in the
 *    DLQ is a poison record / failed invocation nobody would otherwise see
 *    before the 14-day retention drops it.
 *  - Lambda `Errors >= 1` — a failing invocation, before it exhausts retries
 *    and reaches the DLQ.
 *
 * Standard-resolution alarms on AWS-emitted metrics: a few R$/month total, no
 * new billable service (issue #30 hard constraint).
 *
 * `enabled` is the `cloudwatchAlarmsEnabled` flag threaded down from
 * `bin/poker.ts` — a no-op cost lever for turning every alarm this app
 * creates off without touching the Lambda/DLQ resources themselves.
 */
export function addLambdaDlqAlarms(
  scope: Construct,
  idPrefix: string,
  fn: lambda.IFunction,
  dlq: sqs.IQueue,
  enabled: boolean,
): void {
  if (!enabled) return;
  const alertsTopic = sns.Topic.fromTopicArn(scope, `${idPrefix}AlertsTopic`, ALERTS_TOPIC_ARN);
  const action = new cloudwatchActions.SnsAction(alertsTopic);

  const wire = (alarm: cloudwatch.Alarm) => {
    alarm.addAlarmAction(action);
    alarm.addOkAction(action);
  };

  wire(new cloudwatch.Alarm(scope, `${idPrefix}DlqDepthAlarm`, {
    alarmName: `${dlq.queueName}-messages-visible`,
    alarmDescription: `${idPrefix}: message(s) in the dead-letter queue — a dropped/poison record needs manual triage.`,
    metric: dlq.metricApproximateNumberOfMessagesVisible({
      period: cdk.Duration.minutes(5),
      statistic: cloudwatch.Stats.MAXIMUM,
    }),
    threshold: 1,
    evaluationPeriods: 1,
    datapointsToAlarm: 1,
    comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
    treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
  }));

  wire(new cloudwatch.Alarm(scope, `${idPrefix}ErrorsAlarm`, {
    alarmName: `${fn.functionName}-errors`,
    alarmDescription: `${idPrefix}: the Lambda raised an error in the last 5 minutes.`,
    metric: fn.metricErrors({
      period: cdk.Duration.minutes(5),
      statistic: cloudwatch.Stats.SUM,
    }),
    threshold: 1,
    evaluationPeriods: 1,
    datapointsToAlarm: 1,
    comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
    treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
  }));
}

/** CTechPoker is internal/metrics's defaultNamespace (METRICS_NAMESPACE is
 * never set for this app, so it's also the actual one). Environment is
 * always a dimension there — see internal/metrics's package doc. */
const METRICS_NAMESPACE = 'CTechPoker';

/**
 * Alarms sourced from the app's own EMF metrics (`internal/metrics`,
 * `internal/app/handpipeline.go`), not from a native DynamoDB/Lambda metric.
 * Issue #290's acceptance criterion: a hand-pipeline budget violation must be
 * alarmable from a runtime metric, not only from `TestHandPipelineDynamoBudget`
 * pinning the ceiling in CI.
 *
 * `HandPipelineDuration` is measured from dispatch (queueing included) against
 * `handPipelineTimeout` (30s, `internal/app/handpipeline.go`); p95 sustained
 * above 80% of that budget for 15 minutes is a real, load-driven slowdown —
 * transient GC pauses or a single slow hand do not sustain a p95 for three
 * 5-minute periods. `treatMissingData` is NOT_BREACHING: a quiet period with
 * no completed hands emits no datapoint and must not page.
 */
export function addHandPipelineBudgetAlarm(
  scope: Construct,
  environment: string,
  enabled: boolean,
): void {
  if (!enabled) return;
  const alertsTopic = sns.Topic.fromTopicArn(scope, 'HandPipelineBudgetAlertsTopic', ALERTS_TOPIC_ARN);
  const action = new cloudwatchActions.SnsAction(alertsTopic);

  const durationBudgetMs = 30_000;
  const alarm = new cloudwatch.Alarm(scope, 'HandPipelineDurationBudgetAlarm', {
    alarmName: `${environment}-hand-pipeline-duration-budget`,
    alarmDescription:
      'p95 post-hand gamification pipeline duration is approaching handPipelineTimeout — ' +
      'see api/internal/app/handpipeline.go and issue #204/#290.',
    metric: new cloudwatch.Metric({
      namespace: METRICS_NAMESPACE,
      metricName: 'HandPipelineDuration',
      dimensionsMap: {Environment: environment},
      statistic: 'p95',
      period: cdk.Duration.minutes(5),
    }),
    threshold: durationBudgetMs * 0.8,
    evaluationPeriods: 3,
    datapointsToAlarm: 3,
    comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
    treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
  });
  alarm.addAlarmAction(action);
  alarm.addOkAction(action);
}

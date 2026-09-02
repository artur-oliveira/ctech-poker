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
 */
export function addLambdaDlqAlarms(
  scope: Construct,
  idPrefix: string,
  fn: lambda.IFunction,
  dlq: sqs.IQueue,
): void {
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

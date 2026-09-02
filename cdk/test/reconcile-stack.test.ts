import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {ReconcileStack} from '../lib/reconcile-stack';

function buildStack() {
  const app = new App();
  return Template.fromStack(new ReconcileStack(app, 'TestReconcileStack', {
    environment: 'dev',
    authDomainName: 'accounts-dev.aoctech.app',
    pendingCashoutsTableArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_pending_cashouts',
    walletUrlParam: '/ctech/dev/poker/wallet-url',
    pokerClientIdParam: '/ctech/dev/poker/client-id',
    pokerClientSecretParam: '/ctech/dev/poker/client-secret',
  }));
}

test('protects scheduled reconciliation with retry, DLQ and an errors alarm', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::Scheduler::Schedule', {
    Name: 'dev-ctech-poker-reconcile',
    ScheduleExpression: 'rate(5 minutes)',
    Target: Match.objectLike({
      DeadLetterConfig: {Arn: {'Fn::GetAtt': [Match.stringLikeRegexp('ReconcileDLQ'), 'Arn']}},
      RetryPolicy: {MaximumEventAgeInSeconds: 3600, MaximumRetryAttempts: 3},
    }),
  });
  template.hasResourceProperties('AWS::SQS::Queue', {
    QueueName: 'dev-ctech-poker-reconcile-dlq',
    MessageRetentionPeriod: 1209600,
  });
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: Match.objectLike({
      Statement: Match.arrayWith([Match.objectLike({
        Action: Match.arrayWith(['sqs:SendMessage']),
      })]),
    }),
  });
  // #30: DLQ-depth + Lambda-errors alarms, both notifying the existing
  // ctech-prod-alerts topic (imported, not created here).
  template.resourceCountIs('AWS::CloudWatch::Alarm', 2);
  template.resourceCountIs('AWS::SNS::Topic', 0);
  const alerts = 'arn:aws:sns:us-east-1:868899309401:ctech-prod-alerts';
  template.hasResourceProperties('AWS::CloudWatch::Alarm', {
    Namespace: 'AWS/SQS',
    MetricName: 'ApproximateNumberOfMessagesVisible',
    Threshold: 1,
    ComparisonOperator: 'GreaterThanOrEqualToThreshold',
    TreatMissingData: 'notBreaching',
    AlarmActions: [alerts],
    OKActions: [alerts],
  });
  template.hasResourceProperties('AWS::CloudWatch::Alarm', {
    Namespace: 'AWS/Lambda',
    MetricName: 'Errors',
    AlarmActions: [alerts],
    OKActions: [alerts],
  });
});

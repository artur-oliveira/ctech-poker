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

test('protects scheduled reconciliation with retry and a DLQ', () => {
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
  // Poker creates no CloudWatch alarms and no SNS topic.
  template.resourceCountIs('AWS::CloudWatch::Alarm', 0);
  template.resourceCountIs('AWS::SNS::Topic', 0);
});

// cmd/reconcile only Querys gsi_status and UpdateItems the row it resolved; there is no
// Scan in api/ at all. Keeps the grant from drifting back to table-wide reads (#56).
test('grants the reconcile Lambda no dynamodb:Scan', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: Match.objectLike({
      Statement: Match.arrayWith([
        Match.objectLike({Action: ['dynamodb:Query', 'dynamodb:UpdateItem']}),
      ]),
    }),
  });
  expect(JSON.stringify(template.toJSON())).not.toContain('dynamodb:Scan');
});

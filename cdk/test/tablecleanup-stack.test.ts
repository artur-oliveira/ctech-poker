import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {TableCleanupStack} from '../lib/tablecleanup-stack';

function buildStack() {
  const app = new App();
  const stack = new TableCleanupStack(app, 'TestTableCleanupStack', {
    environment: 'dev',
    tableStateArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_table_state',
    roomsTableArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_rooms',
    walletUrlParam: '/ctech/dev/poker/wallet-url',
    pokerClientIdParam: '/ctech/dev/poker/client-id',
    pokerClientSecretParam: '/ctech/dev/poker/client-secret',
  });
  return Template.fromStack(stack);
}

test('creates the tablecleanup Lambda on the provided.al2023 runtime', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::Lambda::Function', {
    FunctionName: 'dev-ctech-poker-tablecleanup',
    Runtime: 'provided.al2023',
    Handler: 'bootstrap',
  });
});

test('schedules the sweep every 30 minutes', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::Scheduler::Schedule', {
    Name: 'dev-ctech-poker-tablecleanup',
    ScheduleExpression: 'rate(30 minutes)',
    Target: Match.objectLike({
      DeadLetterConfig: {Arn: {'Fn::GetAtt': [Match.stringLikeRegexp('TableCleanupDLQ'), 'Arn']}},
      RetryPolicy: {MaximumEventAgeInSeconds: 7200, MaximumRetryAttempts: 3},
    }),
  });
  template.hasResourceProperties('AWS::SQS::Queue', {
    QueueName: 'dev-ctech-poker-tablecleanup-dlq',
    MessageRetentionPeriod: 1209600,
  });
  // All alarms removed (dlq-messages/throttles/missed-run 2026-08-17, the
  // Lambda-errors alarm 2026-08-19): unmonitored, no SNS subscriber, billed
  // past the CloudWatch free tier.
  template.resourceCountIs('AWS::CloudWatch::Alarm', 0);
});

test('the Lambda role can Query the table-state index but never Scan it', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: Match.objectLike({
      Statement: Match.arrayWith([
        Match.objectLike({
          // BatchGetItem backs tablestore's consistent-read load path.
          Action: Match.arrayWith(['dynamodb:Query', 'dynamodb:BatchGetItem']),
          Resource: Match.arrayWith([
            'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_table_state',
            'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_table_state/index/*',
          ]),
        }),
      ]),
    }),
  });
  const json = JSON.stringify(template.toJSON());
  expect(json).not.toContain('dynamodb:Scan');
});

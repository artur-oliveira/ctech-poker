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
    ScheduleExpression: 'rate(30 minutes)',
  });
});

test('the Lambda role can Query the table-state index but never Scan it', () => {
  const template = buildStack();
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: Match.objectLike({
      Statement: Match.arrayWith([
        Match.objectLike({
          Action: Match.arrayWith(['dynamodb:Query']),
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

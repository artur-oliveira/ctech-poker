import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {PushStack} from '../lib/push-stack';
import {DynamoDBStack} from '../lib/dynamodb-stack';

function buildStack() {
  const app = new App();
  const stack = new PushStack(app, 'TestPushStack', {
    environment: 'dev',
    pushTableArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_push',
    dailyRewardTableArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_daily_reward',
    sessionsTableArn: 'arn:aws:dynamodb:us-east-1:868899309401:table/dev_poker_player_sessions',
  });
  return Template.fromStack(stack);
}

function statements(template: Template, roleLogicalPrefix: string): any[] {
  const policies = template.findResources('AWS::IAM::Policy');
  return Object.values(policies)
    .filter((p: any) => (p.Properties.Roles ?? []).some((r: any) => JSON.stringify(r).includes(roleLogicalPrefix)))
    .flatMap((p: any) => p.Properties.PolicyDocument.Statement);
}

test('the push table has the endpoint and slot indexes, keys only, and a TTL, in the {env}_poker_ naming', () => {
  const stack = new DynamoDBStack(new App(), 'TestDynamo', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_push',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({
        IndexName: 'endpoint-index',
        KeySchema: [{AttributeName: 'endpoint_id', KeyType: 'HASH'}, {AttributeName: 'pk', KeyType: 'RANGE'}],
        Projection: {ProjectionType: 'KEYS_ONLY'},
      }),
      Match.objectLike({
        IndexName: 'slot-index',
        KeySchema: [{AttributeName: 'slot_key', KeyType: 'HASH'}, {AttributeName: 'kind_sub', KeyType: 'RANGE'}],
        Projection: {ProjectionType: 'KEYS_ONLY'},
      }),
    ]),
  });
});

test('the schedule runs every 15 minutes in São Paulo time', () => {
  buildStack().hasResourceProperties('AWS::Scheduler::Schedule', {
    Name: 'dev-ctech-poker-push-tick',
    ScheduleExpression: 'cron(0/15 * * * ? *)',
    ScheduleExpressionTimezone: 'America/Sao_Paulo',
    FlexibleTimeWindow: {Mode: 'OFF'},
  });
});

test('both Lambdas are provided.al2023 on arm64', () => {
  const t = buildStack();
  for (const name of ['dev-ctech-poker-push-tick', 'dev-ctech-poker-push-sender']) {
    t.hasResourceProperties('AWS::Lambda::Function', {
      FunctionName: name, Runtime: 'provided.al2023', Architectures: ['arm64'], Handler: 'bootstrap',
    });
  }
});

test('the sender can read devices and delete a gone one but never write a reminder or Scan', () => {
  const stmts = statements(buildStack(), 'PushSenderRole');
  const ddb = stmts.filter((s) => JSON.stringify(s.Action).includes('dynamodb'));
  const actions = ddb.flatMap((s) => (Array.isArray(s.Action) ? s.Action : [s.Action]));
  expect(actions).toEqual(expect.arrayContaining(['dynamodb:Query', 'dynamodb:DeleteItem']));
  expect(actions).not.toContain('dynamodb:PutItem');
  expect(actions).not.toContain('dynamodb:Scan');
});

test('the tick can invoke only the sender and reads the daily reward and sessions tables', () => {
  const stmts = statements(buildStack(), 'PushTickRole');
  const invoke = stmts.filter((s) => JSON.stringify(s.Action).includes('lambda:InvokeFunction'));
  expect(invoke).toHaveLength(1);
  expect(JSON.stringify(invoke[0].Resource)).toMatch(/PushSender[0-9A-F]+/); // the sender function's ARN, no wildcard
  const text = JSON.stringify(stmts);
  expect(text).toContain('dev_poker_daily_reward');
  expect(text).toContain('dev_poker_player_sessions');
  expect(text).not.toContain('dynamodb:Scan');
});

test('no KMS key, no SQS event source, no alarm is added (zero-idle cost policy)', () => {
  const t = buildStack();
  t.resourceCountIs('AWS::KMS::Key', 0);
  t.resourceCountIs('AWS::Lambda::EventSourceMapping', 0);
  t.resourceCountIs('AWS::CloudWatch::Alarm', 0);
});

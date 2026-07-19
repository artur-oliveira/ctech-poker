import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_table_state, poker_action_log, poker_action_guards tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  // dynamodb.TableV2 always synthesizes as AWS::DynamoDB::GlobalTable (even
  // with zero extra replicas) — not AWS::DynamoDB::Table.
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 3);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {TableName: 'dev_poker_table_state'});
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_action_log',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    StreamSpecification: {StreamViewType: 'NEW_IMAGE'},
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_action_guards',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_table_state, poker_action_log, poker_action_guards tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  // dynamodb.TableV2 always synthesizes as AWS::DynamoDB::GlobalTable (even
  // with zero extra replicas) — not AWS::DynamoDB::Table.
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 11);
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

test('creates gamification tables and hands-won leaderboard index', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack3', {environment: 'dev'});
  const template = Template.fromStack(stack);
  for (const name of ['poker_achievement_progress', 'poker_leaderboard_stats', 'poker_daily_reward']) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {TableName: `dev_${name}`});
  }
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_leaderboard_stats',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_hands_won'}),
      Match.objectLike({IndexName: 'gsi_hands_played'}),
      Match.objectLike({IndexName: 'gsi_win_rate'}),
    ]),
  });
});

test('creates poker_player_profiles table without secondary indexes', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPlayerProfilesStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_profiles',
  });
});

test('creates poker_rooms table with public and share-code GSIs', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack2', {environment: 'dev'});
  const template = Template.fromStack(stack);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_rooms',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_public'}),
      Match.objectLike({IndexName: 'gsi_share_code'}),
    ]),
  });
});

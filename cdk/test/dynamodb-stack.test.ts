import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_table_state, poker_action_log, poker_action_guards tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  // dynamodb.TableV2 always synthesizes as AWS::DynamoDB::GlobalTable (even
  // with zero extra replicas) — not AWS::DynamoDB::Table.
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 24);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_table_state',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({
        IndexName: 'gsi_active_last_action',
        KeySchema: Match.arrayWith([
          Match.objectLike({AttributeName: 'gsi_active', KeyType: 'HASH'}),
          Match.objectLike({AttributeName: 'last_action_at', KeyType: 'RANGE'}),
        ]),
      }),
    ]),
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_action_log',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    StreamSpecification: {StreamViewType: 'NEW_IMAGE'},
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_action_guards',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_table_state_history',
    KeySchema: Match.arrayWith([
      Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
      Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
    ]),
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

test('creates poker_player_profiles table with exact friend-code lookup', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPlayerProfilesStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_profiles',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_friend_code'}),
    ]),
  });
});

test('creates private player notes with a viewer/opponent composite key', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPlayerNotesStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_notes',
    KeySchema: Match.arrayWith([
      Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
      Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
    ]),
  });
});

test('creates expiring opaque hand shares', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestHandSharesStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_hand_shares',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates private poker stats with expiring hand guards', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPokerStatsStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_poker_stats',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('expires only resolved pending cashout records through the shared ttl attribute', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPendingCashoutsStack', {environment: 'dev'});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_pending_cashouts',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates poker_reaction_entitlements and poker_reaction_purchases tables with player/purchase composite keys', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestReactionPurchaseStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  for (const name of ['poker_reaction_entitlements', 'poker_reaction_purchases']) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
      TableName: `dev_${name}`,
      KeySchema: Match.arrayWith([
        Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
        Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
      ]),
    });
  }
});

test('creates poker_cosmetic_entitlements and poker_cosmetic_purchases tables with player/purchase composite keys', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestCosmeticPurchaseStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  for (const name of ['poker_cosmetic_entitlements', 'poker_cosmetic_purchases']) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
      TableName: `dev_${name}`,
      KeySchema: Match.arrayWith([
        Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
        Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
      ]),
    });
  }
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
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates social graph, recent players, inbox and reports storage', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestSocialStorageStack', {environment: 'dev'});
  const template = Template.fromStack(stack);

  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_social_edges',
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_recent_players',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_recent'}),
    ]),
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_social_events',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_inbox'}),
      Match.objectLike({IndexName: 'gsi_unread'}),
    ]),
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_reports',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_status'}),
    ]),
  });
});

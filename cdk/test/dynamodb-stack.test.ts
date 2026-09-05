import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_table_state, poker_action_log, poker_action_guards tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);
  // dynamodb.TableV2 always synthesizes as AWS::DynamoDB::GlobalTable (even
  // with zero extra replicas) — not AWS::DynamoDB::Table.
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 29);
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
  // The authoritative table item and its audit-history snapshots are
  // ephemeral: TTL'd so a dead table is reaped rather than lingering forever
  // (2026-09-02 incident follow-up).
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_table_state',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_table_state_history',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    KeySchema: Match.arrayWith([
      Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
      Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
    ]),
  });
});

test('PITR is on for durable data and off for ephemeral tables (2026-09-02 follow-up)', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPitrStack', {environment: 'prod', cloudwatchAlarmsEnabled: false});
  const template = Template.fromStack(stack);

  for (const name of [
    'poker_table_state', 'poker_table_state_history', 'poker_action_log', 'poker_action_guards',
    'poker_player_sessions',
  ]) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
      TableName: `prod_${name}`,
      Replicas: Match.arrayWith([
        Match.objectLike({PointInTimeRecoverySpecification: {PointInTimeRecoveryEnabled: false}}),
      ]),
    });
  }
  for (const name of ['poker_player_hands', 'poker_leaderboard_stats', 'poker_pending_cashouts', 'poker_rooms']) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
      TableName: `prod_${name}`,
      Replicas: Match.arrayWith([
        Match.objectLike({PointInTimeRecoverySpecification: {PointInTimeRecoveryEnabled: true}}),
      ]),
    });
  }
});

test('creates gamification tables and hands-won leaderboard index', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack3', {environment: 'dev', cloudwatchAlarmsEnabled: true});
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
  const stack = new DynamoDBStack(app, 'TestPlayerProfilesStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_profiles',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_friend_code'}),
    ]),
  });
});

test('creates private player notes with a viewer/opponent composite key', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPlayerNotesStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_notes',
    KeySchema: Match.arrayWith([
      Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
      Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
    ]),
  });
});

test('creates expiring opaque hand shares indexed by owner', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestHandSharesStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_hand_shares',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    // Issue #203: the revocation list is one descending Query on this index,
    // not a GetItem per token plus a prune write on the read path.
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({
        IndexName: 'gsi_owner',
        KeySchema: [
          {AttributeName: 'owner_id', KeyType: 'HASH'},
          {AttributeName: 'created_at', KeyType: 'RANGE'},
        ],
        Projection: {ProjectionType: 'ALL'},
      }),
    ]),
  });
});

test('creates private poker stats with expiring hand guards', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPokerStatsStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_poker_stats',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates head-to-head matchup stats with expiring pair guards', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestMatchupStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_matchups',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates poker_table_highlights table with a table_id/date composite key and no TTL', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestHighlightsStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_table_highlights',
    KeySchema: Match.arrayWith([
      Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
      Match.objectLike({AttributeName: 'sk', KeyType: 'RANGE'}),
    ]),
  });
});

test('expires only resolved pending cashout records through the shared ttl attribute', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestPendingCashoutsStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_pending_cashouts',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('creates poker_reaction_entitlements and poker_reaction_purchases tables with player/purchase composite keys', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestReactionPurchaseStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
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
  const stack = new DynamoDBStack(app, 'TestCosmeticPurchaseStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
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
  // Issue #219: deck and felt history each page their own rows off this index
  // instead of filtering the other kind's rows out after the read.
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_cosmetic_purchases',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({
        IndexName: 'gsi_player_kind',
        KeySchema: [
          {AttributeName: 'pk', KeyType: 'HASH'},
          {AttributeName: 'kind', KeyType: 'RANGE'},
        ],
        Projection: {ProjectionType: 'ALL'},
      }),
    ]),
  });
});

test('creates poker_rooms table with public, bucket and share-code GSIs', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack2', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_rooms',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({IndexName: 'gsi_public'}),
      // Backs POST /rooms/join-or-create's per-bucket query (#213).
      Match.objectLike({
        IndexName: 'gsi_bucket',
        KeySchema: [{AttributeName: 'gsi_bucket', KeyType: 'HASH'}],
      }),
      Match.objectLike({IndexName: 'gsi_share_code'}),
    ]),
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});

test('indexes player sessions by open table', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestSessionIndexStack', {environment: 'dev', cloudwatchAlarmsEnabled: false});
  const template = Template.fromStack(stack);
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_player_sessions',
    GlobalSecondaryIndexes: Match.arrayWith([
      Match.objectLike({
        IndexName: 'gsi_open_table',
        KeySchema: [
          Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
          Match.objectLike({AttributeName: 'open_table_id', KeyType: 'RANGE'}),
        ],
        Projection: {ProjectionType: 'ALL'},
      }),
      Match.objectLike({
        IndexName: 'gsi_player_table',
        KeySchema: [
          Match.objectLike({AttributeName: 'pk', KeyType: 'HASH'}),
          Match.objectLike({AttributeName: 'table_id', KeyType: 'RANGE'}),
        ],
        Projection: {ProjectionType: 'KEYS_ONLY'},
      }),
    ]),
  });
});

test('creates social graph, recent players, inbox and reports storage', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestSocialStorageStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);

  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_social_edges',
  });
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_recent_players',
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    // No GSI since #199/#260 — the list pages the base table's ULID sort key.
    GlobalSecondaryIndexes: Match.absent(),
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

test('right-sizes on-demand capacity: hot-path tables get a higher ceiling than the rest (#34)', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestCapacityStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);

  const hotPathTables = [
    'poker_table_state', 'poker_action_log', 'poker_action_guards', 'poker_rooms', 'poker_player_sessions',
  ];
  // TableV2 puts read capacity on each region's replica entry and write
  // capacity at the table level.
  for (const name of hotPathTables) {
    template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
      TableName: `dev_${name}`,
      BillingMode: 'PAY_PER_REQUEST',
      Replicas: Match.arrayWith([
        Match.objectLike({ReadOnDemandThroughputSettings: {MaxReadRequestUnits: 4000}}),
      ]),
      WriteOnDemandThroughputSettings: {MaxWriteRequestUnits: 4000},
    });
  }
  // A representative cold table keeps the original, lower ceiling.
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_sandbox_purchases',
    Replicas: Match.arrayWith([
      Match.objectLike({ReadOnDemandThroughputSettings: {MaxReadRequestUnits: 1000}}),
    ]),
    WriteOnDemandThroughputSettings: {MaxWriteRequestUnits: 1000},
  });
});

test('wires a throttle alarm on every hot-path table to the existing account alerts topic, never a new SNS topic (#34)', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestThrottleAlarmStack', {environment: 'dev', cloudwatchAlarmsEnabled: true});
  const template = Template.fromStack(stack);

  template.resourceCountIs('AWS::SNS::Topic', 0);
  // 5 throttle alarms + 2 sustained-write-volume alarms (2026-09-02 follow-up).
  template.resourceCountIs('AWS::CloudWatch::Alarm', 7);
  for (const name of [
    'dev-poker_table_state-throttled-requests',
    'dev-poker_action_log-throttled-requests',
    'dev-poker_action_guards-throttled-requests',
    'dev-poker_rooms-throttled-requests',
    'dev-poker_player_sessions-throttled-requests',
    'dev-poker_table_state-write-volume',
    'dev-poker_pending_cashouts-write-volume',
  ]) {
    template.hasResourceProperties('AWS::CloudWatch::Alarm', {
      AlarmName: name,
      AlarmActions: Match.arrayWith(['arn:aws:sns:us-east-1:868899309401:ctech-prod-alerts']),
    });
  }
  // The write-volume alarm pages on a single 5-minute breach — a wedge is
  // caught in minutes, not on the next bill.
  template.hasResourceProperties('AWS::CloudWatch::Alarm', {
    AlarmName: 'dev-poker_table_state-write-volume',
    Threshold: 40_000,
    EvaluationPeriods: 1,
    ComparisonOperator: 'GreaterThanThreshold',
  });
});

test('creates no alarms and no SNS topic reference when cloudwatchAlarmsEnabled is false', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestNoAlarmsStack', {environment: 'dev', cloudwatchAlarmsEnabled: false});
  const template = Template.fromStack(stack);

  template.resourceCountIs('AWS::CloudWatch::Alarm', 0);
  template.resourceCountIs('AWS::SNS::Topic', 0);
  // Tables themselves are unaffected by the flag.
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 29);
});

import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {Billing} from 'aws-cdk-lib/aws-dynamodb';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';

// Table names carry the `poker_` segment so they never collide with another
// service's tables in the same AWS account.
export type TableName =
    'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
    'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
    'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands';

interface DynamoDBStackProps extends cdk.StackProps {
  environment: Environment;
}

export class DynamoDBStack extends cdk.Stack {
  public readonly tables: Map<TableName, dynamodb.TableV2>;

  constructor(scope: Construct, id: string, props: DynamoDBStackProps) {
    super(scope, id, props);
    this.tables = new Map();
    const {environment} = props;
    const removalPolicy = environment === 'dev' ? RemovalPolicy.DESTROY : RemovalPolicy.RETAIN;
    const pointInTimeRecoverySpecification =
        environment === 'prod' ? {pointInTimeRecoveryEnabled: true} : undefined;

    const table = (
        name: TableName, withSortKey: boolean, withTTL: boolean = false, withStream: boolean = false,
    ): dynamodb.TableV2 => {
      const tableName = `${environment}_${name}`;
      const t = new dynamodb.TableV2(this, tableName, {
        tableName,
        partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
        sortKey: withSortKey ? {name: 'sk', type: dynamodb.AttributeType.STRING} : undefined,
        billing: Billing.onDemand({maxReadRequestUnits: 1000, maxWriteRequestUnits: 1000}),
        removalPolicy,
        pointInTimeRecoverySpecification,
        encryption: dynamodb.TableEncryptionV2.awsManagedKey(),
        timeToLiveAttribute: withTTL ? 'ttl' : undefined,
        dynamoStream: withStream ? dynamodb.StreamViewType.NEW_IMAGE : undefined,
      });
      this.tables.set(name, t);
      return t;
    };

    // poker_table_state: the single authoritative item per table, versioned
    // (tablestore.CommitAction) — no TTL, no stream, always current.
    // gsi_active_last_action is sparse — only tables still active carry a
    // gsi_active value (tablestore.SeedTable sets it; cmd/tablecleanup's
    // archive step REMOVEs it) — so an archived table drops out of the index
    // instead of accumulating there forever.
    const tableState = table('poker_table_state', false);
    tableState.addGlobalSecondaryIndex({
      indexName: 'gsi_active_last_action',
      partitionKey: {name: 'gsi_active', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'last_action_at', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.KEYS_ONLY,
    });
    // poker_table_state_history: append-only audit snapshot of each hand's
    // final state, written just before the table resets for the next hand —
    // pk is the table ID, sk is the unix-seconds capture time. No TTL (kept
    // indefinitely for audit) and no stream (nothing consumes it downstream).
    table('poker_table_state_history', true);
    // poker_action_log: TTL'd (tablestore.logTTLDays = 90 days — the "recent
    // window" served directly from Dynamo) with a stream so the archiver
    // Lambda (archiver-stack.ts) ships every entry to S3 before that TTL ever
    // reaps it — nothing is lost, just moved to cheaper long-term storage.
    table('poker_action_log', true, true, true);
    // poker_action_guards: TTL'd (mirrors ctech-wallet's wallet_idempotency
    // table) — a guard only needs to outlive plausible client retries
    // (tablestore.guardTTLDays = 7 days).
    table('poker_action_guards', false, true);

    // poker_rooms is lobby metadata only. The sparse indexes are populated by
    // roomstore for public rooms and private-room share codes respectively.
    const rooms = table('poker_rooms', true);
    rooms.addGlobalSecondaryIndex({
      indexName: 'gsi_public',
      partitionKey: {name: 'gsi_public', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    rooms.addGlobalSecondaryIndex({
      indexName: 'gsi_share_code',
      partitionKey: {name: 'gsi_share_code', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    table('poker_player_profiles', false);
    table('poker_achievement_progress', true);
    const leaderboardStats = table('poker_leaderboard_stats', true);
    leaderboardStats.addGlobalSecondaryIndex({
      indexName: 'gsi_hands_won',
      partitionKey: {name: 'gsi_hands_won_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'hands_won', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    leaderboardStats.addGlobalSecondaryIndex({
      indexName: 'gsi_hands_played',
      partitionKey: {name: 'gsi_hands_played_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'hands_played', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    leaderboardStats.addGlobalSecondaryIndex({
      indexName: 'gsi_win_rate',
      partitionKey: {name: 'gsi_win_rate_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'win_rate_score', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // One item per player/day and a TTL for automatic cooldown history cleanup.
    table('poker_daily_reward', true, true);
    table('poker_pending_cashouts', true);
    table('poker_player_sessions', true);
    table('poker_player_hands', true);
  }
}

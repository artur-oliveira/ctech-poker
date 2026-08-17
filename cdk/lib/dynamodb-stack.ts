import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {Billing} from 'aws-cdk-lib/aws-dynamodb';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {DYNAMO_INDEX, DYNAMO_TABLE} from './constants';

// Table names carry the `poker_` segment so they never collide with another
// service's tables in the same AWS account.
export type TableName =
  'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
  'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
  'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands' |
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_sandbox_purchases' |
  'poker_reaction_entitlements' | 'poker_reaction_purchases' |
  (typeof DYNAMO_TABLE)[keyof typeof DYNAMO_TABLE];

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
    const rooms = table('poker_rooms', true, true);
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
    const playerProfiles = table('poker_player_profiles', false);
    playerProfiles.addGlobalSecondaryIndex({
      indexName: DYNAMO_INDEX.friendCode,
      partitionKey: {name: 'friend_code', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // Private annotation namespace: pk is always the authenticated viewer and
    // sk is the opponent. Nothing reads this table while constructing public
    // table snapshots.
    table('poker_player_notes', true);
    // Opaque public token -> sanitized hand projection. TTL enforces the
    // owner's chosen expiry without retaining public links indefinitely.
    table('poker_hand_shares', false, true);
    // One permanent private aggregate per player plus short-lived idempotency
    // guards for completed hands.
    table('poker_player_poker_stats', false, true);
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
    // One row per purchase, pk=player_id sk=purchase_id — permanent history
    // (no TTL), unlike ctech-wallet's own pending-purchase row.
    table('poker_sandbox_purchases', true);
    // poker_reaction_entitlements: pk = player_id, sk = reaction_id — one row
    // per owned premium reaction. No TTL (permanent), no GSI
    // (Actor.handleReaction reads it by exact key, cached in Valkey — see
    // docs/specs/2026-08-12-premium-reactions.md).
    table('poker_reaction_entitlements', true);
    // poker_reaction_purchases: pk = player_id, sk = purchase_id — permanent
    // purchase history, mirrors poker_sandbox_purchases. No GSI: pix
    // confirmation is webhook-driven (no local pending sweep), fichas
    // purchases are synchronous.
    table('poker_reaction_purchases', true);
    // Resolved money-movement safety records are retained for 30 days for
    // audit/debugging, then reaped by DynamoDB TTL. Unresolved entries never
    // receive ttl and therefore cannot expire before reconciliation.
    const pendingCashouts = table('poker_pending_cashouts', true, true);
    pendingCashouts.addGlobalSecondaryIndex({
      indexName: 'gsi_status',
      partitionKey: {name: 'gsi_status', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // poker_player_sessions: TTL'd — only tracks which table a player is
    // currently at (or was recently at); the durable per-hand history lives
    // in poker_player_hands instead.
    table('poker_player_sessions', true, true);
    const playerHands = table('poker_player_hands', true);
    playerHands.addGlobalSecondaryIndex({
      indexName: 'gsi_table_id',
      partitionKey: {
        name: 'pk', type: dynamodb.AttributeType.STRING
      },
      sortKeys: [
        {
          name: 'table_id', type: dynamodb.AttributeType.STRING,
        },
        {
          name: 'sk', type: dynamodb.AttributeType.STRING,
        }
      ],
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // Directed relationship rows. Mirrored friendship transitions will be
    // committed atomically by the social store introduced in the next slice.
    table(DYNAMO_TABLE.socialEdges, true);

    // Materialized opponent recency. TTL bounds retention to 90 days while
    // the sparse chronological index supports cursor pagination per viewer.
    const recentPlayers = table(DYNAMO_TABLE.recentPlayers, true, true);
    recentPlayers.addGlobalSecondaryIndex({
      indexName: DYNAMO_INDEX.recentPlayers,
      partitionKey: {name: 'gsi_recent_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'gsi_recent_sk', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // Durable in-app inbox. The unread GSI is sparse: read events omit its
    // partition key and disappear from the counter without deleting history.
    const socialEvents = table(DYNAMO_TABLE.socialEvents, true, true);
    socialEvents.addGlobalSecondaryIndex({
      indexName: DYNAMO_INDEX.socialInbox,
      partitionKey: {name: 'gsi_inbox_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'gsi_inbox_sk', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    socialEvents.addGlobalSecondaryIndex({
      indexName: DYNAMO_INDEX.socialUnread,
      partitionKey: {name: 'gsi_unread_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'gsi_unread_sk', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // Open reports deliberately have no ttl attribute. Resolution adds one,
    // allowing DynamoDB to reap the record after the documented audit window.
    const playerReports = table(DYNAMO_TABLE.playerReports, true, true);
    playerReports.addGlobalSecondaryIndex({
      indexName: DYNAMO_INDEX.reportStatus,
      partitionKey: {name: 'gsi_status_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'gsi_status_sk', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
  }
}

import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {Billing} from 'aws-cdk-lib/aws-dynamodb';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import * as cloudwatchActions from 'aws-cdk-lib/aws-cloudwatch-actions';
import * as sns from 'aws-cdk-lib/aws-sns';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {ALERTS_TOPIC_ARN, DYNAMO_INDEX, DYNAMO_TABLE} from './constants';

// Tables on the hot commit path: every committed table action is a
// TransactWriteItems touching poker_table_state + poker_action_log +
// poker_action_guards (transactional writes bill 2x WCU), plus poker_rooms
// (lobby liveness) and poker_player_sessions (per-connection presence)
// churn on every seat/leave/reconnect. See
// docs/plans/2026-09-02-systematic-review-and-issue-backlog.md §3 Issue 6
// and GitHub issue #34. Everything else (purchase history, matchups,
// highlights, hand shares, ...) is comparatively cold and keeps the
// original 1000 RRU/WCU on-demand ceiling.
const HOT_PATH_TABLES: ReadonlySet<TableName> = new Set<TableName>([
  'poker_table_state', 'poker_action_log', 'poker_action_guards', 'poker_rooms', 'poker_player_sessions',
]);
// A dozen concurrently active tables during a promo, or a post-deploy
// reconnect burst, comfortably clears 1000 RRU/WCU on the hot-path tables
// without ever approaching DynamoDB's on-demand scaling limits — see the
// per-table cap review in issue #34. On-demand only bills for units actually
// consumed, so raising the ceiling here has $0 cost unless traffic grows
// into it.
const HOT_PATH_CAPACITY = {maxReadRequestUnits: 4000, maxWriteRequestUnits: 4000};
const COLD_PATH_CAPACITY = {maxReadRequestUnits: 1000, maxWriteRequestUnits: 1000};

// Table names carry the `poker_` segment so they never collide with another
// service's tables in the same AWS account.
export type TableName =
  'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
  'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
  'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands' |
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_player_matchups' |
  'poker_sandbox_purchases' |
  'poker_reaction_entitlements' | 'poker_reaction_purchases' |
  'poker_cosmetic_entitlements' | 'poker_cosmetic_purchases' |
  'poker_table_entitlements' | 'poker_table_highlights' |
  'poker_hand_reveals' | 'poker_hand_reveal_payments' |
  (typeof DYNAMO_TABLE)[keyof typeof DYNAMO_TABLE];

interface DynamoDBStackProps extends cdk.StackProps {
  environment: Environment;
  cloudwatchAlarmsEnabled: boolean;
}

export class DynamoDBStack extends cdk.Stack {
  public readonly tables: Map<TableName, dynamodb.TableV2>;

  constructor(scope: Construct, id: string, props: DynamoDBStackProps) {
    super(scope, id, props);
    this.tables = new Map();
    const {environment, cloudwatchAlarmsEnabled} = props;
    const removalPolicy = environment === 'dev' ? RemovalPolicy.DESTROY : RemovalPolicy.RETAIN;

    // PITR (continuous backups) is per-table, not blanket-on. It earns its
    // keep only where the data is durable and non-reconstructable — hands,
    // profiles, stats, purchases, money-safety rows. Ephemeral tables (the
    // authoritative table item and its audit-history snapshots, the S3-archived
    // action log, the 7-day idempotency guards, per-connection presence) opt
    // out with `pitr: false`: nothing there is worth a point-in-time restore,
    // and a runaway write path (2026-09-02 incident,
    // docs/specs/2026-09-03-next-hand-rearm-storm.md) shows up in
    // TimedPITRStorage-ByteHrs too. Only ever set in prod.
    const table = (
      name: TableName, withSortKey: boolean, withTTL: boolean = false, withStream: boolean = false,
      pitr: boolean = true,
    ): dynamodb.TableV2 => {
      const tableName = `${environment}_${name}`;
      const capacity = HOT_PATH_TABLES.has(name) ? HOT_PATH_CAPACITY : COLD_PATH_CAPACITY;
      const t = new dynamodb.TableV2(this, tableName, {
        tableName,
        partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
        sortKey: withSortKey ? {name: 'sk', type: dynamodb.AttributeType.STRING} : undefined,
        billing: Billing.onDemand(capacity),
        removalPolicy,
        pointInTimeRecoverySpecification:
          environment === 'prod' ? {pointInTimeRecoveryEnabled: pitr} : undefined,
        encryption: dynamodb.TableEncryptionV2.awsManagedKey(),
        timeToLiveAttribute: withTTL ? 'ttl' : undefined,
        dynamoStream: withStream ? dynamodb.StreamViewType.NEW_IMAGE : undefined,
      });
      this.tables.set(name, t);
      return t;
    };

    // Throttle alarm on the hot-path tables — wired to the existing
    // account-wide alerts topic (never a new SNS topic; see #34). Fires on
    // any read or write throttle event within the window: DynamoDB emits
    // ReadThrottleEvents/WriteThrottleEvents in on-demand mode exactly as it
    // does in provisioned mode, whether the throttle comes from the
    // maxRead/WriteRequestUnits ceiling above or from a hot single-partition
    // burst. CloudWatch alarms are ~$0.10/mo each on the standard tier — five
    // hot-path tables is a negligible, predictable cost, not a metered spend
    // that scales with traffic. Gated on `cloudwatchAlarmsEnabled` all the
    // same — a cost lever to turn every alarm this app creates off at once.
    const alertsTopic = cloudwatchAlarmsEnabled
      ? sns.Topic.fromTopicArn(this, 'AlertsTopic', ALERTS_TOPIC_ARN) : undefined;
    const addThrottleAlarm = (t: dynamodb.TableV2, name: TableName) => {
      if (!alertsTopic) return;
      const throttleEvents = new cloudwatch.MathExpression({
        expression: 'reads + writes',
        usingMetrics: {
          reads: t.metric('ReadThrottleEvents', {statistic: 'sum'}),
          writes: t.metric('WriteThrottleEvents', {statistic: 'sum'}),
        },
        period: cdk.Duration.minutes(5),
      });
      const alarm = new cloudwatch.Alarm(this, `${name}ThrottleAlarm`, {
        alarmName: `${environment}-${name}-throttled-requests`,
        alarmDescription: `${environment}_${name} is being throttled — see issue #34's per-table capacity review.`,
        metric: throttleEvents,
        threshold: 1,
        evaluationPeriods: 3,
        datapointsToAlarm: 3,
        comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
        treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
      });
      alarm.addAlarmAction(new cloudwatchActions.SnsAction(alertsTopic));
    };

    // Sustained-write-volume alarm. The throttle alarm above never fired on
    // 2026-09-02: on-demand scaled the write storm just fine, it just billed
    // for it (docs/specs/2026-09-03-next-hand-rearm-storm.md). This one
    // watches ConsumedWriteCapacityUnits directly — an active six-max table
    // peaks near ~8k WCU/5min, the incident hit >200k. `threshold` is set per
    // table well above a busy peak and well below a runaway loop; one 5-min
    // breach pages, so a wedge is caught in minutes, not on the next bill.
    const addWriteVolumeAlarm = (t: dynamodb.TableV2, name: TableName, threshold: number) => {
      if (!alertsTopic) return;
      const alarm = new cloudwatch.Alarm(this, `${name}WriteVolumeAlarm`, {
        alarmName: `${environment}-${name}-write-volume`,
        alarmDescription:
          `${environment}_${name} consumed >${threshold} write units in 5 minutes — a runaway write path, ` +
          'not organic play. See docs/specs/2026-09-03-next-hand-rearm-storm.md.',
        metric: t.metric('ConsumedWriteCapacityUnits', {
          statistic: 'sum', period: cdk.Duration.minutes(5),
        }),
        threshold,
        evaluationPeriods: 1,
        datapointsToAlarm: 1,
        comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
        treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
      });
      alarm.addAlarmAction(new cloudwatchActions.SnsAction(alertsTopic));
    };

    // poker_table_state: the single authoritative item per table, versioned
    // (tablestore.CommitAction). Ephemeral — TTL'd (tablestore.stateTTLDays,
    // refreshed on every commit) so a dead table is reaped instead of lingering
    // forever, and PITR-off (nothing here is worth a point-in-time restore; a
    // rejoin re-seeds). No stream. gsi_active_last_action is sparse — only
    // tables still active carry a gsi_active value (tablestore.SeedTable sets
    // it; cmd/tablecleanup's archive step REMOVEs it) — so an archived table
    // drops out of the index instead of accumulating there forever.
    const tableState = table('poker_table_state', false, true, false, false);
    tableState.addGlobalSecondaryIndex({
      indexName: 'gsi_active_last_action',
      partitionKey: {name: 'gsi_active', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'last_action_at', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.KEYS_ONLY,
    });
    addThrottleAlarm(tableState, 'poker_table_state');
    addWriteVolumeAlarm(tableState, 'poker_table_state', 40_000);
    // poker_table_state_history: append-only audit snapshot of each hand's
    // final state, written just before the table resets for the next hand —
    // pk is the table ID, sk is the unix-seconds capture time. Ephemeral audit
    // data: TTL'd at stateTTLDays and PITR-off, same as poker_table_state.
    table('poker_table_state_history', true, true, false, false);
    // poker_action_log: TTL'd (tablestore.logTTLDays = 90 days — the "recent
    // window" served directly from Dynamo) with a stream so the archiver
    // Lambda (archiver-stack.ts) ships every entry to S3 before that TTL ever
    // reaps it — nothing is lost, just moved to cheaper long-term storage.
    // PITR-off: S3 is already the durable copy, a second continuous backup of
    // the same rows buys nothing.
    const actionLog = table('poker_action_log', true, true, true, false);
    addThrottleAlarm(actionLog, 'poker_action_log');
    // poker_action_guards: TTL'd (mirrors ctech-wallet's wallet_idempotency
    // table) — a guard only needs to outlive plausible client retries
    // (tablestore.guardTTLDays = 7 days). PITR-off: a lost idempotency crumb
    // is not worth restoring.
    const actionGuards = table('poker_action_guards', false, true, false, false);
    addThrottleAlarm(actionGuards, 'poker_action_guards');

    // poker_rooms is lobby metadata only. The sparse indexes are populated by
    // roomstore for public rooms and private-room share codes respectively.
    const rooms = table('poker_rooms', true, true);
    rooms.addGlobalSecondaryIndex({
      indexName: 'gsi_public',
      partitionKey: {name: 'gsi_public', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // gsi_bucket is the lobby's per-bucket directory: one partition per
    // (currency mode, blinds, seats) pick, written by roomstore.BucketKey for
    // public rooms only (sparse, same convention as gsi_public). It is what
    // makes POST /rooms/join-or-create cost a function of the requested
    // bucket instead of the whole public directory (#213).
    rooms.addGlobalSecondaryIndex({
      indexName: 'gsi_bucket',
      partitionKey: {name: 'gsi_bucket', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    addThrottleAlarm(rooms, 'poker_rooms');
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
    // poker_hand_reveals: one permanent row per sandbox hand that ended
    // without a showdown with exactly one winner (pk = hand_id — globally
    // unique already, so the buy/check endpoints never need the table id to
    // look a hand up). Holds every participant's true hole cards regardless
    // of whether they were ever shown, gated entirely by the paid-reveal
    // endpoint that is the only reader of this table — sessionlog.HandItem
    // and hand-shares are untouched and keep their existing write-time
    // redaction as their only guarantee. No TTL: matches poker_player_hands'
    // real (permanent) retention, not a TTL'd table.
    // See docs/specs/2026-08-21-pay-to-see-winner-cards-history.md.
    table('poker_hand_reveals', false);
    // poker_hand_reveal_payments: one permanent row per (hand, buyer) pair
    // recording a paid reveal purchase — kept in its own table so a payment
    // write never races the poker_hand_reveals write (that one is written
    // once, by the hand-complete/hand-updated hooks, and never touched
    // again). No TTL, mirrors poker_sandbox_purchases' permanent history.
    table('poker_hand_reveal_payments', false);
    // One permanent private aggregate per player plus short-lived idempotency
    // guards for completed hands.
    table('poker_player_poker_stats', false, true);
    // poker_player_matchups: one permanent aggregate per unordered player
    // pair (pk "pair#<mode>#<idLow>#<idHigh>") plus short-lived per-pair
    // idempotency guards for completed hands (pk "guard#<table>#<hand>#pair#...") —
    // same PK-only, TTL'd shape as poker_player_poker_stats.
    // See docs/specs/2026-08-21-head-to-head-stats.md.
    table('poker_player_matchups', false, true);
    // poker_table_highlights: one item per table per day (pk table_id, sk
    // date), overwritten in place as bigger pots come in — rolls over
    // naturally at UTC midnight since the sort key changes. No TTL; a
    // 30-day-old item table costs nothing to keep.
    table('poker_table_highlights', true);
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
    // poker_cosmetic_entitlements: pk = player_id, sk = "kind#item_id" — one
    // row per owned premium deck/felt cosmetic, mirrors
    // poker_reaction_entitlements. No TTL (permanent), no GSI (player.Service
    // reads it by exact key — see
    // docs/specs/2026-08-21-premium-cosmetics-overhaul.md).
    table('poker_cosmetic_entitlements', true);
    // poker_cosmetic_purchases: pk = player_id, sk = purchase_id — permanent
    // purchase history, mirrors poker_reaction_purchases. Pix confirmation is
    // webhook-driven (no local pending sweep) and fichas purchases are
    // synchronous, so nothing sweeps this table; the one index it needs is
    // gsi_player_kind, so the deck and felt history endpoints can each page
    // their own rows with a key condition instead of reading the other kind's
    // rows through a FilterExpression and returning short pages (issue #219).
    // Both key attributes are already on every row ever written — no backfill,
    // no new attribute. Projection ALL because the endpoint returns whole
    // purchase records; the extra write cost lands on a table that sees one
    // write per purchase, not per hand.
    const cosmeticPurchases = table('poker_cosmetic_purchases', true);
    cosmeticPurchases.addGlobalSecondaryIndex({
      indexName: 'gsi_player_kind',
      partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'kind', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // poker_table_entitlements: pk = player_id, sk = "ent#<origin_table_id>"
    // — one row per paid table reservation (docs/plans/2026-08-21-entry-fee-entitlement.md).
    // sk is fixed at the originally-paid table (the idempotency key that
    // stops a concurrent buy-in from double-charging); bound_table_id is the
    // separate, mutable attribute a rebind moves when that table becomes
    // unavailable. TTL reaps rows well after their absolute expiry.
    table('poker_table_entitlements', true, true);
    // Resolved money-movement safety records are retained for 30 days for
    // audit/debugging, then reaped by DynamoDB TTL. Unresolved entries never
    // receive ttl and therefore cannot expire before reconciliation.
    // Keeps PITR: these are money-safety records — a lost unresolved row is a
    // stranded credit. It also spiked 1:1 with the wedged next-hand loop on
    // 2026-09-02 (one rejected settlement Put per rejected transaction), so it
    // gets the write-volume alarm too — from a near-zero organic baseline any
    // sustained volume here is a loop.
    const pendingCashouts = table('poker_pending_cashouts', true, true);
    pendingCashouts.addGlobalSecondaryIndex({
      indexName: 'gsi_status',
      partitionKey: {name: 'gsi_status', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    addWriteVolumeAlarm(pendingCashouts, 'poker_pending_cashouts', 5_000);
    // poker_player_sessions: TTL'd — only tracks which table a player is
    // currently at (or was recently at); the durable per-hand history lives
    // in poker_player_hands instead. PITR-off: per-connection presence, not
    // history.
    const playerSessions = table('poker_player_sessions', true, true, false, false);
    addThrottleAlarm(playerSessions, 'poker_player_sessions');
    // gsi_open_table is SPARSE: only an unclosed session carries
    // open_table_id (sessionlog.RecordSession derives it from ended_at, and
    // CloseSession's full-item put drops it), so this index holds just the
    // tables a player is seated at right now. It is what makes
    // FindOpenSession/FindLatestOpenSession single key queries instead of a
    // FilterExpression paged over the player's whole 30-day history (#224).
    // Projection ALL: the caller closes the session it reads back, so it
    // needs the whole item, which is a handful of scalars.
    playerSessions.addGlobalSecondaryIndex({
      indexName: 'gsi_open_table',
      partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'open_table_id', type: dynamodb.AttributeType.STRING},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    // A second index for HasSessionAtTable ("was this player ever at this
    // table?", open or closed) is deliberately NOT declared here yet:
    // DynamoDB rejects an update that creates more than one GSI on a table
    // ("Cannot perform more than one GSI creation or deletion in a single
    // update"), which failed the prod deploy of #224. gsi_open_table ships
    // first because it is the one on the seating hot path; gsi_player_table
    // follows in its own deploy once this one is ACTIVE, together with the
    // sessionlog change that queries it.
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

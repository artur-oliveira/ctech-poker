import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {Billing} from 'aws-cdk-lib/aws-dynamodb';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';

// Table names carry the `poker_` segment so they never collide with another
// service's tables in the same AWS account.
export type TableName = 'poker_table_state' | 'poker_action_log' | 'poker_action_guards';

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
    table('poker_table_state', false);
    // poker_action_log: TTL'd (tablestore.logTTLDays = 90 days — the "recent
    // window" served directly from Dynamo) with a stream so the archiver
    // Lambda (archiver-stack.ts) ships every entry to S3 before that TTL ever
    // reaps it — nothing is lost, just moved to cheaper long-term storage.
    table('poker_action_log', true, true, true);
    // poker_action_guards: TTL'd (mirrors ctech-wallet's wallet_idempotency
    // table) — a guard only needs to outlive plausible client retries
    // (tablestore.guardTTLDays = 7 days).
    table('poker_action_guards', false, true);
  }
}

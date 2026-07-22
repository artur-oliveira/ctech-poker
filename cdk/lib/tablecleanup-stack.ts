import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {localGoBundling} from './bundle';

const TABLE_CLEANUP_RATE_MINUTES = 30;

interface TableCleanupStackProps extends cdk.StackProps {
  environment: Environment;
  tableStateArn: string;
  roomsTableArn: string;
  walletUrlParam: string;
  pokerClientIdParam: string;
  pokerClientSecretParam: string;
}

/**
 * Stale-table sweep — mirrors reconcile-stack.ts's shape: a Lambda built
 * from cmd/tablecleanup on an EventBridge schedule. Archives sandbox tables
 * idle past cmd/tablecleanup's staleCutoff, refunding seated stacks first
 * via ctech-wallet.
 */
export class TableCleanupStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: TableCleanupStackProps) {
    super(scope, id, props);
    const {environment, tableStateArn, roomsTableArn, walletUrlParam, pokerClientIdParam, pokerClientSecretParam} = props;

    const role = new iam.Role(this, 'TableCleanupRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Query', 'dynamodb:GetItem', 'dynamodb:UpdateItem'],
      resources: [tableStateArn, `${tableStateArn}/index/*`],
    }));
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:GetItem'],
      resources: [roomsTableArn],
    }));
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        `arn:aws:ssm:${this.region}:${this.account}:parameter${walletUrlParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientIdParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientSecretParam}`,
      ],
    }));

    const fn = new lambda.Function(this, 'TableCleanupFunction', {
      functionName: `${environment}-ctech-poker-tablecleanup`,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('../api/cmd/tablecleanup', {
        bundling: {
          local: localGoBundling('../api/cmd/tablecleanup'),
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: ['bash', '-c', 'GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap .'],
        },
      }),
      role,
      timeout: cdk.Duration.minutes(2),
      memorySize: 256,
      environment: {
        ENVIRONMENT: environment,
        WALLET_URL_PARAM: walletUrlParam,
        POKER_CLIENT_ID_PARAM: pokerClientIdParam,
        POKER_CLIENT_SECRET_PARAM: pokerClientSecretParam,
      },
    });

    const schedulerRole = new iam.Role(this, 'TableCleanupSchedulerInvokeRole', {
      assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
    });
    fn.grantInvoke(schedulerRole);

    new scheduler.CfnSchedule(this, 'TableCleanupSchedule', {
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: `rate(${TABLE_CLEANUP_RATE_MINUTES} minutes)`,
      target: {arn: fn.functionArn, roleArn: schedulerRole.roleArn},
    });
  }
}

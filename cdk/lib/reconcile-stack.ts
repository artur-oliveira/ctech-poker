import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import * as logs from 'aws-cdk-lib/aws-logs';
import {Construct} from 'constructs';
import path from 'node:path';
import {Environment} from '@aoctech/cdk';

const RECONCILE_RATE_MINUTES = 5;
const API_DIR = path.join(__dirname, '../../api');

interface ReconcileStackProps extends cdk.StackProps {
  environment: Environment;
  pendingCashoutsTableArn: string;
  walletUrlParam: string;
  pokerClientIdParam: string;
  pokerClientSecretParam: string;
}

/**
 * Cash-out reconciliation job — mirrors ctech-wallet/cdk/lib/reconcile-stack.ts
 * shape: a Lambda built from cmd/reconcile on a 5-minute EventBridge schedule.
 */
export class ReconcileStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ReconcileStackProps) {
    super(scope, id, props);
    const {environment, pendingCashoutsTableArn, walletUrlParam, pokerClientIdParam, pokerClientSecretParam} = props;

    const role = new iam.Role(this, 'ReconcileRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Scan', 'dynamodb:UpdateItem'],
      resources: [pendingCashoutsTableArn],
    }));
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        `arn:aws:ssm:${this.region}:${this.account}:parameter${walletUrlParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientIdParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientSecretParam}`,
      ],
    }));

    const fn = new lambda.Function(this, 'ReconcileFunction', {
      functionName: `${environment}-ctech-poker-reconcile`,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset(path.join(API_DIR, 'dist/reconcile')),
      role,
      timeout: cdk.Duration.minutes(2),
      memorySize: 256,
      environment: {
        ENVIRONMENT: environment,
        WALLET_URL_PARAM: walletUrlParam,
        POKER_CLIENT_ID_PARAM: pokerClientIdParam,
        POKER_CLIENT_SECRET_PARAM: pokerClientSecretParam,
      },
      logRetention: environment === 'prod' ? logs.RetentionDays.ONE_MONTH : logs.RetentionDays.ONE_WEEK,
    });

    const schedulerRole = new iam.Role(this, 'SchedulerInvokeRole', {
      assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
    });
    fn.grantInvoke(schedulerRole);

    new scheduler.CfnSchedule(this, 'ReconcileSchedule', {
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: `rate(${RECONCILE_RATE_MINUTES} minutes)`,
      target: {arn: fn.functionArn, roleArn: schedulerRole.roleArn},
    });
  }
}

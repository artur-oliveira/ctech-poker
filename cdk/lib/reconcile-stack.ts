import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import * as sqs from 'aws-cdk-lib/aws-sqs';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {localGoBundling} from "./bundle";
import {reconcileDlqName, reconcileJobName} from './constants';

const RECONCILE_RATE_MINUTES = 5;

interface ReconcileStackProps extends cdk.StackProps {
  environment: Environment;
  authDomainName: string;
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
    const {environment, authDomainName, pendingCashoutsTableArn, walletUrlParam, pokerClientIdParam, pokerClientSecretParam} = props;

    const role = new iam.Role(this, 'ReconcileRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Scan', 'dynamodb:Query', 'dynamodb:UpdateItem'],
      resources: [pendingCashoutsTableArn, `${pendingCashoutsTableArn}/index/*`],
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
      functionName: reconcileJobName(environment),
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('../api', {
        bundling: {
          local: localGoBundling('../api', './cmd/reconcile'),
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: ['bash', '-c', 'GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap ./cmd/reconcile'],
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
        CTECH_ISSUER_URL: `https://${authDomainName}`
      },
    });

    const schedulerRole = new iam.Role(this, 'SchedulerInvokeRole', {
      assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
    });
    fn.grantInvoke(schedulerRole);

    const dlq = new sqs.Queue(this, 'ReconcileDLQ', {
      queueName: reconcileDlqName(environment),
      encryption: sqs.QueueEncryption.SQS_MANAGED,
      retentionPeriod: cdk.Duration.days(14),
      receiveMessageWaitTime: cdk.Duration.seconds(20),
    });
    dlq.grantSendMessages(schedulerRole);

    new scheduler.CfnSchedule(this, 'ReconcileSchedule', {
      name: reconcileJobName(environment),
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: `rate(${RECONCILE_RATE_MINUTES} minutes)`,
      target: {
        arn: fn.functionArn,
        roleArn: schedulerRole.roleArn,
        deadLetterConfig: {arn: dlq.queueArn},
        retryPolicy: {maximumEventAgeInSeconds: 3600, maximumRetryAttempts: 3},
      },
    });

    new cloudwatch.Alarm(this, 'ReconcileDLQAlarm', {
      alarmName: `${reconcileJobName(environment)}-dlq-messages`,
      metric: dlq.metricApproximateNumberOfMessagesVisible({period: cdk.Duration.minutes(5)}),
      threshold: 1, evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
    new cloudwatch.Alarm(this, 'ReconcileErrorsAlarm', {
      alarmName: `${reconcileJobName(environment)}-errors`,
      metric: fn.metricErrors({period: cdk.Duration.minutes(5)}),
      threshold: 1, evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
    new cloudwatch.Alarm(this, 'ReconcileThrottlesAlarm', {
      alarmName: `${reconcileJobName(environment)}-throttles`,
      metric: fn.metricThrottles({period: cdk.Duration.minutes(5)}),
      threshold: 1, evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
    new cloudwatch.Alarm(this, 'ReconcileMissedRunAlarm', {
      alarmName: `${reconcileJobName(environment)}-missed-run`,
      alarmDescription: `No reconcile invocation in two ${RECONCILE_RATE_MINUTES}-minute windows.`,
      metric: fn.metricInvocations({period: cdk.Duration.minutes(RECONCILE_RATE_MINUTES * 2), statistic: 'Sum'}),
      threshold: 1, comparisonOperator: cloudwatch.ComparisonOperator.LESS_THAN_THRESHOLD,
      evaluationPeriods: 1, treatMissingData: cloudwatch.TreatMissingData.BREACHING,
    });
  }
}

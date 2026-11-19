import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import * as sqs from 'aws-cdk-lib/aws-sqs';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {localGoBundling} from './bundle';
import {
  PUSH_TICK_SCHEDULE, PUSH_TICK_TIMEZONE, SSM_POKER, alertsTopicParam, pushParameterPath, pushSenderName,
  pushTickDlqName, pushTickName,
} from './constants';

interface PushStackProps extends cdk.StackProps {
  environment: Environment;
  pushTableArn: string;
  dailyRewardTableArn: string;
  sessionsTableArn: string;
}

/**
 * Scheduled Web Push (docs/specs/2026-11-18-scheduled-web-push.md): EventBridge Scheduler runs push-tick every 15 minutes
 * of São Paulo time; the tick reads the reminders due in the slot, applies Poker's rules and invokes push-sender, which
 * holds the VAPID key and sends. Both are inert until an operator writes the VAPID parameters. Zero-idle cost: no KMS key
 * of ours, no SQS event source, no alarm; ~2,900 tick invocations a month, inside the free tiers.
 */
export class PushStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: PushStackProps) {
    super(scope, id, props);
    const {environment, pushTableArn, dailyRewardTableArn, sessionsTableArn} = props;

    const goLambda = (cmd: string) => lambda.Code.fromAsset('../api', {
      bundling: {
        local: localGoBundling('../api', `./cmd/${cmd}`),
        image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
        command: ['bash', '-c', `GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap ./cmd/${cmd}`],
      },
    });
    const alertsTopicArn = ssm.StringParameter.valueForStringParameter(this, alertsTopicParam(environment));
    const ssmArn = (path: string) => `arn:aws:ssm:${this.region}:${this.account}:parameter${path}`;
    const vapidParams = ['vapid-private-key', 'vapid-private-key-previous', 'vapid-public-key']
      .map((n) => ssmArn(pushParameterPath(environment, n as 'vapid-public-key')));

    // ── push-sender ──────────────────────────────────────────────────────────
    const senderRole = new iam.Role(this, 'PushSenderRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    // Devices of a person (Query), and the delete of a device that is gone (the count row moves in the same transaction).
    senderRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Query', 'dynamodb:DeleteItem', 'dynamodb:UpdateItem'],
      resources: [pushTableArn],
    }));
    senderRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [...vapidParams, ssmArn(SSM_POKER(environment).appUrl)],
    }));
    senderRole.addToPolicy(new iam.PolicyStatement({
      actions: ['kms:Decrypt'],
      resources: [`arn:aws:kms:${this.region}:${this.account}:key/*`],
      conditions: {
        StringEquals: {'kms:ViaService': `ssm.${this.region}.amazonaws.com`},
        StringLike: {'kms:EncryptionContext:PARAMETER_ARN': vapidParams},
      },
    }));
    senderRole.addToPolicy(new iam.PolicyStatement({actions: ['sns:Publish'], resources: [alertsTopicArn]}));
    const sender = new lambda.Function(this, 'PushSender', {
      functionName: pushSenderName(environment),
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: goLambda('push-sender'),
      role: senderRole,
      timeout: cdk.Duration.seconds(30),
      memorySize: 128,
      reservedConcurrentExecutions: 5,
      retryAttempts: 2,
      maxEventAge: cdk.Duration.hours(1),
      environment: {
        ENVIRONMENT: environment,
        PUSH_VAPID_PRIVATE_KEY_SSM_PATH: pushParameterPath(environment, 'vapid-private-key'),
        PUSH_VAPID_PRIVATE_KEY_PREVIOUS_SSM_PATH: pushParameterPath(environment, 'vapid-private-key-previous'),
        PUSH_VAPID_PUBLIC_KEY_SSM_PATH: pushParameterPath(environment, 'vapid-public-key'),
        PUSH_SUBJECT_SSM_PATH: SSM_POKER(environment).appUrl,
        ALERTS_TOPIC_ARN: alertsTopicArn,
      },
    });

    // ── push-tick ────────────────────────────────────────────────────────────
    const tickRole = new iam.Role(this, 'PushTickRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    // Reminders of a slot (the index), devices of a person, the day mark and a moved slot.
    tickRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Query', 'dynamodb:GetItem', 'dynamodb:UpdateItem'],
      resources: [pushTableArn, `${pushTableArn}/index/*`],
    }));
    // The rules read the streak (one GetItem) and the newest sessions (one Query).
    tickRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:GetItem'],
      resources: [dailyRewardTableArn],
    }));
    tickRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Query'],
      resources: [sessionsTableArn],
    }));
    tickRole.addToPolicy(new iam.PolicyStatement({actions: ['lambda:InvokeFunction'], resources: [sender.functionArn]}));
    const tick = new lambda.Function(this, 'PushTick', {
      functionName: pushTickName(environment),
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: goLambda('push-tick'),
      role: tickRole,
      timeout: cdk.Duration.minutes(2),
      memorySize: 256,
      reservedConcurrentExecutions: 1,
      environment: {
        ENVIRONMENT: environment,
        PUSH_SENDER_FUNCTION: sender.functionName,
      },
    });

    const schedulerRole = new iam.Role(this, 'PushTickSchedulerInvokeRole', {
      assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
    });
    tick.grantInvoke(schedulerRole);
    const dlq = new sqs.Queue(this, 'PushTickDLQ', {
      queueName: pushTickDlqName(environment),
      encryption: sqs.QueueEncryption.SQS_MANAGED,
      retentionPeriod: cdk.Duration.days(14),
      receiveMessageWaitTime: cdk.Duration.seconds(20),
    });
    dlq.grantSendMessages(schedulerRole);
    new scheduler.CfnSchedule(this, 'PushTickSchedule', {
      name: pushTickName(environment),
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: PUSH_TICK_SCHEDULE,
      scheduleExpressionTimezone: PUSH_TICK_TIMEZONE,
      target: {
        arn: tick.functionArn,
        roleArn: schedulerRole.roleArn,
        deadLetterConfig: {arn: dlq.queueArn},
        retryPolicy: {maximumEventAgeInSeconds: 900, maximumRetryAttempts: 2},
      },
    });
  }
}

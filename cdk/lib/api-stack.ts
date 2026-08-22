import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as autoscaling from 'aws-cdk-lib/aws-autoscaling';
import * as hooktargets from 'aws-cdk-lib/aws-autoscaling-hooktargets';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import {Construct} from 'constructs';
import {Ec2ScriptRunner, Environment, HaproxyEc2Service,} from '@aoctech/cdk';
import {
  API_CURRENT_ARTIFACT_KEY,
  APP_PORT,
  asgName,
  HEALTH_CHECK_PATH,
  instanceRoleName,
  S3_PREFIX,
  SERVICE,
  SSM_ACCOUNT,
  SSM_POKER,
  SSM_SHARED,
} from './constants';

interface ApiStackProps extends cdk.StackProps {
  environment: Environment;
  // Must be a concrete string (not a token): ec2.Vpc.fromLookup resolves
  // subnet/AZ metadata at synthesis time. CI reads /ctech/{env}/network/vpc-id
  // from SSM into CTECH_VPC_ID before running cdk deploy (see ctech-cdk/CLAUDE.md
  // "Known Constraints").
  vpcId: string;
  instanceProfileName: string;
  deploymentsBucketName: string;
  logsBucketName: string;
  avatarsBucketName: string;
  avatarBaseUrlParam: string;
  tableStateArn: string;
  tableStateHistoryArn: string;
  actionLogArn: string;
  actionGuardsArn: string;
  roomsTableArn: string;
  playerProfilesTableArn: string;
  playerNotesTableArn: string;
  handSharesTableArn: string;
  pokerStatsTableArn: string;
  walletUrlParam: string;
  pokerClientIdParam: string;
  pokerClientSecretParam: string;
  turnstileSecretParam: string;
  realMoneyEnabledParam: string;
  legalSignoffRefParam: string;
  achievementProgressTableArn: string;
  leaderboardStatsTableArn: string;
  dailyRewardTableArn: string;
  playerSessionsTableArn: string;
  playerHandsTableArn: string;
  walletWebhookHmacSecretParam: string;
  sandboxPurchasesTableArn: string;
  pendingCashoutsTableArn: string;
  reactionEntitlementsTableArn: string;
  reactionPurchasesTableArn: string;
  cosmeticEntitlementsTableArn: string;
  cosmeticPurchasesTableArn: string;
  socialEdgesTableArn: string;
  recentPlayersTableArn: string;
  socialEventsTableArn: string;
  playerReportsTableArn: string;
  socialGraphEnabledParam: string;
  // Session Manager. **Off by default**: deploys replace the instances through an
  // ASG instance refresh, so nothing needs SSM RunCommand any more, and the
  // agent costs ~70 MiB of RSS on a t4g.nano. Poker pays one extra price for
  // that: the termination-drain lifecycle hook stops the app through RunCommand,
  // so with the agent off it fails open and instances terminate without draining
  // tables. Accepted while this is a development environment.
  enableSsmAgent?: boolean;
}

export const minimumApiCapacity = (_environment: Environment) => 1;

export class PokerApiStack extends cdk.Stack {
  public readonly asgName: string;

  constructor(scope: Construct, id: string, props: ApiStackProps) {
    super(scope, id, props);

    const {
      environment,
      vpcId,
      instanceProfileName,
      deploymentsBucketName,
      logsBucketName,
      avatarsBucketName,
      tableStateArn,
      tableStateHistoryArn,
      actionLogArn,
      actionGuardsArn,
      roomsTableArn,
      playerProfilesTableArn,
      playerNotesTableArn,
      handSharesTableArn,
      pokerStatsTableArn,
      walletUrlParam,
      pokerClientIdParam,
      pokerClientSecretParam,
      turnstileSecretParam,
      realMoneyEnabledParam,
      legalSignoffRefParam,
      avatarBaseUrlParam,
      achievementProgressTableArn,
      leaderboardStatsTableArn,
      playerSessionsTableArn,
      playerHandsTableArn,
      dailyRewardTableArn,
      walletWebhookHmacSecretParam,
      sandboxPurchasesTableArn,
      pendingCashoutsTableArn,
      reactionEntitlementsTableArn,
      reactionPurchasesTableArn,
      cosmeticPurchasesTableArn,
      cosmeticEntitlementsTableArn,
      socialEdgesTableArn,
      recentPlayersTableArn,
      socialEventsTableArn,
      playerReportsTableArn,
      socialGraphEnabledParam,
      enableSsmAgent = false,
    } = props;

    const shared = SSM_SHARED(environment);
    const account = SSM_ACCOUNT(environment);
    const poker = SSM_POKER(environment);

    const instanceRole = new iam.Role(this, 'ApiInstanceRole', {
      roleName: instanceRoleName(environment),
      assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
      managedPolicies: [
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonSSMManagedInstanceCore'),
        iam.ManagedPolicy.fromAwsManagedPolicyName('CloudWatchAgentServerPolicy'),
      ],
    });
    const profile = new iam.CfnInstanceProfile(this, 'ApiInstanceProfile', {
      instanceProfileName,
      roles: [instanceRole.roleName],
    });

    const tableArns = [
      tableStateArn, tableStateHistoryArn, actionLogArn, actionGuardsArn, roomsTableArn, playerProfilesTableArn,
      achievementProgressTableArn, leaderboardStatsTableArn, dailyRewardTableArn, playerSessionsTableArn,
      playerHandsTableArn, playerNotesTableArn, handSharesTableArn, pokerStatsTableArn, sandboxPurchasesTableArn,
      pendingCashoutsTableArn, reactionEntitlementsTableArn, reactionPurchasesTableArn, cosmeticEntitlementsTableArn,
      cosmeticPurchasesTableArn, socialEdgesTableArn, recentPlayersTableArn, socialEventsTableArn, playerReportsTableArn,
    ];
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: [
        'dynamodb:GetItem',
        'dynamodb:BatchGetItem',
        'dynamodb:PutItem',
        'dynamodb:UpdateItem',
        'dynamodb:DeleteItem',
        'dynamodb:Query',
        'dynamodb:DescribeTable',
        'dynamodb:ConditionCheckItem',
        'dynamodb:TransactWriteItems',
      ],
      resources: [...tableArns, ...tableArns.map((arn) => `${arn}/index/*`)],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        shared.valkeyUrl, walletUrlParam, pokerClientIdParam, pokerClientSecretParam, turnstileSecretParam,
        realMoneyEnabledParam, socialGraphEnabledParam, legalSignoffRefParam,
        avatarBaseUrlParam, walletWebhookHmacSecretParam,
        account.internalBaseUrl, account.appUrl, account.internalJwksUrl, poker.appUrl,
      ].map(
        (path) => `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${path}`,
      ),
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:GetObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${deploymentsBucketName}/${S3_PREFIX}/*`],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:PutObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${logsBucketName}/${S3_PREFIX}/*`],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:PutObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}/up/*`],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:GetObject', 's3:DeleteObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}/up/*`],
    }));
    // GetObject on av/ is what lets the API serve /v1.0/avatars/* itself. It
    // replaces the CloudFront origin access control that used to read the
    // prefix — nothing but the API reaches the bucket now.
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:PutObject', 's3:DeleteObject', 's3:GetObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}/av/*`],
    }));

    // ── Shared infrastructure from ctech-cdk ──────────────────────────────────
    const vpc = ec2.Vpc.fromLookup(this, 'Vpc', {vpcId});

    const albSgId = ssm.StringParameter.valueForStringParameter(this, shared.albSgId);
    const edgeSg = ec2.SecurityGroup.fromSecurityGroupId(this, 'EdgeSg', albSgId);

    const isProd = environment === 'prod';
    this.asgName = asgName(environment);
    const logRetention: logs.RetentionDays = isProd ? logs.RetentionDays.ONE_MONTH : logs.RetentionDays.ONE_WEEK;
    const logGroupApp = `/${SERVICE}/${environment}/app`;
    // No nginx in this stack (see APP_PORT doc comment in constants.ts). Keep the
    // existing log group/output stable for deployment and monitoring compatibility.
    const logGroupNginx = `/${SERVICE}/${environment}/nginx`;

    // ── User Data ─────────────────────────────────────────────────────────────
    // Every shared bootstrap step lives in ctech-cdk's assets/ec2 and is fetched
    // from S3 at boot. What stays inline is only what CloudFormation has to
    // resolve: bucket names, SSM paths, log group names and the CloudWatch agent
    // config.
    //
    // The S3 key prefix is the content hash of assets/ec2, read from SSM at
    // deploy time, so editing a shared script changes this user data, versions
    // the launch template and triggers an instance refresh.
    const scripts = new Ec2ScriptRunner(this, 'Scripts', {environment});
    scripts.grantRead(instanceRole);

    const userData = ec2.UserData.forLinux();
    scripts.install(userData);

    // No nginx in this stack (see APP_PORT doc comment in constants.ts): no extra
    // packages, no setup-nginx.sh and no setup-realip.sh.
    scripts.run(userData, 'setup-base.sh', SERVICE);
    scripts.run(userData, 'setup-swap.sh', '256');
    scripts.run(userData, 'setup-dualstack.sh');
    scripts.run(userData, 'setup-cloudflare-ca.sh');

    // setup-base.sh installs the SSM agent and setup-dualstack.sh starts it, so
    // this is what stops it again. See enableSsmAgent above: off also disables
    // the graceful table drain on termination.
    if (!enableSsmAgent) {
      userData.addCommands('systemctl disable --now amazon-ssm-agent 2>/dev/null || true');
    }

    // /etc/app-static.env: non-secret values systemd loads via EnvironmentFile.
    // CDK tokens are substituted at synthesis; bash does not expand them.
    userData.addCommands(
      `cat > /etc/app-static.env << 'ENV'`,
      `ENVIRONMENT=${environment}`,
      `AWS_REGION=${this.region}`,
      `AWS_USE_DUALSTACK_ENDPOINT=true`,
      `PORT=${APP_PORT}`,
      // Poker is reached directly from HAProxy, with no localhost nginx hop.
      // Trust only peers inside this VPC before honoring X-Forwarded-For.
      `TRUSTED_PROXIES=${vpc.vpcCidrBlock}`,
      `AVATAR_BUCKET=${avatarsBucketName}`,
      `ENV`,
    );

    // Read by name on every service start, never embedded: the launch template is
    // readable by anyone holding ec2:DescribeLaunchTemplateVersions. A parameter
    // that does not exist yet leaves the variable empty, which is what keeps the
    // kill switches fail-closed - REAL_MONEY_ENABLED and SOCIAL_GRAPH_ENABLED
    // carry `envDefault:"false"`, and prod requires VALKEY_URL.
    //
    // No DB-number suffix on VALKEY_URL - see constants.ts SSM_SHARED: ctech-dfe
    // and ctech-account both pass it through unmodified, and tablelease keys are
    // already namespaced by prefix (table:{id}).
    scripts.run(userData, 'setup-ssm-env.sh',
      `VALKEY_URL=${shared.valkeyUrl}`,
      `CTECH_URL=${account.internalBaseUrl}`,
      `CTECH_ISSUER_URL=${account.appUrl}`,
      `CTECH_JWKS_URL=${account.internalJwksUrl}`,
      `SERVICE_AUDIENCE=${poker.appUrl}`,
      `WALLET_URL=${walletUrlParam}`,
      `POKER_CLIENT_ID=${pokerClientIdParam}`,
      `POKER_CLIENT_SECRET=${pokerClientSecretParam}`,
      `TURNSTILE_SECRET=${turnstileSecretParam}`,
      `WALLET_WEBHOOK_HMAC_SECRET=${walletWebhookHmacSecretParam}`,
      `REAL_MONEY_ENABLED=${realMoneyEnabledParam}`,
      `SOCIAL_GRAPH_ENABLED=${socialGraphEnabledParam}`,
      `LEGAL_SIGNOFF_REF=${legalSignoffRefParam}`,
      `AVATAR_BASE_URL=${avatarBaseUrlParam}`,
    );

    // Derived from SERVICE_AUDIENCE rather than fetched - the escape hatch
    // start.sh sources after load-ssm-env.sh.
    userData.addCommands(
      `cat > /opt/app/service-env.sh << 'SERVICEENV'`,
      `CORS_ALLOWED_ORIGINS="$SERVICE_AUDIENCE"`,
      `TURNSTILE_EXPECTED_HOSTNAME="\${SERVICE_AUDIENCE#*://}"`,
      `TURNSTILE_EXPECTED_HOSTNAME="\${TURNSTILE_EXPECTED_HOSTNAME%%/*}"`,
      `export CORS_ALLOWED_ORIGINS TURNSTILE_EXPECTED_HOSTNAME`,
      `SERVICEENV`,
      `chmod 0755 /opt/app/service-env.sh`,
    );

    // network.target only: there is no nginx unit to wait for.
    scripts.run(userData, 'setup-app-service.sh', 'CTech Poker API', 'app');
    scripts.run(userData, 'setup-deploy.sh', deploymentsBucketName, 'app',
      `http://127.0.0.1:${APP_PORT}${HEALTH_CHECK_PATH}`);
    scripts.run(userData, 'setup-logs.sh', logsBucketName, S3_PREFIX, SERVICE, '/var/log/app');

    // Logs only. No `metrics` block: EC2 already publishes CPUUtilization and
    // CPUCreditBalance for free, and every custom series this service used to
    // publish was either that again or a number nobody alarmed on.
    // {instance_id} is resolved by the CW agent at runtime, not by bash.
    userData.addCommands(
      `cat > /tmp/cwagent.json << 'CWA'`,
      JSON.stringify({
        agent: {metrics_collection_interval: 60},
        logs: {
          logs_collected: {
            files: {
              collect_list: [
                {file_path: '/var/log/app/app.log', log_group_name: logGroupApp, log_stream_name: '{instance_id}'},
              ],
            },
          },
        },
      }),
      `CWA`,
    );
    scripts.run(userData, 'setup-cloudwatch-agent.sh', '/tmp/cwagent.json');
    scripts.run(userData, 'bootstrap-deploy.sh', deploymentsBucketName, API_CURRENT_ARTIFACT_KEY);

    // ctech-lbalancer still owns the bootstrap route and private CNAME.
    const service = new HaproxyEc2Service(this, 'ApiService', {
      vpc,
      edgeSecurityGroup: edgeSg,
      appPort: APP_PORT,
      userData,
      instanceProfileName,
      securityGroupName: `${environment}-${SERVICE}-api-sg`,
      securityGroupDescription: 'ctech-poker API instances',
      appLogGroupName: logGroupApp,
      // Kept for output/deployment compatibility even though poker has no nginx.
      nginxLogGroupName: logGroupNginx,
      logRetention,
      logRemovalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      asgName: this.asgName,
      minCapacity: minimumApiCapacity(environment),
      maxCapacity: minimumApiCapacity(environment),
      // The ASG runs only inside a narrow daytime window: up at 11:55 and down
      // at 13:15 America/Sao_Paulo. Outside it the service is off — inbound
      // webhooks fail and nothing is reachable. Deliberate for a development
      // environment on a single t4g.nano.
      // schedule: {enableCron: '55 11 * * *', disableCron: '15 13 * * *'},
      spot: {}
    });
    const asg = service.autoScalingGroup;
    asg.node.addDependency(profile);

    // ASG termination pauses before EC2 shutdown, asks systemd to stop the
    // process (which runs Fx OnStop -> DrainAndRelease), then explicitly
    // completes the lifecycle action. The finally block fails open so a
    // broken SSM agent can never strand an instance in Terminating:Wait.
    const drainFunction = new lambda.Function(this, 'TerminationDrainFunction', {
      functionName: `${environment}-${SERVICE}-termination-drain`,
      runtime: lambda.Runtime.PYTHON_3_14,
      handler: 'index.handler',
      timeout: cdk.Duration.seconds(90),
      code: lambda.Code.fromInline(`
import boto3, json, time
asg = boto3.client("autoscaling")
ssm = boto3.client("ssm")

def handler(event, context):
    message = json.loads(event["Records"][0]["Sns"]["Message"])
    instance_id = message["EC2InstanceId"]
    try:
        result = ssm.send_command(
            InstanceIds=[instance_id],
            DocumentName="AWS-RunShellScript",
            Parameters={"commands": ["systemctl stop app"]},
            TimeoutSeconds=55,
        )
        command_id = result["Command"]["CommandId"]
        deadline = time.time() + 55
        while time.time() < deadline:
            try:
                status = ssm.get_command_invocation(
                    CommandId=command_id, InstanceId=instance_id)["Status"]
                if status in ("Success", "Cancelled", "Failed", "TimedOut"):
                    break
            except ssm.exceptions.InvocationDoesNotExist:
                pass
            time.sleep(2)
    finally:
        asg.complete_lifecycle_action(
            LifecycleHookName=message["LifecycleHookName"],
            AutoScalingGroupName=message["AutoScalingGroupName"],
            LifecycleActionToken=message["LifecycleActionToken"],
            LifecycleActionResult="CONTINUE",
        )
`),
    });
    drainFunction.addToRolePolicy(new iam.PolicyStatement({
      actions: ['ssm:SendCommand'],
      resources: [
        `arn:${cdk.Aws.PARTITION}:ssm:${this.region}::document/AWS-RunShellScript`,
        `arn:${cdk.Aws.PARTITION}:ec2:${this.region}:${this.account}:instance/*`,
      ],
    }));
    drainFunction.addToRolePolicy(new iam.PolicyStatement({
      actions: ['ssm:GetCommandInvocation'],
      resources: ['*'],
    }));
    drainFunction.addToRolePolicy(new iam.PolicyStatement({
      actions: ['autoscaling:CompleteLifecycleAction'],
      resources: [asg.autoScalingGroupArn],
    }));
    asg.addLifecycleHook('TerminationDrainHook', {
      lifecycleHookName: `${environment}-${SERVICE}-termination-drain`,
      lifecycleTransition: autoscaling.LifecycleTransition.INSTANCE_TERMINATING,
      defaultResult: autoscaling.DefaultResult.CONTINUE,
      heartbeatTimeout: cdk.Duration.seconds(120),
      notificationTarget: new hooktargets.FunctionHook(drainFunction),
    });

    // DynamoDB access for internal/tablestore.Store — TransactWriteItems is
    // required because every commit (CommitAction) writes the state item,
    // the audit-log entry, and (for player actions) the idempotency guard in
    // one transaction (ARCHITECTURE.md §2, revised: conditional writes are
    // the correctness mechanism).
    //
    // The role and instance profile are owned by this stack so permissions
    // evolve together with the API's storage and runtime configuration.
    new cdk.CfnOutput(this, 'TableStateArn', {value: tableStateArn, exportName: `${id}-table-state-arn`});
    new cdk.CfnOutput(this, 'ActionLogArn', {value: actionLogArn, exportName: `${id}-action-log-arn`});
    new cdk.CfnOutput(this, 'ActionGuardsArn', {value: actionGuardsArn, exportName: `${id}-action-guards-arn`});
    new cdk.CfnOutput(this, 'RoomsTableArn', {value: roomsTableArn, exportName: `${id}-rooms-table-arn`});
    new cdk.CfnOutput(this, 'WalletUrlParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${walletUrlParam}`,
      exportName: `${id}-wallet-url-parameter-arn`,
    });
    new cdk.CfnOutput(this, 'PokerClientIdParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${pokerClientIdParam}`,
      exportName: `${id}-poker-client-id-parameter-arn`,
    });
    new cdk.CfnOutput(this, 'PokerClientSecretParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${pokerClientSecretParam}`,
      exportName: `${id}-poker-client-secret-parameter-arn`,
    });
    new cdk.CfnOutput(this, 'RealMoneyEnabledParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${realMoneyEnabledParam}`,
      exportName: `${id}-real-money-enabled-parameter-arn`,
    });
    new cdk.CfnOutput(this, 'SocialGraphEnabledParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${socialGraphEnabledParam}`,
      exportName: `${id}-social-graph-enabled-parameter-arn`,
    });
    new cdk.CfnOutput(this, 'LegalSignoffRefParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${legalSignoffRefParam}`,
      exportName: `${id}-legal-signoff-ref-parameter-arn`,
    });

    // ── Outputs ───────────────────────────────────────────────────────────────
    new cdk.CfnOutput(this, 'AsgName', {value: asg.autoScalingGroupName, exportName: `${id}-asg-name`});
    new cdk.CfnOutput(this, 'AppLogGroupName', {
      value: service.appLogGroup.logGroupName,
      exportName: `${id}-app-log-group`,
    });
    new cdk.CfnOutput(this, 'NginxLogGroupName', {
      value: service.nginxLogGroup!.logGroupName,
      exportName: `${id}-nginx-log-group`,
    });
  }
}

import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import * as autoscaling from 'aws-cdk-lib/aws-autoscaling';
import * as hooktargets from 'aws-cdk-lib/aws-autoscaling-hooktargets';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import {Construct} from 'constructs';
import {
  buildCloudWatchAgentConfig,
  Ec2ScriptRunner,
  Environment,
  HaproxyEc2Service,
} from '@aoctech/cdk';
import {
  API_CURRENT_ARTIFACT_KEY,
  APP_PORT,
  asgName,
  HEALTH_CHECK_PATH,
  instanceRoleName,
  operationsDashboardName,
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
  socialEdgesTableArn: string;
  recentPlayersTableArn: string;
  socialEventsTableArn: string;
  playerReportsTableArn: string;
  socialGraphEnabledParam: string;
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
      socialEdgesTableArn,
      recentPlayersTableArn,
      socialEventsTableArn,
      playerReportsTableArn,
      socialGraphEnabledParam,
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
      pendingCashoutsTableArn, reactionEntitlementsTableArn, reactionPurchasesTableArn,
      socialEdgesTableArn, recentPlayersTableArn, socialEventsTableArn, playerReportsTableArn,
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
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:PutObject', 's3:DeleteObject'],
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

    userData.addCommands(
      // Generated rather than shipped: the log group name and metric namespace are
      // CloudFormation values. {instance_id} is resolved by the CW agent at
      // runtime, not by bash.
      `cat > /tmp/cwagent.json << 'CWA'`,
      buildCloudWatchAgentConfig({
        metricNamespace: `CtechPoker/${environment}/Host`,
        appProcessPattern: '/opt/app/current/(app|bootstrap)',
        logFiles: [
          {filePath: '/var/log/app/app.log', logGroupName: logGroupApp, logStreamName: '{instance_id}'},
        ],
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
      maxCapacity: isProd ? 3 : 1,
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
    
    const alarmMetricFilter = service.appLogGroup.addMetricFilter('AlarmLogFilter', {
      filterPattern: logs.FilterPattern.literal('"ALARM:"'),
      metricNamespace: `CtechPoker/${environment}`,
      metricName: 'AlarmLogLines',
      metricValue: '1',
    });
    new cloudwatch.Alarm(this, 'AlarmLogAlarm', {
      alarmName: `${environment}-${SERVICE}-alarm-log-lines`,
      alarmDescription: 'An ALARM log line was emitted (reconcile credit failure or manual review condition).',
      metric: alarmMetricFilter.metric({statistic: 'Sum', period: cdk.Duration.minutes(5)}),
      threshold: 1,
      evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
    
    const leaseFailoverMetric = new cloudwatch.Metric({
      namespace: `CtechPoker/${environment}`,
      metricName: 'LeaseFailovers',
      statistic: 'Sum',
      period: cdk.Duration.minutes(5),
    });
    new cloudwatch.Alarm(this, 'LeaseFailoverSpikeAlarm', {
      alarmName: `${environment}-${SERVICE}-lease-failover-spike`,
      alarmDescription: 'Table lease failovers spiked — earliest signal of an instance going bad.',
      metric: leaseFailoverMetric,
      threshold: 5,
      evaluationPeriods: 2,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    const playerReportedMetric = new cloudwatch.Metric({
      namespace: `CtechPoker/${environment}`,
      metricName: 'PlayerReported',
      statistic: 'Sum',
      period: cdk.Duration.minutes(5),
    });
    new cloudwatch.Alarm(this, 'PlayerReportSpikeAlarm', {
      alarmName: `${environment}-${SERVICE}-player-report-spike`,
      alarmDescription: 'Player reports exceeded the initial moderation baseline; triage the open queue.',
      metric: playerReportedMetric,
      threshold: 20,
      evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    const socialRateLimitedMetric = new cloudwatch.Metric({
      namespace: `CtechPoker/${environment}`,
      metricName: 'SocialRateLimited',
      statistic: 'Sum',
      period: cdk.Duration.minutes(5),
    });
    new cloudwatch.Alarm(this, 'SocialRateLimitSpikeAlarm', {
      alarmName: `${environment}-${SERVICE}-social-rate-limit-spike`,
      alarmDescription: 'Social/report throttling exceeded the initial abuse baseline.',
      metric: socialRateLimitedMetric,
      threshold: 25,
      evaluationPeriods: 2,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    // One low-cost operational view for the gameplay SLOs. SEARCH expressions
    // intentionally aggregate bounded dimensions (route/status/version) and
    // legacy table_id series; no per-table widget or alarm is created.
    const namespace = `CtechPoker/${environment}`;
    const search = (metricName: string, statistic: string = 'Sum') => new cloudwatch.MathExpression({
      expression: `SEARCH('{${namespace}} MetricName="${metricName}"', '${statistic}', 300)`,
      period: cdk.Duration.minutes(5),
    });
    const dashboard = new cloudwatch.Dashboard(this, 'OperationsDashboard', {
      dashboardName: operationsDashboardName(environment),
      defaultInterval: cdk.Duration.hours(6),
    });
    dashboard.addWidgets(
      new cloudwatch.TextWidget({
        width: 24,
        height: 2,
        markdown: '# CTech Poker — gameplay, transport and money-movement SLOs',
      }),
      new cloudwatch.GraphWidget({
        title: 'Action → ACK latency (p95) and successful actions',
        width: 12,
        left: [search('ActionLatencyMs', 'p95')],
        right: [search('ActionsSucceeded')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Reconnects and time to authoritative snapshot (p95)',
        width: 12,
        left: [search('SnapshotLatencyMs', 'p95')],
        right: [search('Disconnects')],
      }),
      new cloudwatch.GraphWidget({
        title: 'DynamoDB conflicts and persistence errors',
        width: 12,
        left: [search('DynamoDBVersionConflicts'), search('TableStateHistorySaveError')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Pending money movements (count and oldest age)',
        width: 12,
        left: [search('PendingCashouts', 'Maximum')],
        right: [search('OldestPendingCashoutAgeSeconds', 'Maximum')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Actors, connections, mailbox pressure and lease failovers',
        width: 12,
        left: [search('ConnectionsOpened'), search('ConnectionsClosed'), search('ActorsCreated'), search('ActorsRemoved')],
        right: [search('MailboxBackpressure'), search('LeaseFailovers')],
      }),
      new cloudwatch.GraphWidget({
        title: 'HTTP auth and throttling responses by route/version',
        width: 12,
        left: [search('HTTPResponses')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Player-safety reports and social throttling',
        width: 12,
        left: [search('PlayerReported')],
        right: [search('SocialRateLimited')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Wallet dependency: latency, retries and circuit breaker',
        width: 12,
        left: [search('WalletLatencyMs', 'p95')],
        right: [search('WalletRetries'), search('WalletCircuitOpened'), search('WalletCircuitOpenRejected')],
      }),
      new cloudwatch.GraphWidget({
        title: 'Equity compute duration and cache behavior',
        width: 12,
        left: [search('EquityDurationMs', 'p95')],
        right: [search('EquityCacheHits'), search('EquityCacheMisses'), search('EquityCacheEvictions')],
      }),
    );
    
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

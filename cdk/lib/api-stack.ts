import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import * as autoscaling from 'aws-cdk-lib/aws-autoscaling';
import * as hooktargets from 'aws-cdk-lib/aws-autoscaling-hooktargets';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import {Construct} from 'constructs';
import {
  addCloudWatchAgentDualStackOverride,
  addDualStackSsmAgentCommands,
  addSwapCommands,
  Environment,
  PrivateIpv4Ec2Service,
} from '@aoctech/cdk';
import {
  ALB_LISTENER_PRIORITY,
  API_CURRENT_ARTIFACT_KEY,
  APP_PORT,
  asgName,
  HEALTH_CHECK_PATH,
  instanceRoleName,
  operationsDashboardName,
  S3_PREFIX,
  SERVICE,
  SSM_SHARED,
} from './constants';

interface ApiStackProps extends cdk.StackProps {
  environment: Environment;
  // Must be a concrete string (not a token): ec2.Vpc.fromLookup resolves
  // subnet/AZ metadata at synthesis time. CI reads /ctech/{env}/network/vpc-id
  // from SSM into CTECH_VPC_ID before running cdk deploy (see ctech-cdk/CLAUDE.md
  // "Known Constraints").
  vpcId: string;
  /** ALB host header, e.g. poker-api-dev.aoctech.app. */
  domainName: string;
  /** Browser app host, used for CORS and the JWT audience. */
  appDomainName: string;
  /** CTech Account issuer host. */
  authDomainName: string;
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
}

export const minimumApiCapacity = (environment: Environment) => environment === 'prod' ? 2 : 1;

export class PokerApiStack extends cdk.Stack {
  public readonly asgName: string;
  
  constructor(scope: Construct, id: string, props: ApiStackProps) {
    super(scope, id, props);
    
    const {
      environment,
      vpcId,
      domainName,
      appDomainName,
      authDomainName,
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
    } = props;
    
    const shared = SSM_SHARED(environment);

    new ssm.StringParameter(this, 'AvatarBaseUrlParameter', {
      parameterName: avatarBaseUrlParam,
      stringValue: `https://${appDomainName}/avatars`,
      description: 'Public same-origin base URL for versioned poker avatars',
    });
    
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
      playerHandsTableArn, playerNotesTableArn, handSharesTableArn, pokerStatsTableArn,
    ];
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: [
        'dynamodb:GetItem',
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
        realMoneyEnabledParam, legalSignoffRefParam,
        avatarBaseUrlParam,
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
    const albSg = ec2.SecurityGroup.fromSecurityGroupId(this, 'AlbSg', albSgId);
    
    const httpsListenerArn = ssm.StringParameter.valueForStringParameter(
      this, shared.httpsListenerArn,
    );
    const httpsListener = elbv2.ApplicationListener.fromApplicationListenerAttributes(
      this, 'HttpsListener',
      {listenerArn: httpsListenerArn, securityGroup: albSg},
    );
    
    const isProd = environment === 'prod';
    this.asgName = asgName(environment);
    const logRetention: logs.RetentionDays = isProd ? logs.RetentionDays.ONE_MONTH : logs.RetentionDays.ONE_WEEK;
    const logGroupApp = `/${SERVICE}/${environment}/app`;
    // No nginx in this skeleton (see APP_PORT doc comment in constants.ts) — this
    // log group stays empty until a future task ships JSON access logs to it, but
    // PrivateIpv4Ec2Service requires a name regardless (its HTTP2XX-5XX metric
    // filters attach here).
    const logGroupNginx = `/${SERVICE}/${environment}/nginx`;
    
    // ── User Data ─────────────────────────────────────────────────────────────
    const userData = ec2.UserData.forLinux();
    
    userData.addCommands(
      // ── Packages + directories ───────────────────────────────────────────────
      'dnf install -y amazon-cloudwatch-agent amazon-ssm-agent unzip jq',
      'useradd --system --no-create-home --shell /sbin/nologin webapp',
      'mkdir -p /opt/app/releases /var/log/app',
      'chown -R webapp:webapp /opt/app /var/log/app',
    );
    
    addSwapCommands(userData);
    addDualStackSsmAgentCommands(userData);
    addCloudWatchAgentDualStackOverride(userData);
    
    userData.addCommands(
      // {instance_id} is resolved by the CW agent at runtime, not by bash.
      `cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json << 'CWA'`,
      `{`,
      `  "logs": {`,
      `    "logs_collected": {`,
      `      "files": {`,
      `        "collect_list": [`,
      `          {"file_path":"/var/log/app/app.log","log_group_name":"${logGroupApp}","log_stream_name":"{instance_id}"}`,
      `        ]`,
      `      }`,
      `    }`,
      `  }`,
      `}`,
      `CWA`,
      `/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json -s`,
      
      // ── Static env file (loaded by systemd EnvironmentFile=) ─────────────────
      // CDK tokens are substituted at synthesis time; bash does not expand them.
      // Only non-secret values live here. Secrets (none yet) would come from SSM
      // in start.sh, same convention as ctech-wallet/ctech-dfe.
      `cat > /etc/app-static.env << 'ENV'`,
      `ENVIRONMENT=${environment}`,
      `AWS_REGION=${this.region}`,
      `AWS_USE_DUALSTACK_ENDPOINT=true`,
      `PORT=${APP_PORT}`,
      `SERVICE_AUDIENCE=https://${appDomainName}`,
      `CTECH_URL=https://${authDomainName}`,
      // Poker is reached directly from the ALB, with no localhost nginx hop.
      // Trust only peers inside this VPC before honoring X-Forwarded-For.
      `TRUSTED_PROXIES=${vpc.vpcCidrBlock}`,
      `CORS_ALLOWED_ORIGINS=https://${appDomainName}`,
      `TURNSTILE_EXPECTED_HOSTNAME=${appDomainName}`,
      `AVATAR_BUCKET=${avatarsBucketName}`,
      `ENV`,
      
      // ── start.sh: fetches runtime configuration and M2M credentials from SSM.
      // No DB-number suffix — see constants.ts SSM_SHARED doc comment: ctech-dfe and
      // ctech-account both pass VALKEY_URL straight through unmodified, and
      // tablelease keys are already namespaced by prefix (table:{id}).
      `cat > /opt/app/start.sh << 'START'`,
      `#!/bin/bash`,
      // APP_VERSION ships inside the release artifact (release.env), written by CI.
      `if [ -f /opt/app/current/release.env ]; then set -a; . /opt/app/current/release.env; set +a; fi`,
      // Falls back to empty → config.Load() fails closed in prod (VALKEY_URL
      // required there); in dev/stage the app falls back to an in-memory backend.
      `VALKEY_URL=$(aws ssm get-parameter --name "${shared.valkeyUrl}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export VALKEY_URL`,
      `WALLET_URL=$(aws ssm get-parameter --name "${walletUrlParam}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export WALLET_URL`,
      `POKER_CLIENT_ID=$(aws ssm get-parameter --name "${pokerClientIdParam}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export POKER_CLIENT_ID`,
      `POKER_CLIENT_SECRET=$(aws ssm get-parameter --name "${pokerClientSecretParam}" --with-decryption --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export POKER_CLIENT_SECRET`,
      `TURNSTILE_SECRET=$(aws ssm get-parameter --name "${turnstileSecretParam}" --with-decryption --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export TURNSTILE_SECRET`,
      // Real-money kill switch: both fetched fresh on every boot/restart so
      // ops can flip them via SSM without a redeploy. Falls back to
      // false/empty → config.Load() keeps real-money mode off (fails closed).
      `REAL_MONEY_ENABLED=$(aws ssm get-parameter --name "${realMoneyEnabledParam}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "false")`,
      `export REAL_MONEY_ENABLED`,
      `LEGAL_SIGNOFF_REF=$(aws ssm get-parameter --name "${legalSignoffRefParam}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export LEGAL_SIGNOFF_REF`,
      `AVATAR_BASE_URL=$(aws ssm get-parameter --name "${avatarBaseUrlParam}" --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export AVATAR_BASE_URL`,
      `exec /opt/app/current/app`,
      `START`,
      `chmod +x /opt/app/start.sh`,
      
      // ── systemd app.service ──────────────────────────────────────────────────
      `cat > /etc/systemd/system/app.service << 'SVC'`,
      `[Unit]`,
      `Description=CTech Poker API`,
      `After=network.target`,
      `StartLimitIntervalSec=300`,
      `StartLimitBurst=5`,
      ``,
      `[Service]`,
      `User=webapp`,
      `Group=webapp`,
      `WorkingDirectory=/opt/app/current`,
      `Environment=HOME=/opt/app`,
      `EnvironmentFile=/etc/app-static.env`,
      `ExecStartPre=/bin/test -x /opt/app/current/app`,
      `ExecStart=/opt/app/start.sh`,
      `StandardOutput=append:/var/log/app/app.log`,
      `StandardError=append:/var/log/app/app.log`,
      `Restart=on-failure`,
      `RestartSec=30`,
      ``,
      `[Install]`,
      `WantedBy=multi-user.target`,
      `SVC`,
      `systemctl daemon-reload`,
      `systemctl enable app`,
      
      // ── deploy.sh: called by SSM RunCommand from GitHub Actions ──────────────
      // Expects a zip containing a pre-built `app` binary (linux/arm64).
      // __BUCKET__ is replaced by sed so bash $variables are not expanded at write
      // time (quoted 'DEPLOY' delimiter).
      `cat > /opt/app/deploy.sh << 'DEPLOY'`,
      `#!/bin/bash`,
      `set -euo pipefail`,
      `S3_KEY="$1"`,
      `RELEASE_DIR="/opt/app/releases/$(date +%Y%m%d_%H%M%S)"`,
      `mkdir -p "$RELEASE_DIR"`,
      `echo "Downloading release: $S3_KEY"`,
      `aws s3 cp "s3://__BUCKET__/$S3_KEY" /tmp/release.zip`,
      `unzip -o /tmp/release.zip -d "$RELEASE_DIR"`,
      `chmod +x "$RELEASE_DIR/app"`,
      `chown -R webapp:webapp "$RELEASE_DIR"`,
      `ln -sfT "$RELEASE_DIR" /opt/app/current`,
      `systemctl restart app 2>/dev/null || systemctl start app`,
      `for i in {1..60}; do`,
      // No nginx in front — the health check hits the app's own port directly.
      `  if curl -sf http://127.0.0.1:${APP_PORT}${HEALTH_CHECK_PATH} >/dev/null; then`,
      `    echo "Health check passed"`,
      `    break`,
      `  fi`,
      `  if systemctl is-failed --quiet app; then`,
      `    echo "Application failed to start"`,
      `    journalctl -u app --no-pager -n 100 || true`,
      `    exit 1`,
      `  fi`,
      `  sleep 2`,
      `done`,
      `curl -sf http://127.0.0.1:${APP_PORT}${HEALTH_CHECK_PATH} >/dev/null || {`,
      `  echo "Timed out waiting for health check"`,
      `  exit 1`,
      `}`,
      `ls -dt /opt/app/releases/*/ 2>/dev/null | tail -n +2 | xargs rm -rf 2>/dev/null || true`,
      `echo "Deployment successful"`,
      `DEPLOY`,
      `sed -i 's|__BUCKET__|${deploymentsBucketName}|g' /opt/app/deploy.sh`,
      `chmod +x /opt/app/deploy.sh`,
      
      // ── upload-logs.sh: bundles rotated logs and ships to S3 ─────────────────
      // IMDSv2 token required (requireImdsv2 is enforced on this instance).
      `cat > /opt/app/upload-logs.sh << 'UPLOAD'`,
      `#!/bin/bash`,
      `TOKEN=$(curl -sf -X PUT "http://169.254.169.254/latest/api/token" \\`,
      `    -H "X-aws-ec2-metadata-token-ttl-seconds: 60")`,
      `INSTANCE_ID=$(curl -sf -H "X-aws-ec2-metadata-token: $TOKEN" \\`,
      `    "http://169.254.169.254/latest/meta-data/instance-id" || echo "unknown")`,
      `DATE=$(date +%Y%m%d)`,
      `BUCKET="__LOG_BUCKET__"`,
      `ARCHIVE="/tmp/\${DATE}-\${INSTANCE_ID}.tar.gz"`,
      `ROTATED=$(find /var/log/app -name "*-\${DATE}.gz" 2>/dev/null)`,
      `[ -z "$ROTATED" ] && exit 0`,
      `tar czf "$ARCHIVE" $ROTATED 2>/dev/null || exit 0`,
      `aws s3 cp "$ARCHIVE" "s3://\${BUCKET}/${S3_PREFIX}/\${DATE}-\${INSTANCE_ID}.tar.gz" --region ${this.region} || exit 0`,
      `find /var/log/app -name "*-\${DATE}.gz" -delete`,
      `rm -f "$ARCHIVE"`,
      `UPLOAD`,
      `sed -i 's|__LOG_BUCKET__|${logsBucketName}|g' /opt/app/upload-logs.sh`,
      `chmod +x /opt/app/upload-logs.sh`,
      
      // ── logrotate: daily, gzip, copytruncate, ship to S3 ─────────────────────
      `cat > /etc/logrotate.d/${SERVICE} << 'LOGROTATE'`,
      `/var/log/app/app.log {`,
      `    daily`,
      `    compress`,
      `    copytruncate`,
      `    missingok`,
      `    notifempty`,
      `    dateext`,
      `    dateformat -%Y%m%d`,
      `    rotate 1`,
      `    sharedscripts`,
      `    postrotate`,
      `        /opt/app/upload-logs.sh`,
      `    endscript`,
      `}`,
      `LOGROTATE`,
      
      // ── Bootstrap: deploy current.zip if it already exists in S3 ─────────────
      `aws s3api head-object --bucket "${deploymentsBucketName}" --key "${API_CURRENT_ARTIFACT_KEY}" 2>/dev/null && /opt/app/deploy.sh ${API_CURRENT_ARTIFACT_KEY} || echo "No bootstrap artifact, waiting for first deploy"`,
    );
    
    // ── Shared no-NAT-Gateway EC2/ASG pattern (@aoctech/cdk) ───────────────────
    // Priority must be unique across services: 15=dfe, 25=account, 35=wallet, 45=poker.
    const service = new PrivateIpv4Ec2Service(this, 'ApiService', {
      vpc,
      albSg,
      httpsListener,
      securityGroupName: `${environment}-${SERVICE}-api-sg`,
      securityGroupDescription: 'ctech-poker API instances',
      appPort: APP_PORT,
      instanceProfileName,
      userData,
      logGroupAppName: logGroupApp,
      logGroupNginxName: logGroupNginx,
      logRetention,
      logRemovalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      metricNamespace: `CtechPoker/${environment}`,
      targetGroupName: `${this.asgName}-tg`,
      healthCheckPath: HEALTH_CHECK_PATH,
      asgName: this.asgName,
      minCapacity: minimumApiCapacity(environment),
      maxCapacity: isProd ? 3 : 1,
      domainName,
      listenerRulePriority: ALB_LISTENER_PRIORITY,
    });
    if (isProd) {
      service.autoScalingGroup.scaleOnCpuUtilization('CpuTargetTracking', {
        targetUtilizationPercent: 60,
        cooldown: cdk.Duration.minutes(3),
      });
    }
    service.autoScalingGroup.node.addDependency(profile);
    service.targetGroup.setAttribute('deregistration_delay.timeout_seconds', '60');

    // ASG termination pauses before EC2 shutdown, asks systemd to stop the
    // process (which runs Fx OnStop -> DrainAndRelease), then explicitly
    // completes the lifecycle action. The finally block fails open so a
    // broken SSM agent can never strand an instance in Terminating:Wait.
    const drainFunction = new lambda.Function(this, 'TerminationDrainFunction', {
      functionName: `${environment}-${SERVICE}-termination-drain`,
      runtime: lambda.Runtime.PYTHON_3_13,
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
      resources: [service.autoScalingGroup.autoScalingGroupArn],
    }));
    service.autoScalingGroup.addLifecycleHook('TerminationDrainHook', {
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
        title: 'ALB target health and response latency',
        width: 12,
        left: [
          service.targetGroup.metricHealthyHostCount({statistic: 'Minimum'}),
          service.targetGroup.metricUnhealthyHostCount({statistic: 'Maximum'}),
        ],
        right: [service.targetGroup.metricTargetResponseTime({statistic: 'p95'})],
      }),
      new cloudwatch.GraphWidget({
        title: 'ALB traffic and target 5xx',
        width: 12,
        left: [service.targetGroup.metricRequestCount({statistic: 'Sum'})],
        right: [service.targetGroup.metricHttpCodeTarget(elbv2.HttpCodeTarget.TARGET_5XX_COUNT, {statistic: 'Sum'})],
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
    new cdk.CfnOutput(this, 'LegalSignoffRefParameterArn', {
      value: `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${legalSignoffRefParam}`,
      exportName: `${id}-legal-signoff-ref-parameter-arn`,
    });
    
    // ── Outputs ───────────────────────────────────────────────────────────────
    new cdk.CfnOutput(this, 'AsgName', {value: service.asgName, exportName: `${id}-asg-name`});
    new cdk.CfnOutput(this, 'AppLogGroupName', {
      value: service.appLogGroup.logGroupName,
      exportName: `${id}-app-log-group`,
    });
    new cdk.CfnOutput(this, 'NginxLogGroupName', {
      value: service.nginxLogGroup.logGroupName,
      exportName: `${id}-nginx-log-group`,
    });
  }
}

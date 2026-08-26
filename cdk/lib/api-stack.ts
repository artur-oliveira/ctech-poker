import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as autoscaling from 'aws-cdk-lib/aws-autoscaling';
import * as hooktargets from 'aws-cdk-lib/aws-autoscaling-hooktargets';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import {Construct} from 'constructs';
import {Ec2ScriptRunner, Environment, HaproxyEc2Service, SSM as CtechSSM} from '@aoctech/cdk';
import {
  API_CURRENT_ARTIFACT_KEY,
  APP_PORT,
  APP_PORT_ALT,
  asgName,
  HEALTH_CHECK_PATH,
  instanceRoleName,
  NGINX_PORT,
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
  highlightsTableArn: string;
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
  handRevealsTableArn: string;
  playerMatchupsTableArn: string;
  walletWebhookHmacSecretParam: string;
  sandboxPurchasesTableArn: string;
  pendingCashoutsTableArn: string;
  reactionEntitlementsTableArn: string;
  reactionPurchasesTableArn: string;
  cosmeticEntitlementsTableArn: string;
  cosmeticPurchasesTableArn: string;
  tableEntitlementsTableArn: string;
  socialEdgesTableArn: string;
  recentPlayersTableArn: string;
  socialEventsTableArn: string;
  playerReportsTableArn: string;
  socialGraphEnabledParam: string;
  // Session Manager. **On**: CI deploys over SSM RunCommand (/opt/app/deploy.sh),
  // and the termination-drain lifecycle hook stops the app through RunCommand
  // too — with the agent off, draining fails open and instances terminate
  // without releasing table leases.
  enableSsmAgent?: boolean;
  // 'alpine' pilots the same ctech-billing/ctech-account/ctech-wallet custom
  // AMI + OpenRC pattern here. Default 'alpine'; 'al2023' is the one-line
  // rollback.
  osFamily?: 'al2023' | 'alpine';
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
      highlightsTableArn,
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
      handRevealsTableArn,
      playerMatchupsTableArn,
      dailyRewardTableArn,
      walletWebhookHmacSecretParam,
      sandboxPurchasesTableArn,
      pendingCashoutsTableArn,
      reactionEntitlementsTableArn,
      reactionPurchasesTableArn,
      cosmeticPurchasesTableArn,
      cosmeticEntitlementsTableArn,
      tableEntitlementsTableArn,
      socialEdgesTableArn,
      recentPlayersTableArn,
      socialEventsTableArn,
      playerReportsTableArn,
      socialGraphEnabledParam,
      enableSsmAgent = false,
      osFamily = 'alpine',
    } = props;
    const isAlpine = osFamily === 'alpine';

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
      playerHandsTableArn, handRevealsTableArn, playerMatchupsTableArn,
      playerNotesTableArn, handSharesTableArn, pokerStatsTableArn, highlightsTableArn, sandboxPurchasesTableArn,
      pendingCashoutsTableArn, reactionEntitlementsTableArn, reactionPurchasesTableArn, cosmeticEntitlementsTableArn,
      cosmeticPurchasesTableArn, tableEntitlementsTableArn, socialEdgesTableArn, recentPlayersTableArn, socialEventsTableArn, playerReportsTableArn,
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
    // update-realip.sh reads the AWS-managed CloudFront origin-facing prefix
    // list. Both actions are read-only and do not support resource-level
    // permissions, so Resource must be *. Missing until now — setup-realip.sh
    // has been failing this call since it was added (confirmed live:
    // UnauthorizedOperation on ec2:DescribeManagedPrefixLists), same permission
    // ctech-account/ctech-wallet already grant.
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ec2:DescribeManagedPrefixLists', 'ec2:GetManagedPrefixListEntries'],
      resources: ['*'],
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
    const userData = ec2.UserData.forLinux();
    let scripts: Ec2ScriptRunner | undefined;

    if (isAlpine) {
      const scriptsBucket = ssm.StringParameter.valueForStringParameter(
        this, CtechSSM.ec2ScriptsAlpine(environment).bucket,
      );
      const scriptsVersion = ssm.StringParameter.valueForStringParameter(
        this, CtechSSM.ec2ScriptsAlpine(environment).version,
      );
      instanceRole.addToPolicy(new iam.PolicyStatement({
        actions: ['s3:GetObject'],
        resources: [`arn:${cdk.Aws.PARTITION}:s3:::${scriptsBucket}/*`],
      }));
      userData.addCommands(
        'export AWS_USE_DUALSTACK_ENDPOINT=true',
        `CTECH_SCRIPTS_BUCKET="${scriptsBucket}"`,
        `CTECH_SCRIPTS_VERSION="${scriptsVersion}"`,
        'ctech_run(){ s=$1; shift; ctech-ec2-agent s3-cp -bucket "$CTECH_SCRIPTS_BUCKET" -key "$CTECH_SCRIPTS_VERSION/$s" -dest "/tmp/$s"; bash "/tmp/$s" "$@"; }',
      );
      userData.addCommands(`ctech_run setup-base.sh ${SERVICE} nginx nginx-openrc`);
      userData.addCommands('ctech_run setup-swap.sh 256');
      userData.addCommands('ctech_run setup-dualstack.sh');
      userData.addCommands('ctech_run setup-cloudflare-ca.sh');
      if (!enableSsmAgent) {
        userData.addCommands('rc-service amazon-ssm-agent stop 2>/dev/null || true', 'rc-update del amazon-ssm-agent default 2>/dev/null || true');
      }
    } else {
      scripts = new Ec2ScriptRunner(this, 'Scripts', {environment});
      scripts.grantRead(instanceRole);
      scripts.install(userData);

      scripts.run(userData, 'setup-base.sh', SERVICE, 'nginx');
      scripts.run(userData, 'setup-swap.sh', '256');
      scripts.run(userData, 'setup-dualstack.sh');
      scripts.run(userData, 'setup-cloudflare-ca.sh');

      // setup-base.sh installs the SSM agent and setup-dualstack.sh starts it, so
      // this is what stops it again. See enableSsmAgent above: off also disables
      // the graceful table drain on termination.
      if (!enableSsmAgent) {
        userData.addCommands('systemctl disable --now amazon-ssm-agent 2>/dev/null || true');
      }
    }

    // /etc/app-static.env: non-secret values systemd loads via EnvironmentFile.
    // CDK tokens are substituted at synthesis; bash does not expand them.
    userData.addCommands(
      `cat > /etc/app-static.env << 'ENV'`,
      `ENVIRONMENT=${environment}`,
      `AWS_REGION=${this.region}`,
      `AWS_USE_DUALSTACK_ENDPOINT=true`,
      `PORT=${APP_PORT}`,
      // nginx is now the only caller reaching the app — same as every other
      // CTech service fronted by it.
      `TRUSTED_PROXIES=127.0.0.1`,
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
    const ssmEnvArgs = [
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
    ];
    if (isAlpine) {
      const quoted = ssmEnvArgs.map((a) => `'${a.replace(/'/g, `'\\''`)}'`).join(' ');
      userData.addCommands(`ctech_run setup-ssm-env.sh ${quoted}`);
    } else {
      scripts!.run(userData, 'setup-ssm-env.sh', ...ssmEnvArgs);
    }

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

    // The app's WebSocket upgraders reject a request whose Upgrade/Connection
    // headers were not forwarded. $connection_upgrade comes from the map in
    // the shared nginx.conf. Two locations: the general socket and the
    // per-table one (tableId varies, hence the regex).
    userData.addCommands(
      `cat > /etc/nginx/conf.d/location-ws.conf << 'WSLOC'`,
      `location = /v1.0/ws {`,
      `    proxy_pass http://app;`,
      `    proxy_http_version 1.1;`,
      `    proxy_set_header Upgrade $http_upgrade;`,
      `    proxy_set_header Connection $connection_upgrade;`,
      `    proxy_set_header Host $host;`,
      `    proxy_set_header X-Real-IP $remote_addr;`,
      `    proxy_set_header X-Forwarded-For $remote_addr;`,
      `    proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;`,
      `    proxy_read_timeout 3600s;`,
      `    proxy_send_timeout 3600s;`,
      `    proxy_buffering off;`,
      `}`,
      `location ~ ^/v1\\.0/tables/[^/]+/ws$ {`,
      `    proxy_pass http://app;`,
      `    proxy_http_version 1.1;`,
      `    proxy_set_header Upgrade $http_upgrade;`,
      `    proxy_set_header Connection $connection_upgrade;`,
      `    proxy_set_header Host $host;`,
      `    proxy_set_header X-Real-IP $remote_addr;`,
      `    proxy_set_header X-Forwarded-For $remote_addr;`,
      `    proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;`,
      `    proxy_read_timeout 3600s;`,
      `    proxy_send_timeout 3600s;`,
      `    proxy_buffering off;`,
      `}`,
      `WSLOC`,
    );

    if (isAlpine) {
      userData.addCommands(`ctech_run setup-realip.sh '${vpc.vpcCidrBlock}'`);
      // app-port-alt/alt-port turn on the zero-downtime rolling deploy: a second
      // app process nginx round-robins into, so deploy.sh can restart one unit
      // at a time instead of dropping the health check during a restart.
      userData.addCommands(`ctech_run setup-nginx.sh ${NGINX_PORT} ${APP_PORT} ${HEALTH_CHECK_PATH} 100 1m ${APP_PORT_ALT}`);
      // Alpine's setup-app-service.sh has no After=-units argument — OpenRC
      // services here only ever declare `need net`.
      userData.addCommands(`ctech_run setup-app-service.sh 'CTech Poker API' app ${APP_PORT_ALT}`);
      userData.addCommands(
        `ctech_run setup-deploy.sh ${deploymentsBucketName} app 'http://127.0.0.1:${NGINX_PORT}${HEALTH_CHECK_PATH}'`,
      );
      userData.addCommands(
        `ctech_run setup-logs.sh ${logsBucketName} ${S3_PREFIX} ${SERVICE} /var/log/app /var/log/nginx`,
      );

      // ctech-ec2-agent logs-tail replaces the CloudWatch Agent (musl has no
      // working aws-cli/CW-agent build). One logGroup per config file, so two
      // separate services + configs, same as ctech-account/ctech-wallet.
      userData.addCommands(
        `cat > /tmp/ctech-logs-app.json << 'LOGSAPP'`,
        JSON.stringify({
          logGroup: logGroupApp,
          files: [
            {path: '/var/log/app/app.log', streamPrefix: 'app'},
            {path: '/var/log/app/app2.log', streamPrefix: 'app2'},
          ],
        }),
        `LOGSAPP`,
        `ctech_run setup-ctech-ec2-agent.sh /tmp/ctech-logs-app.json app`,
        `cat > /tmp/ctech-logs-nginx.json << 'LOGSNGINX'`,
        JSON.stringify({
          logGroup: logGroupNginx,
          files: [
            {path: '/var/log/nginx/access.log', streamPrefix: 'access'},
            {path: '/var/log/nginx/error.log', streamPrefix: 'error'},
          ],
        }),
        `LOGSNGINX`,
        `ctech_run setup-ctech-ec2-agent.sh /tmp/ctech-logs-nginx.json nginx`,
      );
      userData.addCommands(`ctech_run bootstrap-deploy.sh ${deploymentsBucketName} ${API_CURRENT_ARTIFACT_KEY}`);
    } else {
      scripts!.run(userData, 'setup-realip.sh', vpc.vpcCidrBlock);
      scripts!.run(userData, 'setup-nginx.sh', `${NGINX_PORT}`, `${APP_PORT}`, HEALTH_CHECK_PATH, '100', '1m', `${APP_PORT_ALT}`);
      scripts!.run(userData, 'setup-app-service.sh', 'CTech Poker API', 'app',
        'network.target nginx.service', `${APP_PORT_ALT}`);
      scripts!.run(userData, 'setup-deploy.sh', deploymentsBucketName, 'app',
        `http://127.0.0.1:${NGINX_PORT}${HEALTH_CHECK_PATH}`);
      scripts!.run(userData, 'setup-logs.sh', logsBucketName, S3_PREFIX, SERVICE, '/var/log/app', '/var/log/nginx');

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
                  {file_path: '/var/log/app/app2.log', log_group_name: logGroupApp, log_stream_name: '{instance_id}/app2'},
                  {file_path: '/var/log/nginx/access.log', log_group_name: logGroupNginx, log_stream_name: '{instance_id}/access'},
                  {file_path: '/var/log/nginx/error.log', log_group_name: logGroupNginx, log_stream_name: '{instance_id}/error'},
                ],
              },
            },
          },
        }),
        `CWA`,
      );
      scripts!.run(userData, 'setup-cloudwatch-agent.sh', '/tmp/cwagent.json');
      scripts!.run(userData, 'bootstrap-deploy.sh', deploymentsBucketName, API_CURRENT_ARTIFACT_KEY);
    }

    // ctech-lbalancer still owns the bootstrap route and private CNAME. Target
    // port is NGINX_PORT (unchanged value, 8080) now that nginx fronts the app
    // — ctech-lbalancer's route needs no update.
    const machineImage = isAlpine
      ? ec2.MachineImage.fromSsmParameter(
          CtechSSM.amiAlpine(environment).arm64,
          {os: ec2.OperatingSystemType.LINUX},
        )
      : undefined; // HaproxyEc2Service defaults to latest AL2023 arm64 minimal.

    const service = new HaproxyEc2Service(this, 'ApiService', {
      vpc,
      edgeSecurityGroup: edgeSg,
      appPort: NGINX_PORT,
      userData,
      machineImage,
      instanceProfileName,
      securityGroupName: `${environment}-${SERVICE}-api-sg`,
      securityGroupDescription: 'ctech-poker API instances',
      appLogGroupName: logGroupApp,
      nginxLogGroupName: logGroupNginx,
      logRetention,
      logRemovalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      asgName: this.asgName,
      minCapacity: minimumApiCapacity(environment),
      // +1 over min: gives CapacityRebalance headroom to launch the
      // replacement before terminating the spot-interrupted instance instead
      // of waiting for it to go down first.
      maxCapacity: minimumApiCapacity(environment) + 1,
      // The ASG runs only inside a narrow daytime window: up at 11:55 and down
      // at 13:15 America/Sao_Paulo. Outside it the service is off — inbound
      // webhooks fail and nothing is reachable. Deliberate for a development
      // environment on a single t4g.nano.
      // schedule: {enableCron: '55 11 * * *', disableCron: '15 13 * * *'},
      spot: {}
    });
    const asg = service.autoScalingGroup;
    asg.node.addDependency(profile);

    // ASG termination pauses before EC2 shutdown, asks systemd to stop both
    // app processes (each runs Fx OnStop -> DrainAndRelease, releasing every
    // table lease it holds), then explicitly completes the lifecycle action.
    // The finally block fails open so a broken SSM agent can never strand an
    // instance in Terminating:Wait.
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
            Parameters={"commands": [${isAlpine ? '"rc-service app stop; rc-service app2 stop"' : '"systemctl stop app app2"'}]},
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

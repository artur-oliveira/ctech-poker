import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import type {Environment} from '@aoctech/cdk';
import {minimumApiCapacity, PokerApiStack} from '../lib/api-stack';

test('keeps one minimum API instance in every environment', () => {
  expect(minimumApiCapacity('prod')).toBe(1);
  expect(minimumApiCapacity('stage')).toBe(1);
  expect(minimumApiCapacity('dev')).toBe(1);
});

// The brief's template snippet instantiates PokerApiStack with only `env` —
// adapted here to supply the props the real construct actually requires
// (environment/vpcId/instanceProfileName/bucket names), confirmed
// against ctech-wallet/cdk/lib/api-stack.ts's ApiStackProps shape. Dummy
// values throughout; ec2.Vpc.fromLookup falls back to CDK's built-in dummy
// VPC data when no cdk.context.json cache entry exists, so this does not
// attempt a real AWS lookup.
function synthStack(overrides: Partial<{environment: Environment; cloudwatchAlarmsEnabled: boolean}> = {}) {
  const app = new App();
  return new PokerApiStack(app, 'TestPokerApiStack', {
    env: {account: '123456789012', region: 'us-east-1'},
    environment: 'dev',
    ...overrides,
    vpcId: 'vpc-0123456789abcdef0',
    instanceProfileName: 'dev-ctech-poker-api-instance-profile',
    deploymentsBucketName: 'dev-ctech-deployments',
    logsBucketName: 'dev-ctech-application-logs',
    avatarsBucketName: 'dev-ctech-poker-avatars',
    avatarBaseUrlParam: '/ctech/dev/poker/avatar-base-url',
    tableStateArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_table_state',
    tableStateHistoryArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_table_state_history',
    actionLogArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_action_log',
    actionGuardsArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_action_guards',
    roomsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_rooms',
    playerProfilesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_profiles',
    playerNotesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_notes',
    handSharesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_hand_shares',
    pokerStatsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_poker_stats',
    highlightsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_table_highlights',
    walletUrlParam: '/ctech/dev/poker/wallet-url',
    pokerClientIdParam: '/ctech/dev/poker/poker-client-id',
    pokerClientSecretParam: '/ctech/dev/poker/poker-client-secret',
    turnstileSecretParam: '/ctech/dev/poker/turnstile-secret',
    realMoneyEnabledParam: '/ctech/dev/poker/real-money-enabled',
    legalSignoffRefParam: '/ctech/dev/poker/legal-signoff-ref',
    achievementProgressTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_achievement_progress',
    leaderboardStatsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_leaderboard_stats',
    dailyRewardTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_daily_reward',
    playerSessionsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_sessions',
    playerHandsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_hands',
    handRevealsTableArn:  'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_hand_reveals',
    playerMatchupsTableArn:  'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_matchups',
    walletWebhookHmacSecretParam: '/ctech/dev/poker/wallet-webhook-hmac-secret',
    sandboxPurchasesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_sandbox_purchases',
    pendingCashoutsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_pending_cashouts',
    reactionEntitlementsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_reaction_entitlements',
    reactionPurchasesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_reaction_purchases',
    cosmeticEntitlementsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_cosmetic_entitlements',
    cosmeticPurchasesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_cosmetic_purchases',
    tableEntitlementsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_table_entitlements',
    socialEdgesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_social_edges',
    recentPlayersTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_recent_players',
    socialEventsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_social_events',
    playerReportsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_reports',
    socialGraphEnabledParam: '/ctech/dev/poker/social-graph-enabled',
  });
}

/** EC2's hard cap on user data, which a deploy discovers and not a review. */
const USER_DATA_LIMIT_BYTES = 16384;

/** The rendered user data, with unresolved tokens standing in for their values. */
function userDataText(template: Template): string {
  const launchTemplate = Object.values(template.findResources('AWS::EC2::LaunchTemplate'))[0] as any;
  const encoded = launchTemplate.Properties.LaunchTemplateData.UserData['Fn::Base64'];
  if (typeof encoded === 'string') return encoded;
  return (encoded['Fn::Join'][1] as unknown[])
    .map((part) => (typeof part === 'string' ? part : '<<token>>'))
    .join('');
}

test('synthesizes without error and declares exactly one ASG', () => {
  const template = Template.fromStack(synthStack());
  template.resourceCountIs('AWS::AutoScaling::AutoScalingGroup', 1);
  template.hasResourceProperties('AWS::IAM::Role', {RoleName: 'dev-ctech-poker-api-role'});
  template.resourceCountIs('AWS::IAM::InstanceProfile', 1);
  // No custom CloudWatch metrics, alarms or dashboard (2026-08-19): the CW agent
  // config is logs-only and the operations dashboard was SEARCH-ing a namespace
  // the API never published to.
  template.resourceCountIs('AWS::CloudWatch::Alarm', 0);
  template.resourceCountIs('AWS::CloudWatch::Dashboard', 0);
  const rendered = JSON.stringify(template.toJSON());
  expect(rendered).toContain('dev-ctech-poker-avatars/up/*');
  expect(rendered).toContain('dev-ctech-poker-avatars/av/*');
  // The instance role must be able to READ av/: the API serves
  // /v1.0/avatars/* itself now that no CloudFront OAC reads the bucket.
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {
      Statement: Match.arrayWith([Match.objectLike({
        Action: Match.arrayWith(['s3:GetObject']),
        Resource: Match.objectLike({'Fn::Join': Match.arrayWith([
          Match.arrayWith([':s3:::dev-ctech-poker-avatars/av/*']),
        ])}),
      })]),
    },
  });
  expect(rendered).not.toContain('dev-ctech-poker-frontend');
  expect(rendered).toContain('dev_poker_reaction_entitlements');
  expect(rendered).toContain('dev_poker_reaction_purchases');
  expect(rendered).toContain('dev_poker_social_edges');
  expect(rendered).toContain('dev_poker_recent_players');
  expect(rendered).toContain('dev_poker_social_events');
  expect(rendered).toContain('dev_poker_player_reports');
  expect(rendered).toContain('dynamodb:BatchGetItem');
  expect(rendered).toContain('/ctech/dev/poker/social-graph-enabled');
  expect(rendered).toContain('SOCIAL_GRAPH_ENABLED');
  expect(rendered).not.toContain('AWS::WAFv2');
  template.hasResourceProperties('AWS::AutoScaling::LifecycleHook', {
    LifecycleHookName: 'dev-ctech-poker-termination-drain',
    LifecycleTransition: 'autoscaling:EC2_INSTANCE_TERMINATING',
    DefaultResult: 'CONTINUE',
    HeartbeatTimeout: 120,
  });
  template.hasResourceProperties('AWS::AutoScaling::AutoScalingGroup', {
    HealthCheckType: 'EC2',
  });
  template.resourceCountIs('AWS::ElasticLoadBalancingV2::TargetGroup', 0);
  template.resourceCountIs('AWS::ElasticLoadBalancingV2::ListenerRule', 0);
  expect(rendered).toContain('autoscaling:CompleteLifecycleAction');
  expect(rendered).toContain('ssm:SendCommand');
});

test('hand-pipeline duration budget alarm exists only when prod + cloudwatchAlarmsEnabled (#290)', () => {
  const off = Template.fromStack(synthStack());
  off.resourceCountIs('AWS::CloudWatch::Alarm', 0);

  const on = Template.fromStack(synthStack({environment: 'prod', cloudwatchAlarmsEnabled: true}));
  on.hasResourceProperties('AWS::CloudWatch::Alarm', {
    AlarmName: 'prod-hand-pipeline-duration-budget',
    Namespace: 'CTechPoker',
    MetricName: 'HandPipelineDuration',
    Dimensions: Match.arrayWith([Match.objectLike({Name: 'Environment', Value: 'prod'})]),
    ExtendedStatistic: 'p95',
    Threshold: 24_000,
  });
});

test('ASG spreads across multiple AZs and diversifies spot instance types (#35)', () => {
  const template = Template.fromStack(synthStack());
  // The dummy VPC ec2.Vpc.fromLookup falls back to (no cdk.context.json
  // cache entry for this vpc-id/account/region combination) carries subnets
  // in 2 AZs — enough to prove the ASG is not artificially pinned to one.
  // The real shared VPC (see cdk.context.json) has 3 (us-east-1b/c/d); this
  // stack applies no `availabilityZones` filter, so it inherits whatever
  // the looked-up VPC provides.
  const asgResources = template.findResources('AWS::AutoScaling::AutoScalingGroup');
  const [asg] = Object.values(asgResources) as any[];
  expect(asg.Properties.VPCZoneIdentifier.length).toBeGreaterThanOrEqual(2);

  // A correlated spot-reclaim event for a single instance type/pool must not
  // be able to zero the whole ASG: at least 3 equivalent Graviton burstable
  // types are offered, so price-capacity-optimized has somewhere else to
  // bid. Every override carries no WeightedCapacity, so it defaults to 1 —
  // launching any of these three types still costs exactly one unit of ASG
  // capacity (minCapacity/maxCapacity are unaffected).
  const overrides = asg.Properties.MixedInstancesPolicy.LaunchTemplate.Overrides;
  expect(overrides.length).toBeGreaterThanOrEqual(2);
  expect(overrides.every((o: any) => o.WeightedCapacity === undefined)).toBe(true);
  expect(overrides.map((o: any) => o.InstanceType)).toEqual(
    expect.arrayContaining(['t4g.nano', 't4g.micro',]),
  );
});

test('user data only fetches and runs the shared ctech-cdk scripts', () => {
  const text = userDataText(Template.fromStack(synthStack()));
  expect(text).toContain('ctech_run');
  expect(text).toContain('setup-base.sh');
  expect(text).toContain('setup-app-service.sh');
  expect(text).toContain('setup-deploy.sh');
  // nginx now fronts the app (zero-downtime rolling deploy needs it to
  // round-robin between app and app2).
  expect(text).toContain('setup-nginx.sh');
  expect(text).toContain('setup-realip.sh');
  // app-port-alt/alt-port turn on the rolling deploy.
  expect(text).toContain("setup-nginx.sh 8080 8000 /v1.0/health-check 100 1m 8001");
  expect(text).toContain("ctech_run setup-app-service.sh 'CTech Poker API' app 8001");
  // Downloaded to a file and then executed: a pipe truncated mid-transfer runs a
  // partial script and reports success.
  expect(text).not.toMatch(/aws s3 cp [^\n]*\| *bash/);
  // Only app-static.env, service-env.sh, the WS nginx location fragment and
  // the CloudWatch agent config are still written inline; everything else
  // moved to the shared scripts.
  expect((text.match(/cat > /g) ?? []).length).toBeLessThanOrEqual(5);
});

test('user data stays under the EC2 limit', () => {
  expect(Buffer.byteLength(userDataText(Template.fromStack(synthStack())), 'utf8'))
    .toBeLessThan(USER_DATA_LIMIT_BYTES);
});

test('no secret value is written into the launch template', () => {
  // Secrets are read from SSM by name at service start, using the instance role.
  const text = userDataText(Template.fromStack(synthStack()));
  for (const secret of ['POKER_CLIENT_SECRET', 'TURNSTILE_SECRET', 'WALLET_WEBHOOK_HMAC_SECRET']) {
    expect(text).toContain(`${secret}=/ctech/dev/poker/`);
  }
});

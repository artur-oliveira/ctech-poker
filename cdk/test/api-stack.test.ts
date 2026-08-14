import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
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
test('synthesizes without error and declares exactly one ASG', () => {
  const app = new App();
  const stack = new PokerApiStack(app, 'TestPokerApiStack', {
    env: {account: '123456789012', region: 'us-east-1'},
    environment: 'dev',
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
    walletWebhookHmacSecretParam: '/ctech/dev/poker/wallet-webhook-hmac-secret',
    sandboxPurchasesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_sandbox_purchases',
    pendingCashoutsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_pending_cashouts',
    reactionEntitlementsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_reaction_entitlements',
    reactionPurchasesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_reaction_purchases',
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::AutoScaling::AutoScalingGroup', 1);
  template.hasResourceProperties('AWS::IAM::Role', {RoleName: 'dev-ctech-poker-api-role'});
  template.resourceCountIs('AWS::IAM::InstanceProfile', 1);
  template.resourceCountIs('AWS::CloudWatch::Alarm', 2);
  template.hasResourceProperties('AWS::CloudWatch::Dashboard', {
    DashboardName: 'dev-ctech-poker-operations',
  });
  const rendered = JSON.stringify(template.toJSON());
  expect(rendered).toContain('dev-ctech-poker-avatars/up/*');
  expect(rendered).toContain('dev-ctech-poker-avatars/av/*');
  expect(rendered).not.toContain('dev-ctech-poker-frontend');
  expect(rendered).toContain('dev_poker_reaction_entitlements');
  expect(rendered).toContain('dev_poker_reaction_purchases');
  for (const signal of [
    'ActionLatencyMs',
    'ActionsSucceeded',
    'SnapshotLatencyMs',
    'DynamoDBVersionConflicts',
    'PendingCashouts',
    'OldestPendingCashoutAgeSeconds',
    'ConnectionsOpened',
    'HTTPResponses',
  ]) {
    expect(rendered).toContain(signal);
  }
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

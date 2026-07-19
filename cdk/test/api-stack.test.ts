import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {PokerApiStack} from '../lib/api-stack';

// The brief's template snippet instantiates PokerApiStack with only `env` —
// adapted here to supply the props the real construct actually requires
// (environment/vpcId/domainName/instanceProfileName/bucket names), confirmed
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
    domainName: 'poker-api-dev.aoctech.app',
    appDomainName: 'poker-dev.aoctech.app',
    authDomainName: 'accounts-dev.aoctech.app',
    instanceProfileName: 'dev-ctech-poker-api-instance-profile',
    deploymentsBucketName: 'dev-ctech-deployments',
    logsBucketName: 'dev-ctech-application-logs',
    tableStateArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_table_state',
    actionLogArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_action_log',
    actionGuardsArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_action_guards',
    roomsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_rooms',
    playerProfilesTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_player_profiles',
    walletUrlParam: '/ctech/dev/poker/wallet-url',
    pokerClientIdParam: '/ctech/dev/poker/poker-client-id',
    pokerClientSecretParam: '/ctech/dev/poker/poker-client-secret',
    achievementProgressTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_achievement_progress',
    leaderboardStatsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_leaderboard_stats',
    rouletteSpinsTableArn: 'arn:aws:dynamodb:us-east-1:123456789012:table/dev_poker_roulette_spins',
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::AutoScaling::AutoScalingGroup', 1);
  template.resourceCountIs('AWS::IAM::Role', 1);
  template.resourceCountIs('AWS::IAM::InstanceProfile', 1);
});

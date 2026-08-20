#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib';
import {Environment} from '@aoctech/cdk';

import {PokerApiStack} from '../lib/api-stack';
import {DynamoDBStack} from '../lib/dynamodb-stack';
import {ArchiverStack} from '../lib/archiver-stack';
import {FrontendStack} from '../lib/frontend-stack';
import {ReconcileStack} from '../lib/reconcile-stack';
import {TableCleanupStack} from '../lib/tablecleanup-stack';
import {
  ACCOUNTS_API_DOMAIN_PREFIX, ACCOUNTS_DOMAIN_PREFIX,
  API_DOMAIN_PREFIX,
  APP_DOMAIN_PREFIX,
  avatarsBucketName,
  avatarsS3Origins,
  AWS_ACCOUNT,
  AWS_REGION,
  CERT_ARN,
  DYNAMO_TABLE,
  domainForEnv,
  GITHUB_REPO_DEFAULT,
  instanceProfileName,
  SSM_POKER,
} from '../lib/constants';
import {OidcStack} from "../lib/oidc-stack";

const app = new cdk.App();

const CLOUDFLARE_CHALLENGE_SRC = 'challenges.cloudflare.com'
const ENVIRONMENT = (process.env.ENVIRONMENT || 'dev') as Environment;
const GITHUB_REPO = (process.env.GITHUB_REPO || GITHUB_REPO_DEFAULT);
// VPC is managed by ctech-cdk (shared across every CTech service in this
// account — same default used by ctech-wallet/ctech-dfe/ctech-account). The ID
// must be a concrete string (not a token) because ec2.Vpc.fromLookup resolves
// subnet/AZ metadata at synthesis time. CI reads /ctech/{env}/network/vpc-id
// from SSM and exports it as CTECH_VPC_ID before running cdk deploy.
const CTECH_VPC_ID = process.env.CTECH_VPC_ID || 'vpc-0adfd86727d17445b';
// Shared S3 buckets owned by ctech-cdk. CI reads these from SSM
// (/ctech/{env}/s3/deployments-bucket and /ctech/{env}/s3/logs-bucket)
// and sets them as env vars before running cdk deploy.
const CTECH_DEPLOYMENTS_BUCKET = process.env.CTECH_DEPLOYMENTS_BUCKET || `${ENVIRONMENT}-ctech-deployments`;
const CTECH_LOGS_BUCKET = process.env.CTECH_LOGS_BUCKET || `${ENVIRONMENT}-ctech-application-logs`;
// Session Manager on the API instances. Default on, and poker has the strongest
// reason of any service to keep it: the termination-drain lifecycle hook stops
// the app through SSM RunCommand, so without the agent instances terminate
// without draining tables. It is also the only shell onto the box. Set
// ENABLE_SSM_AGENT=false to reclaim the agent's ~70 MiB of RSS at that price.
const ENABLE_SSM_AGENT = (process.env.ENABLE_SSM_AGENT || 'true') === 'true';

const env = {account: AWS_ACCOUNT, region: AWS_REGION};
const pokerParameters = SSM_POKER(ENVIRONMENT);

// Cost allocation tags — applied to every resource in every stack.
// Requires manual activation as a cost allocation tag in the Billing console
// (Billing > Cost Allocation Tags) before it appears as a Cost Explorer group-by key.
cdk.Tags.of(app).add('Project', 'ctech-poker');
cdk.Tags.of(app).add('Environment', ENVIRONMENT);

const id = (name: string) =>
  `CtechPoker-${ENVIRONMENT.charAt(0).toUpperCase() + ENVIRONMENT.slice(1)}-${name}`;

new OidcStack(app, 'CtechPoker-Global-OIDC', {
  env,
  githubRepo: GITHUB_REPO,
  deploymentsBucket: CTECH_DEPLOYMENTS_BUCKET,
  description: 'CTech Poker GitHub Actions deployment roles (global)',
});
// =====================
// DynamoDB (table state, action log + archival)
// =====================
const dynamoStack = new DynamoDBStack(app, id('DynamoDB'), {
  env,
  environment: ENVIRONMENT,
  description: `CTech Poker DynamoDB tables - ${ENVIRONMENT}`,
});

new ArchiverStack(app, id('Archiver'), {
  env,
  environment: ENVIRONMENT,
  actionLogTable: dynamoStack.tables.get('poker_action_log')!,
  description: `CTech Poker action-log archiver (DynamoDB Streams -> S3) - ${ENVIRONMENT}`,
});

// =====================
// API (EC2 + ASG, shared ALB from ctech-cdk)
// =====================
new PokerApiStack(app, id('API'), {
  env,
  environment: ENVIRONMENT,
  vpcId: CTECH_VPC_ID,
  instanceProfileName: instanceProfileName(ENVIRONMENT),
  deploymentsBucketName: CTECH_DEPLOYMENTS_BUCKET,
  logsBucketName: CTECH_LOGS_BUCKET,
  avatarsBucketName: avatarsBucketName(ENVIRONMENT),
  avatarBaseUrlParam: pokerParameters.avatarBaseUrl,
  tableStateArn: dynamoStack.tables.get('poker_table_state')!.tableArn,
  tableStateHistoryArn: dynamoStack.tables.get('poker_table_state_history')!.tableArn,
  actionLogArn: dynamoStack.tables.get('poker_action_log')!.tableArn,
  actionGuardsArn: dynamoStack.tables.get('poker_action_guards')!.tableArn,
  roomsTableArn: dynamoStack.tables.get('poker_rooms')!.tableArn,
  playerProfilesTableArn: dynamoStack.tables.get('poker_player_profiles')!.tableArn,
  playerNotesTableArn: dynamoStack.tables.get('poker_player_notes')!.tableArn,
  handSharesTableArn: dynamoStack.tables.get('poker_hand_shares')!.tableArn,
  pokerStatsTableArn: dynamoStack.tables.get('poker_player_poker_stats')!.tableArn,
  achievementProgressTableArn: dynamoStack.tables.get('poker_achievement_progress')!.tableArn,
  leaderboardStatsTableArn: dynamoStack.tables.get('poker_leaderboard_stats')!.tableArn,
  dailyRewardTableArn: dynamoStack.tables.get('poker_daily_reward')!.tableArn,
  playerSessionsTableArn: dynamoStack.tables.get('poker_player_sessions')!.tableArn,
  playerHandsTableArn: dynamoStack.tables.get('poker_player_hands')!.tableArn,
  walletWebhookHmacSecretParam: pokerParameters.walletWebhookHmacSecret,
  sandboxPurchasesTableArn: dynamoStack.tables.get('poker_sandbox_purchases')!.tableArn,
  pendingCashoutsTableArn: dynamoStack.tables.get('poker_pending_cashouts')!.tableArn,
  reactionEntitlementsTableArn: dynamoStack.tables.get('poker_reaction_entitlements')!.tableArn,
  reactionPurchasesTableArn: dynamoStack.tables.get('poker_reaction_purchases')!.tableArn,
  walletUrlParam: pokerParameters.walletInternalUrl,
  pokerClientIdParam: pokerParameters.clientId,
  pokerClientSecretParam: pokerParameters.clientSecret,
  turnstileSecretParam: pokerParameters.turnstileSecret,
  realMoneyEnabledParam: pokerParameters.realMoneyEnabled,
  legalSignoffRefParam: pokerParameters.legalSignoffRef,
  socialGraphEnabledParam: pokerParameters.socialGraphEnabled,
  enableSsmAgent: ENABLE_SSM_AGENT,
  socialEdgesTableArn: dynamoStack.tables.get(DYNAMO_TABLE.socialEdges)!.tableArn,
  recentPlayersTableArn: dynamoStack.tables.get(DYNAMO_TABLE.recentPlayers)!.tableArn,
  socialEventsTableArn: dynamoStack.tables.get(DYNAMO_TABLE.socialEvents)!.tableArn,
  playerReportsTableArn: dynamoStack.tables.get(DYNAMO_TABLE.playerReports)!.tableArn,
  description: `CTech Poker API (EC2 + ASG + ALB) - ${ENVIRONMENT}`,
});

new FrontendStack(app, id('Frontend'), {
  env,
  environment: ENVIRONMENT,
  certificateArn: CERT_ARN,
  domainName: domainForEnv(ENVIRONMENT, APP_DOMAIN_PREFIX),
  apiDomainName: domainForEnv(ENVIRONMENT, API_DOMAIN_PREFIX),
  authDomainName: domainForEnv(ENVIRONMENT, ACCOUNTS_API_DOMAIN_PREFIX),
  extraConnectSrc: [CLOUDFLARE_CHALLENGE_SRC, ...avatarsS3Origins(ENVIRONMENT)],
  description: `CTech Poker Frontend (S3 + CloudFront) - ${ENVIRONMENT}`,
});

new ReconcileStack(app, id('Reconcile'), {
  env,
  environment: ENVIRONMENT,
  pendingCashoutsTableArn: dynamoStack.tables.get('poker_pending_cashouts')!.tableArn,
  authDomainName: domainForEnv(ENVIRONMENT, ACCOUNTS_DOMAIN_PREFIX),
  walletUrlParam: pokerParameters.walletUrl,
  pokerClientIdParam: pokerParameters.clientId,
  pokerClientSecretParam: pokerParameters.clientSecret,
  description: `CTech Poker Cashout Reconcile Lambda - ${ENVIRONMENT}`,
});

new TableCleanupStack(app, id('TableCleanup'), {
  env,
  environment: ENVIRONMENT,
  tableStateArn: dynamoStack.tables.get('poker_table_state')!.tableArn,
  roomsTableArn: dynamoStack.tables.get('poker_rooms')!.tableArn,
  walletUrlParam: pokerParameters.walletUrl,
  pokerClientIdParam: pokerParameters.clientId,
  pokerClientSecretParam: pokerParameters.clientSecret,
  description: `CTech Poker stale-table cleanup Lambda - ${ENVIRONMENT}`,
});

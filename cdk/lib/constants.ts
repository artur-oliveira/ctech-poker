import {Environment} from '@aoctech/cdk';

/**
 * Every magic string used by more than one file lives here (see root CLAUDE.md
 * conventions mirrored from ctech-wallet/ctech-dfe: "Constants — no magic
 * variables"). Names that AWS resources are actually created with must never
 * be inlined at a call site.
 */

// ── Account / region ────────────────────────────────────────────────────────
// Single AWS account shared by every CTech service (see ctech-wallet,
// ctech-dfe, ctech-account cdk/lib/constants.ts).
export const AWS_ACCOUNT = '868899309401';
export const AWS_REGION = 'us-east-1';

// Wildcard *.aoctech.app cert — owned by ctech-cdk, same one every service uses.
export const CERT_ARN =
  'arn:aws:acm:us-east-1:868899309401:certificate/29678869-bfc3-4688-b81b-55aa5b1d7443';

// Account-wide CloudWatch alarms topic — owned by ctech-cdk, imported via
// sns.Topic.fromTopicArn wherever an alarm needs an action. Never create a
// new SNS topic for poker's own alarms (see issue #34).
export const ALERTS_TOPIC_ARN = 'arn:aws:sns:us-east-1:868899309401:ctech-prod-alerts';

export const GITHUB_REPO_DEFAULT = 'artur-oliveira/ctech-poker';

// ── Naming ──────────────────────────────────────────────────────────────────
export const SERVICE = 'ctech-poker';
export const BASE_DOMAIN = 'aoctech.app';

/** ALB (api) host prefix. */
export const API_DOMAIN_PREFIX = 'poker-api';
export const APP_DOMAIN_PREFIX = 'poker';
export const ACCOUNTS_API_DOMAIN_PREFIX = 'accounts-api';
export const ACCOUNTS_DOMAIN_PREFIX = 'accounts';
export const API_PATH_PATTERNS = ['/v1.0/*', '/.well-known/*'];

/**
 * Shared HTTPS listener rule priorities on the ctech-cdk ALB (confirmed by
 * reading every sibling service CDK that attaches to the shared listener):
 * 15 = py-dfe api (ctech-dfe/cdk/lib/api-v2-stack.ts), 25 = ctech-account api
 * (ctech-account/cdk/bin/ctech-account.ts), 35 = ctech-wallet api
 * (ctech-wallet/cdk/lib/constants.ts). ctech-billing and ctech-vanity have no
 * CDK stack yet and do not attach to this listener. Must stay unique across
 * every service that attaches to the shared listener.
 */
export const ALB_LISTENER_PRIORITY = 45;

/**
 * Port the Go binary listens on internally, behind nginx. Previously this was
 * also the HAProxy target port, before poker had nginx in front; it now
 * matches the ctech-wallet/ctech-dfe/ctech-account layout.
 */
export const APP_PORT = 8000;

/**
 * Loopback-only port for the second app process (zero-downtime rolling
 * deploy). nginx round-robins between APP_PORT and APP_PORT_ALT; never
 * touches the security group. See ctech-cdk's assets/ec2/setup-deploy.sh.
 */
export const APP_PORT_ALT = APP_PORT + 1;

/**
 * Port nginx listens on — the HAProxy target/health-check port. Unchanged
 * from when this was APP_PORT directly, so ctech-lbalancer's route needs no
 * update.
 */
export const NGINX_PORT = 8080;

/**
 * Detailed health check path served by the Go API (RFC draft-inadarei-api-health-check;
 * see api/internal/api/v1/health.go). The ALB target group accepts 200 and 207
 * (degraded but still serving) as healthy.
 */
export const HEALTH_CHECK_PATH = '/v1.0/health-check';

/**
 * Spot instance-type pool for the API ASG's MixedInstancesPolicy (#35: a spot
 * capacity shortage in a single instance type/pool used to be able to zero
 * the whole ASG, in every AZ, non-self-healing). All three are the same
 * burstable Graviton (arm64) family as the previous single type (t4g.nano)
 * — nano/micro/small only differ in memory (0.5/1/2 GiB) at 2 vCPU each, so
 * price-capacity-optimized keeps picking the cheapest available pool and
 * cost stays roughly flat; this only lets the ASG fall back to a slightly
 * larger pool instead of going to zero when one type's spot capacity dries
 * up. WeightedCapacity is left at its CDK/CFN default (1 per instance) for
 * every override, so a launch of any of these three still counts as exactly
 * one unit of ASG capacity — unrelated to the leasing model, which already
 * tolerates 2 concurrently-running instances (minCapacity=1/maxCapacity=2).
 */
export const API_ASG_SPOT_INSTANCE_TYPES = ['t4g.nano', 't4g.micro',] as const;

/** S3 key prefix inside the shared deployments/logs buckets. */
export const S3_PREFIX = SERVICE;
/** Key of the artifact new ASG instances bootstrap from. */
export const API_CURRENT_ARTIFACT_KEY = `${S3_PREFIX}/current.zip`;

export const API_MEMORY_PRESSURE_LOG_MESSAGE = 'ALARM: process memory pressure';
export const API_MEMORY_METRIC_NAMESPACE = 'CTech/Poker';
export const API_MEMORY_PRESSURE_METRIC_NAME = 'ProcessMemoryPressure';

// ── Per-environment names ───────────────────────────────────────────────────
export const asgName = (env: Environment) => `${env}-${SERVICE}`;
export const instanceProfileName = (env: Environment) => `${env}-${SERVICE}-api-instance-profile`;
export const frontendBucketName = (env: Environment) => `${env}-${SERVICE}-frontend`;
export const avatarsBucketName = (env: Environment) => `${env}-${SERVICE}-avatars`;
export const avatarsS3Origins = (env: Environment) => [
  `${avatarsBucketName(env)}.s3.${AWS_REGION}.amazonaws.com`,
  `${avatarsBucketName(env)}.s3.dualstack.${AWS_REGION}.amazonaws.com`,
];
export const routeStoreName = (env: Environment) => `${env}-${SERVICE}-routes`;
export const instanceRoleName = (env: Environment) => `${env}-${SERVICE}-api-role`;
export const reconcileJobName = (env: Environment) => `${env}-${SERVICE}-reconcile`;
export const reconcileDlqName = (env: Environment) => `${reconcileJobName(env)}-dlq`;
export const tableCleanupJobName = (env: Environment) => `${env}-${SERVICE}-tablecleanup`;
export const tableCleanupDlqName = (env: Environment) => `${tableCleanupJobName(env)}-dlq`;

// ── Poker social storage ───────────────────────────────────────────────────
// Keep table and index identifiers centralized so the infrastructure and its
// assertions cannot silently drift as the social modules are implemented.
export const DYNAMO_TABLE = {
  socialEdges: 'poker_social_edges',
  recentPlayers: 'poker_recent_players',
  socialEvents: 'poker_social_events',
  playerReports: 'poker_player_reports',
} as const;

export const DYNAMO_INDEX = {
  friendCode: 'gsi_friend_code',
  socialInbox: 'gsi_inbox',
  socialUnread: 'gsi_unread',
  reportStatus: 'gsi_status',
  handShareOwner: 'gsi_owner',
} as const;

// ── GitHub Actions OIDC trust scoping ──────────────────────────────────────
/**
 * Branches that `.github/workflows/deploy.yml` deploys from (its `push`
 * trigger). The OIDC trust policy is pinned to exactly these refs — no bare
 * `:*` wildcard — so a workflow running on any other ref (a feature branch, a
 * tag, a fork) cannot assume the deployment roles.
 */
export const GHA_DEPLOY_BRANCHES = ['main', 'staging', 'dev'] as const;

// ── GitHub Actions role names (global, not per-env) ─────────────────────────
export const GHA_API_ROLE = `${SERVICE}-gha-api`;
export const GHA_FRONTEND_ROLE = `${SERVICE}-gha-frontend`;
export const GHA_INFRA_ROLE = `${SERVICE}-gha-infra`;
export const GHA_SCOPES_ROLE = `${SERVICE}-gha-scopes`;

// ── SSM parameter paths ─────────────────────────────────────────────────────
/**
 * Shared infra owned by ctech-cdk (see ctech-cdk/lib/constants.ts `SSM` and
 * ctech-cdk/CLAUDE.md's canonical path convention). Not published via
 * @aoctech/cdk's index, so every consumer redeclares the same literal paths —
 * confirmed against ctech-cdk/lib/alb-stack.ts and ctech-cdk/lib/valkey-stack.ts.
 */
export const SSM_SHARED = (env: Environment) => ({
  vpcId: `/ctech/${env}/network/vpc-id`,
  albSgId: `/ctech/${env}/network/alb-sg-id`,
  httpsListenerArn: `/ctech/${env}/alb/https-listener-arn`,
  // Base URL with no DB number. Unlike ctech-wallet (which appends its own
  // /2 for keyspace isolation), ctech-dfe and ctech-account both pass this
  // straight through as VALKEY_URL with no suffix — that's the precedent
  // followed here, and it matches api/internal/config/config.go, which reads
  // VALKEY_URL as a single opaque string. tablelease keys are already
  // namespaced by prefix (`table:{id}`), so no DB-level isolation is needed.
  valkeyUrl: `/ctech/${env}/valkey/url`,
  deploymentsBucket: `/ctech/${env}/s3/deployments-bucket`,
  logsBucket: `/ctech/${env}/s3/logs-bucket`,
});

/**
 * Poker-owned runtime configuration. These parameters are operational
 * prerequisites rather than resources created here: the client credentials
 * only exist after ctech-account seeds poker's M2M client.
 */
export const SSM_POKER = (env: Environment) => ({
  walletUrl: `/ctech/${env}/poker/wallet-url`,
  walletInternalUrl: `/ctech/${env}/poker/wallet-internal-url`,
  appUrl: `/ctech/${env}/poker/app-url`,
  internalBaseUrl: `/ctech/${env}/poker/internal-base-url`,
  clientId: `/ctech/${env}/poker/poker-client-id`,
  clientSecret: `/ctech/${env}/poker/poker-client-secret`,
  turnstileSecret: `/ctech/${env}/poker/turnstile-secret`,
  // Real-money kill switch + legal sign-off reference. Both live in SSM (not
  // baked into userdata) so ops can flip them without a CDK redeploy —
  // config.Load() fails closed if REAL_MONEY_ENABLED=true and this ref is
  // empty (api/internal/config/config.go).
  realMoneyEnabled: `/ctech/${env}/poker/real-money-enabled`,
  socialGraphEnabled: `/ctech/${env}/poker/social-graph-enabled`,
  legalSignoffRef: `/ctech/${env}/poker/legal-signoff-ref`,
  // Read by the API as AVATAR_BASE_URL and prefixed onto every avatar URL it
  // serialises. Since the Cloudflare migration its value is the API's own
  // public read route — https://poker-api[-env].aoctech.app/v1.0/avatars —
  // not a CloudFront path. Provisioned out of band like every other parameter
  // here; see docs/plans for the put-parameter command.
  avatarBaseUrl: `/ctech/${env}/poker/avatar-base-url`,
  // Verifies inbound ctech-wallet webhooks (X-Wallet-Signature). Must match
  // the secret registered for poker's client_id in ctech-wallet's own SSM
  // M2M-clients param — that registration is manual, done outside CDK.
  walletWebhookHmacSecret: `/ctech/${env}/poker/wallet-webhook-hmac-secret`,
});

export const SSM_ACCOUNT = (env: Environment) => ({
  internalBaseUrl: `/ctech-account/${env}/internal-base-url`,
  appUrl: `/ctech-account/${env}/app-url`,
  internalJwksUrl: `/ctech-account/${env}/internal-jwks-url`,
});

// ── Domain helper (identical to ctech-wallet's / ctech-dfe's) ───────────────
export const domainForEnv = (environment: Environment, prefix: string) => {
  switch (environment) {
    case 'prod':
      return `${prefix}.${BASE_DOMAIN}`;
    case 'dev':
    case 'stage':
      return `${prefix}-${environment}.${BASE_DOMAIN}`;
  }
};

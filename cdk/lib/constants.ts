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

export const GITHUB_REPO_DEFAULT = 'artur-oliveira/ctech-poker';

// ── Naming ──────────────────────────────────────────────────────────────────
export const SERVICE = 'ctech-poker';
export const BASE_DOMAIN = 'aoctech.app';

/** ALB (api) host prefix. */
export const API_DOMAIN_PREFIX = 'poker-api';
export const APP_DOMAIN_PREFIX = 'poker';
export const ACCOUNTS_API_DOMAIN_PREFIX = 'accounts-api';
export const ACCOUNTS_DOMAIN_PREFIX = 'accounts';
export const API_PATH_PATTERNS = ['/v1.0/*'];

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
 * Port the Go binary listens on directly (matches api/internal/config/config.go
 * PORT default). Unlike ctech-wallet/ctech-dfe, this skeleton does not front
 * the app with nginx, so appPort IS the ALB target/health-check port — no
 * separate NGINX_PORT.
 */
export const APP_PORT = 8003;

/**
 * Detailed health check path served by the Go API (RFC draft-inadarei-api-health-check;
 * see api/internal/api/v1/health.go). The ALB target group accepts 200 and 207
 * (degraded but still serving) as healthy.
 */
export const HEALTH_CHECK_PATH = '/v1.0/health-check';

/** S3 key prefix inside the shared deployments/logs buckets. */
export const S3_PREFIX = SERVICE;
/** Key of the artifact new ASG instances bootstrap from. */
export const API_CURRENT_ARTIFACT_KEY = `${S3_PREFIX}/current.zip`;

// ── Per-environment names ───────────────────────────────────────────────────
export const asgName = (env: Environment) => `${env}-${SERVICE}-api`;
export const instanceProfileName = (env: Environment) => `${env}-${SERVICE}-api-instance-profile`;
export const frontendBucketName = (env: Environment) => `${env}-${SERVICE}-frontend`;
export const routeStoreName = (env: Environment) => `${env}-${SERVICE}-routes`;
export const instanceRoleName = (env: Environment) => `${env}-${SERVICE}-api-role`;

// ── GitHub Actions role names (global, not per-env) ─────────────────────────
export const GHA_API_ROLE = `${SERVICE}-gha-api`;
export const GHA_FRONTEND_ROLE = `${SERVICE}-gha-frontend`;
export const GHA_INFRA_ROLE = `${SERVICE}-gha-infra`;

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
  clientId: `/ctech/${env}/poker/poker-client-id`,
  clientSecret: `/ctech/${env}/poker/poker-client-secret`,
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

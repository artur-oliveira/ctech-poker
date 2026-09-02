import * as cdk from 'aws-cdk-lib';
import * as iam from 'aws-cdk-lib/aws-iam';
import {Construct} from 'constructs';
import {
  GHA_API_ROLE,
  GHA_DEPLOY_BRANCHES,
  GHA_INFRA_ROLE,
  GHA_SCOPES_ROLE,
  S3_PREFIX,
  SERVICE,
} from './constants';

interface OidcStackProps extends cdk.StackProps {
  githubRepo: string;
  deploymentsBucket: string;
}

/**
 * One-time global stack (not per-environment).
 * Creates the GitHub Actions deployment roles. Auth is pure OIDC — there are no
 * long-lived access keys and no `secrets.*` anywhere in the workflows.
 *
 * ── Trust scoping (issue #41) ─────────────────────────────────────────────
 * The `sub` claim is pinned with `StringEquals` (exact match, never `:*`):
 *   - deploy/api/scopes roles: only the deploy branches
 *     (`repo:<repo>:ref:refs/heads/{main,staging,dev}`), because
 *     `.github/workflows/deploy.yml` only runs those jobs on `push` to those
 *     branches (`if: github.event_name != 'pull_request'`).
 *   - infra role: the deploy branches **plus** `repo:<repo>:pull_request`,
 *     because `infra.yml`'s `diff` job assumes it on `pull_request` to render a
 *     read-only `cdk diff` comment. `cdk diff` needs only describe/read access,
 *     which PowerUserAccess already implies; the PR context cannot deploy
 *     because the workflow gates the deploy job on the event name.
 * The old malformed second `sub` pattern (an "owner@.../repo@..." shape that
 * can never match a real GitHub `sub`) is removed.
 *
 * ── infraRole permissions (issue #41) ────────────────────────────────────
 * `AdministratorAccess` is replaced with `PowerUserAccess` + a narrowly scoped
 * IAM block (CDK must create/update the app roles, instance profiles and
 * Lambda execution roles) + an explicit `Deny` that blocks IAM principal /
 * credential creation (users, access keys, login profiles, MFA, SAML/OIDC
 * providers) and Organizations/Account tampering — i.e. the privilege-
 * escalation surface that made the admin grant high-severity. This is a
 * documented interim: a hand-written CloudFormation-only allowlist was judged
 * too risky to get exactly right for 8 stacks. Follow-up: attach a permissions
 * boundary to the roles CDK creates and tighten to a per-service allowlist.
 */
export class OidcStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: OidcStackProps) {
    super(scope, id, props);

    const {githubRepo, deploymentsBucket} = props;

    // GitHub OIDC provider is owned by ctech-cdk (Ctech-Global stack).
    // Import by well-known ARN — do not create it here.
    const providerArn = `arn:aws:iam::${this.account}:oidc-provider/token.actions.githubusercontent.com`;
    const provider = iam.OpenIdConnectProvider.fromOpenIdConnectProviderArn(
      this, 'GitHubOidc', providerArn,
    );

    const branchSubs = GHA_DEPLOY_BRANCHES.map(
      (b) => `repo:${githubRepo}:ref:refs/heads/${b}`,
    );
    const prSub = `repo:${githubRepo}:pull_request`;

    const principalFor = (subs: string[]) =>
      new iam.WebIdentityPrincipal(provider.openIdConnectProviderArn, {
        StringEquals: {
          'token.actions.githubusercontent.com:aud': 'sts.amazonaws.com',
          'token.actions.githubusercontent.com:sub': subs,
        },
      });

    // Deploy jobs (push to a deploy branch only).
    const deployTrust = principalFor(branchSubs);
    // Infra role additionally trusts pull_request for the read-only `cdk diff`.
    const infraTrust = principalFor([...branchSubs, prSub]);

    const deploymentsPrefixArns = [
      `arn:aws:s3:::${deploymentsBucket}/${S3_PREFIX}`,
      `arn:aws:s3:::${deploymentsBucket}/${S3_PREFIX}/*`,
    ];

    const scopesRole = new iam.Role(this, 'ScopePublisherRole', {
      roleName: GHA_SCOPES_ROLE,
      assumedBy: deployTrust,
    });
    scopesRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        'arn:aws:ssm:*:*:parameter/ctech-account/*/base-url',
        'arn:aws:ssm:*:*:parameter/ctech-account/*/scope-publishers/poker/client-id',
        'arn:aws:ssm:*:*:parameter/ctech-account/*/scope-publishers/poker/client-secret',
      ],
    }));

    // ── API deploy role ─────────────────────────────────────────────────────
    const apiRole = new iam.Role(this, 'ApiDeployRole', {
      roleName: GHA_API_ROLE,
      assumedBy: deployTrust,
    });
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:ListBucket'],
      resources: [`arn:aws:s3:::${deploymentsBucket}`],
      conditions: {StringLike: {'s3:prefix': `${S3_PREFIX}/*`}},
    }));
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:PutObject', 's3:GetObject'],
      resources: deploymentsPrefixArns,
    }));
    // The workflow reads /ctech/{env}/s3/deployments-bucket before uploading.
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: ['arn:aws:ssm:*:*:parameter/ctech/*'],
    }));
    // Trigger the rolling deploy on running instances via SSM RunCommand.
    // Scoped (issue #41): SendCommand only against instances tagged
    // Project=ctech-poker (the ASG instances — `cdk.Tags.of(app)` in
    // bin/poker.ts stamps every resource, and the ASG propagates tags at
    // launch) and only the AWS-RunShellScript document.
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:SendCommand'],
      resources: [`arn:aws:ec2:${this.region}:${this.account}:instance/*`],
      conditions: {
        StringEquals: {'ssm:resourceTag/Project': SERVICE},
      },
    }));
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:SendCommand'],
      resources: [`arn:aws:ssm:${this.region}::document/AWS-RunShellScript`],
    }));
    // Command-status polling. These read-only actions have no resource-level
    // scoping in IAM (SSM only supports `*` for them).
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: [
        'ssm:GetCommandInvocation',
        'ssm:ListCommands',
        'ssm:ListCommandInvocations',
      ],
      resources: ['*'],
    }));
    // Discover the InService instances of the ASG. Describe* has no
    // resource-level scoping; StartInstanceRefresh is pinned to poker's ASGs.
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: [
        'autoscaling:DescribeAutoScalingGroups',
        'autoscaling:DescribeInstanceRefreshes',
        'ec2:DescribeInstances',
      ],
      resources: ['*'],
    }));
    apiRole.addToPolicy(new iam.PolicyStatement({
      actions: ['autoscaling:StartInstanceRefresh'],
      resources: [
        `arn:aws:autoscaling:${this.region}:${this.account}:autoScalingGroup:*:autoScalingGroupName/*-${SERVICE}`,
      ],
    }));

    // ── Infra deploy role ───────────────────────────────────────────────────
    const infraRole = new iam.Role(this, 'InfraDeployRole', {
      roleName: GHA_INFRA_ROLE,
      assumedBy: infraTrust,
    });
    // Everything except IAM/Organizations/Account (issue #41 — was
    // AdministratorAccess).
    infraRole.addManagedPolicy(
      iam.ManagedPolicy.fromAwsManagedPolicyName('PowerUserAccess'),
    );
    // CDK needs to manage the app's IAM roles, instance profiles and policies,
    // and to pass them to the services that use them. Scoped to this service's
    // resources plus the CDK bootstrap/asset roles.
    const iamResourceScopes = [
      `arn:aws:iam::${this.account}:role/${SERVICE}*`,
      `arn:aws:iam::${this.account}:role/*-${SERVICE}*`,
      `arn:aws:iam::${this.account}:role/CtechPoker-*`,
      `arn:aws:iam::${this.account}:role/cdk-*`,
      `arn:aws:iam::${this.account}:policy/${SERVICE}*`,
      `arn:aws:iam::${this.account}:policy/CtechPoker-*`,
      `arn:aws:iam::${this.account}:instance-profile/${SERVICE}*`,
      `arn:aws:iam::${this.account}:instance-profile/*-${SERVICE}*`,
      `arn:aws:iam::${this.account}:instance-profile/CtechPoker-*`,
    ];
    infraRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ManageServiceIamResources',
      actions: [
        'iam:CreateRole', 'iam:DeleteRole', 'iam:GetRole', 'iam:UpdateRole',
        'iam:UpdateRoleDescription', 'iam:UpdateAssumeRolePolicy',
        'iam:TagRole', 'iam:UntagRole',
        'iam:AttachRolePolicy', 'iam:DetachRolePolicy',
        'iam:PutRolePolicy', 'iam:DeleteRolePolicy',
        'iam:PutRolePermissionsBoundary', 'iam:DeleteRolePermissionsBoundary',
        'iam:CreatePolicy', 'iam:DeletePolicy',
        'iam:CreatePolicyVersion', 'iam:DeletePolicyVersion',
        'iam:SetDefaultPolicyVersion', 'iam:TagPolicy', 'iam:UntagPolicy',
        'iam:CreateInstanceProfile', 'iam:DeleteInstanceProfile',
        'iam:AddRoleToInstanceProfile', 'iam:RemoveRoleFromInstanceProfile',
        'iam:TagInstanceProfile', 'iam:UntagInstanceProfile',
        'iam:PassRole',
      ],
      resources: iamResourceScopes,
    }));
    // Read-only IAM introspection CDK/CloudFormation need during diff/deploy.
    infraRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ReadOnlyIam',
      actions: [
        'iam:Get*', 'iam:List*',
        'iam:CreateServiceLinkedRole',
      ],
      resources: ['*'],
    }));
    // Hard ceiling: nothing this role does may mint new long-lived principals
    // or credentials, or touch the OIDC trust anchor / the org. Deny beats the
    // PowerUserAccess and the scoped allow above.
    infraRole.addToPolicy(new iam.PolicyStatement({
      sid: 'DenyPrivilegeEscalation',
      effect: iam.Effect.DENY,
      actions: [
        'iam:CreateUser', 'iam:CreateAccessKey', 'iam:CreateLoginProfile',
        'iam:UpdateLoginProfile', 'iam:CreateServiceSpecificCredential',
        'iam:UploadSSHPublicKey', 'iam:EnableMFADevice', 'iam:CreateVirtualMFADevice',
        'iam:DeactivateMFADevice', 'iam:ResyncMFADevice',
        'iam:CreateSAMLProvider', 'iam:UpdateSAMLProvider', 'iam:DeleteSAMLProvider',
        'iam:CreateOpenIDConnectProvider', 'iam:UpdateOpenIDConnectProviderThumbprint',
        'iam:AddClientIDToOpenIDConnectProvider',
        'iam:RemoveClientIDFromOpenIDConnectProvider',
        'iam:DeleteOpenIDConnectProvider',
      ],
      resources: ['*'],
    }));
    infraRole.addToPolicy(new iam.PolicyStatement({
      sid: 'DenyOrgAndAccountControl',
      effect: iam.Effect.DENY,
      actions: ['organizations:*', 'account:*'],
      resources: ['*'],
    }));

    new cdk.CfnOutput(this, 'ApiRoleArn', {value: apiRole.roleArn});
    new cdk.CfnOutput(this, 'InfraRoleArn', {value: infraRole.roleArn});
    new cdk.CfnOutput(this, 'ScopePublisherRoleArn', {value: scopesRole.roleArn});
  }
}

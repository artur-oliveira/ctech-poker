import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import {OidcStack} from '../lib/oidc-stack';
import {
  GHA_API_ROLE,
  GHA_DEPLOY_BRANCHES,
  GHA_INFRA_ROLE,
} from '../lib/constants';

const REPO = 'artur-oliveira/ctech-poker';

function synth() {
  const app = new App();
  const stack = new OidcStack(app, 'TestOidc', {
    env: {account: '123456789012', region: 'us-east-1'},
    githubRepo: REPO,
    deploymentsBucket: 'dev-ctech-deployments',
  });
  return Template.fromStack(stack);
}

const branchSubs = GHA_DEPLOY_BRANCHES.map(
  (b) => `repo:${REPO}:ref:refs/heads/${b}`,
);

function trustSubs(t: Template, roleName: string): string[] {
  const roles = t.findResources('AWS::IAM::Role', {
    Properties: {RoleName: roleName},
  });
  const role = Object.values(roles)[0] as any;
  const cond = role.Properties.AssumeRolePolicyDocument.Statement[0].Condition;
  const sub = cond.StringEquals['token.actions.githubusercontent.com:sub'];
  return Array.isArray(sub) ? sub : [sub];
}

test('trust conditions are pinned to exact refs — no bare wildcard', () => {
  const t = synth();
  const doc = JSON.stringify(t.toJSON());
  expect(doc).not.toContain(`repo:${REPO}:*`);
  expect(doc).not.toContain('@*/');
});

test('api/scopes roles trust only the deploy branches', () => {
  const t = synth();
  expect(trustSubs(t, GHA_API_ROLE).sort()).toEqual([...branchSubs].sort());
});

test('infra role additionally trusts pull_request for cdk diff', () => {
  const t = synth();
  expect(trustSubs(t, GHA_INFRA_ROLE).sort()).toEqual(
    [...branchSubs, `repo:${REPO}:pull_request`].sort(),
  );
});

test('infra role has PowerUserAccess, never AdministratorAccess', () => {
  const t = synth();
  const roles = t.findResources('AWS::IAM::Role', {
    Properties: {RoleName: GHA_INFRA_ROLE},
  });
  const arns = JSON.stringify(
    (Object.values(roles)[0] as any).Properties.ManagedPolicyArns,
  );
  expect(arns).toContain('PowerUserAccess');
  expect(arns).not.toContain('AdministratorAccess');
});

test('infra role denies IAM principal/credential creation', () => {
  const t = synth();
  t.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {
      Statement: Match.arrayWith([
        Match.objectLike({
          Effect: 'Deny',
          Action: Match.arrayWith(['iam:CreateUser', 'iam:CreateAccessKey']),
        }),
      ]),
    },
  });
});

test('api role SendCommand is tag-scoped to poker instances + RunShellScript', () => {
  const t = synth();
  t.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {
      Statement: Match.arrayWith([
        Match.objectLike({
          Action: 'ssm:SendCommand',
          Condition: {
            StringEquals: {'ssm:resourceTag/Project': 'ctech-poker'},
          },
        }),
        Match.objectLike({
          Action: 'ssm:SendCommand',
          Resource: Match.stringLikeRegexp('document/AWS-RunShellScript$'),
        }),
      ]),
    },
  });
});

test('api role has no unconditional SendCommand on *', () => {
  const t = synth();
  for (const pol of Object.values(t.findResources('AWS::IAM::Policy'))) {
    for (const st of (pol as any).Properties.PolicyDocument.Statement) {
      const actions = Array.isArray(st.Action) ? st.Action : [st.Action];
      if (actions.includes('ssm:SendCommand')) {
        expect(st.Resource).not.toBe('*');
      }
    }
  }
});

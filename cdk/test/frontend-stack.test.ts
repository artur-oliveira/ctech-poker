import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {FrontendStack} from '../lib/frontend-stack';

test('creates private S3 hosting and a CloudFront distribution', () => {
  const app = new App();
  const stack = new FrontendStack(app, 'TestFrontendStack', {
    environment: 'dev',
    certificateArn: 'arn:aws:acm:us-east-1:868899309401:certificate/test',
    domainName: 'poker-dev.aoctech.app',
    apiDomainName: 'poker-api-dev.aoctech.app',
    authDomainName: 'accounts-dev.aoctech.app',
    extraConnectSrc: [
      'https://challenges.cloudflare.com',
      'https://dev-ctech-poker-avatars.s3.us-east-1.amazonaws.com',
      'https://dev-ctech-poker-avatars.s3.dualstack.us-east-1.amazonaws.com',
    ],
  });
  const template = Template.fromStack(stack);
  // One bucket, not two: the avatars bucket moved to StorageStack when the
  // API took over serving /v1.0/avatars/*.
  template.resourceCountIs('AWS::S3::Bucket', 1);
  template.resourceCountIs('AWS::CloudFront::Distribution', 1);
  template.hasResourceProperties('AWS::CloudFront::Distribution', {
    DistributionConfig: {DefaultRootObject: 'index.html'},
  });
  const rendered = JSON.stringify(template.toJSON());
  // The avatar behaviour, its origin path and its rewrite Function are gone.
  expect(rendered).not.toContain('/avatars/*');
  expect(rendered).not.toContain('"OriginPath":"/av"');
  expect(rendered).not.toContain('dev-ctech-poker-avatar-rewrite');
  // The S3 origins stay in connect-src: the browser still POSTs the presigned
  // upload form straight to the bucket.
  expect(rendered).toContain('dev-ctech-poker-avatars.s3.us-east-1.amazonaws.com');
  expect(rendered).toContain('dev-ctech-poker-avatars.s3.dualstack.us-east-1.amazonaws.com');
});

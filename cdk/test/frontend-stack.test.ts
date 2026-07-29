import {App} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
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
  template.resourceCountIs('AWS::S3::Bucket', 2);
  template.resourceCountIs('AWS::CloudFront::Distribution', 1);
  template.hasResourceProperties('AWS::CloudFront::Distribution', {
    DistributionConfig: {
      DefaultRootObject: 'index.html',
      CacheBehaviors: Match.arrayWith([Match.objectLike({PathPattern: '/avatars/*'})]),
    },
  });
  template.hasResourceProperties('AWS::S3::Bucket', {
    BucketName: 'dev-ctech-poker-avatars',
    CorsConfiguration: {CorsRules: [{AllowedHeaders: ['*'], AllowedMethods: ['POST'],
      AllowedOrigins: ['https://poker-dev.aoctech.app'], MaxAge: 3000}]},
  });
  const rendered = JSON.stringify(template.toJSON());
  expect(rendered).toContain('dev-ctech-poker-avatars.s3.us-east-1.amazonaws.com');
  expect(rendered).toContain('dev-ctech-poker-avatars.s3.dualstack.us-east-1.amazonaws.com');
  expect(rendered).toContain('"OriginPath":"/av"');
});

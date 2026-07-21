import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {FrontendStack} from '../lib/frontend-stack';

test('creates private S3 hosting and a CloudFront distribution', () => {
  const app = new App();
  const stack = new FrontendStack(app, 'TestFrontendStack', {
    environment: 'dev',
    certificateArn: 'arn:aws:acm:us-east-1:868899309401:certificate/test',
    apiDomainName: 'poker-api-dev.aoctech.app',
    authDomainName: 'accounts-dev.aoctech.app',
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::S3::Bucket', 1);
  template.resourceCountIs('AWS::CloudFront::Distribution', 1);
  template.resourceCountIs('AWS::WAFv2::WebACL', 1);
  template.hasResourceProperties('AWS::CloudFront::Distribution', {
    DistributionConfig: {DefaultRootObject: 'index.html'},
  });
});

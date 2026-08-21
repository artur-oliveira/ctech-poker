import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {StorageStack} from '../lib/storage-stack';

test('creates the avatars bucket, private, with the quarantine lifecycle rule', () => {
  const template = Template.fromStack(new StorageStack(new App(), 'TestStorageStack', {
    environment: 'dev',
    appDomainName: 'poker-dev.aoctech.app',
  }));
  template.resourceCountIs('AWS::S3::Bucket', 1);
  template.hasResourceProperties('AWS::S3::Bucket', {
    BucketName: 'dev-ctech-poker-avatars',
    PublicAccessBlockConfiguration: {
      BlockPublicAcls: true, BlockPublicPolicy: true,
      IgnorePublicAcls: true, RestrictPublicBuckets: true,
    },
    // POST only, and only from the app origin: the browser uploads straight to
    // S3 with a presigned form. Reads go through the API, so no GET rule.
    CorsConfiguration: {
      CorsRules: [{
        AllowedHeaders: ['*'], AllowedMethods: ['POST'],
        AllowedOrigins: ['https://poker-dev.aoctech.app'], MaxAge: 3000,
      }],
    },
    // Unvalidated uploads must not linger: up/ is where a browser POSTs
    // arbitrary bytes before ValidateAndPublish copies them into av/.
    LifecycleConfiguration: {
      Rules: [{Id: 'ExpireAvatarQuarantine', Prefix: 'up/', ExpirationInDays: 1, Status: 'Enabled'}],
    },
  });
  // No bucket policy grants CloudFront (or anything else) read access.
  template.resourceCountIs('AWS::CloudFront::Distribution', 0);
});

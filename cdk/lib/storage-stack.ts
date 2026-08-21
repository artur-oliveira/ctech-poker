import * as cdk from 'aws-cdk-lib';
import * as s3 from 'aws-cdk-lib/aws-s3';
import {Environment} from '@aoctech/cdk';
import {Construct} from 'constructs';
import {avatarsBucketName} from './constants';

interface StorageStackProps extends cdk.StackProps {
  environment: Environment;
  /** App origin allowed to POST directly to the presigned upload URL. */
  appDomainName?: string;
}

/**
 * Long-lived object storage. Today that is only the avatars bucket, which used
 * to live in FrontendStack because CloudFront read from it. Nothing in front of
 * it survives the Cloudflare migration — the API serves /v1.0/avatars/* itself
 * now — so the bucket had to move out before FrontendStack is torn down, into a
 * stack whose lifecycle is player data rather than a CDN.
 *
 * Deliberately separate from PokerApiStack: the API stack is redeployed on
 * every release and replaces instances through an ASG refresh. A rollback there
 * must never be able to reach user-uploaded content.
 */
export class StorageStack extends cdk.Stack {
  public readonly avatarsBucket: s3.Bucket;

  constructor(scope: Construct, id: string, props: StorageStackProps) {
    super(scope, id, props);
    const isProd = props.environment === 'prod';

    // Same construct id ('AvatarsBucket') and the same physical name as the
    // FrontendStack version, so an existing bucket can be adopted with
    // `cdk import` instead of recreated. See docs/plans for the runbook.
    this.avatarsBucket = new s3.Bucket(this, 'AvatarsBucket', {
      bucketName: avatarsBucketName(props.environment),
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      versioned: isProd,
      removalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      autoDeleteObjects: !isProd,
      // The browser POSTs the presigned form straight to S3, so the bucket
      // itself needs CORS. Reads no longer do — those go through the API.
      cors: props.appDomainName ? [{
        allowedMethods: [s3.HttpMethods.POST],
        allowedOrigins: [`https://${props.appDomainName}`],
        allowedHeaders: ['*'],
        maxAge: 3000,
      }] : [],
      lifecycleRules: [{
        id: 'ExpireAvatarQuarantine',
        prefix: 'up/',
        expiration: cdk.Duration.days(1),
      }],
    });

    new cdk.CfnOutput(this, 'AvatarsBucketName', {value: this.avatarsBucket.bucketName});
  }
}

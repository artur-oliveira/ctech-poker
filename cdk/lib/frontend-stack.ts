import * as cdk from 'aws-cdk-lib';
import * as cloudfront from 'aws-cdk-lib/aws-cloudfront';
import * as origins from 'aws-cdk-lib/aws-cloudfront-origins';
import * as s3 from 'aws-cdk-lib/aws-s3';
import {createNextjsStaticFrontend, Environment} from '@aoctech/cdk';
import {Construct} from 'constructs';
import {
  API_PATH_PATTERNS,
  AVATAR_PUBLIC_PATH_PREFIX,
  AVATAR_STORAGE_PATH_PREFIX,
  avatarRewriteFunctionName,
  avatarsBucketName,
  frontendBucketName,
  routeStoreName,
  SERVICE,
} from './constants';

interface FrontendStackProps extends cdk.StackProps {
  environment: Environment;
  certificateArn: string;
  domainName?: string;
  apiDomainName: string;
  authDomainName: string;
  extraConnectSrc: string[];
}

export class FrontendStack extends cdk.Stack {
  public readonly bucket: s3.Bucket;
  public readonly avatarsBucket: s3.Bucket;
  public readonly distribution: cloudfront.Distribution;
  public readonly routeStore: cloudfront.KeyValueStore;

  constructor(scope: Construct, id: string, props: FrontendStackProps) {
    super(scope, id, props);
    const isProd = props.environment === 'prod';

    this.avatarsBucket = new s3.Bucket(this, 'AvatarsBucket', {
      bucketName: avatarsBucketName(props.environment),
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      versioned: isProd,
      removalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      autoDeleteObjects: !isProd,
      cors: props.domainName ? [{
        allowedMethods: [s3.HttpMethods.POST],
        allowedOrigins: [`https://${props.domainName}`],
        allowedHeaders: ['*'],
        maxAge: 3000,
      }] : [],
      lifecycleRules: [{
        id: 'ExpireAvatarQuarantine',
        prefix: 'up/',
        expiration: cdk.Duration.days(1),
      }],
    });

    const connectSrc = [
      `https://${props.apiDomainName}`,
      `https://${props.authDomainName}`,
      ...props.extraConnectSrc.map((host) => `https://${host}`),
      `wss://${props.apiDomainName}`,
    ];
    const {bucket, distribution, routeStore} = createNextjsStaticFrontend(this, {
      environment: props.environment,
      serviceName: SERVICE,
      bucketName: frontendBucketName(props.environment),
      routeStoreName: routeStoreName(props.environment),
      apiDomainName: props.apiDomainName,
      apiPathPatterns: API_PATH_PATTERNS,
      connectSrc,
      domainName: props.domainName,
      certificateArn: props.domainName ? props.certificateArn : undefined,
      distributionComment: `CTech Poker Frontend - ${props.environment}`,
      permissionsPolicy: 'on-device-speech-recognition=self',
      additionalBehaviors: ({originAccessControl}) => {
        const avatarHeaders = new cloudfront.ResponseHeadersPolicy(this, 'AvatarHeaders', {
          responseHeadersPolicyName: `${props.environment}-${SERVICE}-avatar-headers`,
          securityHeadersBehavior: {contentTypeOptions: {override: true}},
        });
        const avatarRewrite = new cloudfront.Function(this, 'AvatarRewrite', {
          functionName: avatarRewriteFunctionName(props.environment),
          runtime: cloudfront.FunctionRuntime.JS_2_0,
          code: cloudfront.FunctionCode.fromInline(`
function handler(event) {
  event.request.uri = event.request.uri.slice(${AVATAR_PUBLIC_PATH_PREFIX.length});
  return event.request;
}`),
        });
        return {
          [`${AVATAR_PUBLIC_PATH_PREFIX}/*`]: {
            origin: origins.S3BucketOrigin.withOriginAccessControl(this.avatarsBucket, {
              originAccessControl,
              originPath: AVATAR_STORAGE_PATH_PREFIX,
            }),
            viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
            cachePolicy: cloudfront.CachePolicy.CACHING_OPTIMIZED,
            allowedMethods: cloudfront.AllowedMethods.ALLOW_GET_HEAD_OPTIONS,
            compress: true,
            responseHeadersPolicy: avatarHeaders,
            functionAssociations: [{
              function: avatarRewrite,
              eventType: cloudfront.FunctionEventType.VIEWER_REQUEST,
            }],
          },
        };
      },
      outputExportNamePrefix: id,
    });

    this.bucket = bucket;
    this.distribution = distribution;
    this.routeStore = routeStore;

    new cdk.CfnOutput(this, 'AvatarsBucketName', {value: this.avatarsBucket.bucketName});
  }
}

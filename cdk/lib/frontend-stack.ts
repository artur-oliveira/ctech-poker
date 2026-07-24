import * as cdk from 'aws-cdk-lib';
import {Duration} from 'aws-cdk-lib';
import * as acm from 'aws-cdk-lib/aws-certificatemanager';
import * as cloudfront from 'aws-cdk-lib/aws-cloudfront';
import {HttpVersion} from 'aws-cdk-lib/aws-cloudfront';
import * as origins from 'aws-cdk-lib/aws-cloudfront-origins';
import * as s3 from 'aws-cdk-lib/aws-s3';
import * as wafv2 from 'aws-cdk-lib/aws-wafv2';
import {Environment} from '@aoctech/cdk';
import {Construct} from 'constructs';
import {API_PATH_PATTERNS, frontendBucketName, routeStoreName, SERVICE} from './constants';

interface FrontendStackProps extends cdk.StackProps {
  environment: Environment;
  certificateArn: string;
  domainName?: string;
  apiDomainName: string;
  authDomainName: string;
}

export class FrontendStack extends cdk.Stack {
  public readonly bucket: s3.Bucket;
  public readonly distribution: cloudfront.Distribution;
  public readonly routeStore: cloudfront.KeyValueStore;

  constructor(scope: Construct, id: string, props: FrontendStackProps) {
    super(scope, id, props);
    const {environment, certificateArn, domainName, apiDomainName, authDomainName} = props;
    const isProd = environment === 'prod';

    this.bucket = new s3.Bucket(this, 'Bucket', {
      bucketName: frontendBucketName(environment),
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      versioned: isProd,
      removalPolicy: isProd ? cdk.RemovalPolicy.RETAIN : cdk.RemovalPolicy.DESTROY,
      autoDeleteObjects: !isProd,
    });
    const oac = new cloudfront.S3OriginAccessControl(this, 'OAC', {
      originAccessControlName: `${environment}-${SERVICE}-oac`,
    });
    this.routeStore = new cloudfront.KeyValueStore(this, 'RouteStore', {
      keyValueStoreName: routeStoreName(environment),
    });
    const rewrite = new cloudfront.Function(this, 'UrlRewrite', {
      functionName: `${environment}-${SERVICE}-url-rewrite`,
      runtime: cloudfront.FunctionRuntime.JS_2_0,
      keyValueStore: this.routeStore,
      code: cloudfront.FunctionCode.fromInline(`
import cf from 'cloudfront';
const kvs = cf.kvs();
async function handler(event) {
  var uri = event.request.uri;
  if (uri === '/' || /\\.[^/]+$/.test(uri)) return event.request;
  var route = uri.endsWith('/') ? uri.slice(0, -1) : uri;
  event.request.uri = (await kvs.exists(route)) ? route + '.html' : '/404.html';
  return event.request;
}`),
    });
    const securityHeaders = new cloudfront.ResponseHeadersPolicy(this, 'SecurityHeaders', {
      responseHeadersPolicyName: `${environment}-${SERVICE}-security-headers`,
      securityHeadersBehavior: {
        contentTypeOptions: {override: true},
        frameOptions: {frameOption: cloudfront.HeadersFrameOption.DENY, override: true},
        strictTransportSecurity: {
          accessControlMaxAge: Duration.seconds(63072000), includeSubdomains: true, preload: true, override: true,
        },
        referrerPolicy: {
          referrerPolicy: cloudfront.HeadersReferrerPolicy.STRICT_ORIGIN_WHEN_CROSS_ORIGIN, override: true,
        },
        contentSecurityPolicy: {
          contentSecurityPolicy: [
            "default-src 'self'", "base-uri 'self'", "object-src 'none'", "frame-ancestors 'none'",
            "img-src 'self' data:", "style-src 'self' 'unsafe-inline'", "script-src 'self' 'unsafe-inline'",
            `connect-src 'self' https://${authDomainName}`,
          ].join('; '),
          override: true,
        },
      },
    });
    const apiBehavior: cloudfront.BehaviorOptions = {
      origin: new origins.HttpOrigin(apiDomainName, {
        protocolPolicy: cloudfront.OriginProtocolPolicy.HTTPS_ONLY,
        readTimeout: cdk.Duration.seconds(60),
        keepaliveTimeout: cdk.Duration.seconds(60),
      }),
      viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.HTTPS_ONLY,
      cachePolicy: cloudfront.CachePolicy.CACHING_DISABLED,
      originRequestPolicy: cloudfront.OriginRequestPolicy.ALL_VIEWER_EXCEPT_HOST_HEADER,
      allowedMethods: cloudfront.AllowedMethods.ALLOW_ALL,
      compress: true,
      responseHeadersPolicy: securityHeaders,
    };
    this.distribution = new cloudfront.Distribution(this, 'Distribution', {
      comment: `CTech Poker Frontend - ${environment}`,
      defaultRootObject: 'index.html',
      defaultBehavior: {
        origin: origins.S3BucketOrigin.withOriginAccessControl(this.bucket, {originAccessControl: oac}),
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
        cachePolicy: cloudfront.CachePolicy.CACHING_OPTIMIZED,
        allowedMethods: cloudfront.AllowedMethods.ALLOW_GET_HEAD_OPTIONS,
        compress: true,
        responseHeadersPolicy: securityHeaders,
        functionAssociations: [{function: rewrite, eventType: cloudfront.FunctionEventType.VIEWER_REQUEST}],
      },
      additionalBehaviors: Object.fromEntries(API_PATH_PATTERNS.map((pattern) => [pattern, apiBehavior])),
      httpVersion: HttpVersion.HTTP2_AND_3,
      certificate: domainName ? acm.Certificate.fromCertificateArn(this, 'Cert', certificateArn) : undefined,
      domainNames: domainName ? [domainName] : undefined,
      priceClass: cloudfront.PriceClass.PRICE_CLASS_100,
      minimumProtocolVersion: cloudfront.SecurityPolicyProtocol.TLS_V1_2_2021,
    });
    new cdk.CfnOutput(this, 'BucketName', {value: this.bucket.bucketName, exportName: `${id}-bucket-name`});
    new cdk.CfnOutput(this, 'DistributionId', {
      value: this.distribution.distributionId, exportName: `${id}-dist-id`,
    });
    new cdk.CfnOutput(this, 'DistributionDomain', {
      value: this.distribution.distributionDomainName, exportName: `${id}-dist-domain`,
    });
    new cdk.CfnOutput(this, 'RouteStoreArn', {
      value: this.routeStore.keyValueStoreArn, exportName: `${id}-route-store-arn`,
    });
  }
}

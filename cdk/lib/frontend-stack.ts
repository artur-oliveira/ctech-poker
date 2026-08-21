import * as cdk from 'aws-cdk-lib';
import * as cloudfront from 'aws-cdk-lib/aws-cloudfront';
import * as s3 from 'aws-cdk-lib/aws-s3';
import {createNextjsStaticFrontend, Environment} from '@aoctech/cdk';
import {Construct} from 'constructs';
import {
  API_PATH_PATTERNS,
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
  public readonly distribution: cloudfront.Distribution;
  public readonly routeStore: cloudfront.KeyValueStore;

  constructor(scope: Construct, id: string, props: FrontendStackProps) {
    super(scope, id, props);
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
      // No /avatars/* behaviour any more: the API serves them from
      // /v1.0/avatars/*, which is already covered by API_PATH_PATTERNS.
      outputExportNamePrefix: id,
    });

    this.bucket = bucket;
    this.distribution = distribution;
    this.routeStore = routeStore;
  }
}

// Archives poker_action_log's DynamoDB Stream to S3 before logTTLDays (see
// api/internal/tablestore's guardTTLDays/logTTLDays constants) reaps the hot
// copy — DynamoDB serves the recent window, S3 is the indefinite archive.
import {execFileSync} from 'child_process';
import * as cdk from 'aws-cdk-lib';
import {ILocalBundling, RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as s3 from 'aws-cdk-lib/aws-s3';
import {DynamoEventSource} from 'aws-cdk-lib/aws-lambda-event-sources';
import {StartingPosition} from 'aws-cdk-lib/aws-lambda';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';

// localGoBundling builds the archiver binary with the host's own Go
// toolchain, skipping Docker entirely when `go` is on PATH — Docker-in-CI
// (or in a sandboxed dev environment with no Docker daemon) shouldn't be a
// hard requirement just to bundle a single static Go binary. Falls back to
// the Docker-based `bundling.image`/`command` path (still configured below)
// when `go` isn't available.
const localGoBundling: ILocalBundling = {
  tryBundle(outputDir: string): boolean {
    try {
      execFileSync('go', ['build', '-o', `${outputDir}/bootstrap`, '.'], {
        cwd: '../api/cmd/archiver',
        env: {...process.env, GOOS: 'linux', GOARCH: 'arm64', CGO_ENABLED: '0'},
        stdio: 'inherit',
      });
      return true;
    } catch {
      return false; // no local Go toolchain — fall back to Docker bundling
    }
  },
};

interface ArchiverStackProps extends cdk.StackProps {
  environment: Environment;
  actionLogTable: dynamodb.ITableV2;
}

export class ArchiverStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ArchiverStackProps) {
    super(scope, id, props);
    const {environment, actionLogTable} = props;

    const bucket = new s3.Bucket(this, 'ActionLogArchive', {
      bucketName: `poker-action-log-archive-${environment}`,
      removalPolicy: environment === 'dev' ? RemovalPolicy.DESTROY : RemovalPolicy.RETAIN,
      autoDeleteObjects: environment === 'dev',
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
    });

    const fn = new lambda.Function(this, 'ArchiverFn', {
      functionName: `${environment}-poker-action-log-archiver`,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('../api/cmd/archiver', {
        bundling: {
          local: localGoBundling,
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: ['bash', '-c', 'GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap .'],
        },
      }),
      environment: {ARCHIVE_BUCKET: bucket.bucketName},
      timeout: cdk.Duration.seconds(30),
    });

    bucket.grantWrite(fn);
    fn.addEventSource(new DynamoEventSource(actionLogTable, {
      startingPosition: StartingPosition.TRIM_HORIZON,
      batchSize: 100,
      retryAttempts: 3,
    }));
  }
}

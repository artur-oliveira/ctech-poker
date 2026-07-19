#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib';
import {Environment} from '@aoctech/cdk';

import {PokerApiStack} from '../lib/api-stack';
import {DynamoDBStack} from '../lib/dynamodb-stack';
import {ArchiverStack} from '../lib/archiver-stack';
import {
  API_DOMAIN_PREFIX,
  AWS_ACCOUNT,
  AWS_REGION,
  domainForEnv,
  instanceProfileName,
} from '../lib/constants';

const app = new cdk.App();

const ENVIRONMENT = (process.env.ENVIRONMENT || 'dev') as Environment;

// VPC is managed by ctech-cdk (shared across every CTech service in this
// account — same default used by ctech-wallet/ctech-dfe/ctech-account). The ID
// must be a concrete string (not a token) because ec2.Vpc.fromLookup resolves
// subnet/AZ metadata at synthesis time. CI reads /ctech/{env}/network/vpc-id
// from SSM and exports it as CTECH_VPC_ID before running cdk deploy.
const CTECH_VPC_ID = process.env.CTECH_VPC_ID || 'vpc-0adfd86727d17445b';
// Shared S3 buckets owned by ctech-cdk. CI reads these from SSM
// (/ctech/{env}/s3/deployments-bucket and /ctech/{env}/s3/logs-bucket)
// and sets them as env vars before running cdk deploy.
const CTECH_DEPLOYMENTS_BUCKET = process.env.CTECH_DEPLOYMENTS_BUCKET || `${ENVIRONMENT}-ctech-deployments`;
const CTECH_LOGS_BUCKET = process.env.CTECH_LOGS_BUCKET || `${ENVIRONMENT}-ctech-application-logs`;

const env = {account: AWS_ACCOUNT, region: AWS_REGION};

const id = (name: string) =>
    `CtechPoker-${ENVIRONMENT.charAt(0).toUpperCase() + ENVIRONMENT.slice(1)}-${name}`;

// =====================
// DynamoDB (table state, action log + archival)
// =====================
const dynamoStack = new DynamoDBStack(app, id('DynamoDB'), {
  env,
  environment: ENVIRONMENT,
  description: `CTech Poker DynamoDB tables - ${ENVIRONMENT}`,
});

new ArchiverStack(app, id('Archiver'), {
  env,
  environment: ENVIRONMENT,
  actionLogTable: dynamoStack.tables.get('poker_action_log')!,
  description: `CTech Poker action-log archiver (DynamoDB Streams -> S3) - ${ENVIRONMENT}`,
});

// =====================
// API (EC2 + ASG, shared ALB from ctech-cdk)
// =====================
// NOTE: instanceProfileName points at an IAM instance profile that does not
// exist yet — this task only scaffolds the CDK stack skeleton, no IAM stack
// has been written for ctech-poker. `cdk deploy` will fail until a future
// task creates `${env}-ctech-poker-api-instance-profile` (see constants.ts).
new PokerApiStack(app, id('API'), {
  env,
  environment: ENVIRONMENT,
  vpcId: CTECH_VPC_ID,
  domainName: domainForEnv(ENVIRONMENT, API_DOMAIN_PREFIX),
  instanceProfileName: instanceProfileName(ENVIRONMENT),
  deploymentsBucketName: CTECH_DEPLOYMENTS_BUCKET,
  logsBucketName: CTECH_LOGS_BUCKET,
  tableStateArn: dynamoStack.tables.get('poker_table_state')!.tableArn,
  actionLogArn: dynamoStack.tables.get('poker_action_log')!.tableArn,
  actionGuardsArn: dynamoStack.tables.get('poker_action_guards')!.tableArn,
  description: `CTech Poker API (EC2 + ASG + ALB) - ${ENVIRONMENT}`,
});

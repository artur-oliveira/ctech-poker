import {App, Stack} from 'aws-cdk-lib';
import {Match, Template} from 'aws-cdk-lib/assertions';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {ArchiverStack} from '../lib/archiver-stack';

test('creates an archive bucket and a Lambda subscribed to the action log stream', () => {
  const app = new App();
  const tableStack = new Stack(app, 'TestTableStack');
  const table = new dynamodb.TableV2(tableStack, 'TestActionLog', {
    partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
    sortKey: {name: 'sk', type: dynamodb.AttributeType.STRING},
    dynamoStream: dynamodb.StreamViewType.NEW_IMAGE,
  });

  const stack = new ArchiverStack(app, 'TestArchiverStack', {
    environment: 'dev', actionLogTable: table
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::S3::Bucket', 1);
  // Note: autoDeleteObjects (dev only) provisions its own singleton Lambda
  // (Custom::S3AutoDeleteObjects) alongside the archiver function itself —
  // assert on the archiver function by name rather than a total count.
  template.hasResourceProperties('AWS::Lambda::Function', {FunctionName: 'dev-poker-action-log-archiver'});
  template.resourceCountIs('AWS::Lambda::EventSourceMapping', 1);

  // B10: poison records are reported to the shared alerts topic, not dropped and not parked in a queue.
  template.resourceCountIs('AWS::SQS::Queue', 0);
  template.hasResourceProperties('AWS::Lambda::EventSourceMapping', {
    BisectBatchOnFunctionError: true,
    DestinationConfig: {OnFailure: {Destination: Match.anyValue()}},
  });
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {Statement: Match.arrayWith([Match.objectLike({Action: 'sns:Publish', Effect: 'Allow'})])},
  });
  // Poker creates no CloudWatch alarms and no SNS topic.
  template.resourceCountIs('AWS::CloudWatch::Alarm', 0);
  template.resourceCountIs('AWS::SNS::Topic', 0);
});


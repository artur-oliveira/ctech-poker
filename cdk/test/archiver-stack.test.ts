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

  const stack = new ArchiverStack(app, 'TestArchiverStack', {environment: 'dev', actionLogTable: table});
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::S3::Bucket', 1);
  // Note: autoDeleteObjects (dev only) provisions its own singleton Lambda
  // (Custom::S3AutoDeleteObjects) alongside the archiver function itself —
  // assert on the archiver function by name rather than a total count.
  template.hasResourceProperties('AWS::Lambda::Function', {FunctionName: 'dev-poker-action-log-archiver'});
  template.resourceCountIs('AWS::Lambda::EventSourceMapping', 1);

  // B10: poison records must land in a DLQ (not be dropped) and alarm.
  template.hasResourceProperties('AWS::SQS::Queue', {QueueName: 'dev-poker-action-log-archiver-dlq'});
  template.hasResourceProperties('AWS::Lambda::EventSourceMapping', {
    BisectBatchOnFunctionError: true,
    DestinationConfig: {OnFailure: {Destination: {'Fn::GetAtt': [Match.stringLikeRegexp('ArchiverDLQ'), 'Arn']}}},
  });
  template.hasResourceProperties('AWS::CloudWatch::Alarm', {
    AlarmName: 'dev-poker-action-log-archiver-dlq-messages',
    MetricName: 'ApproximateNumberOfMessagesVisible',
    Threshold: 1,
  });
});

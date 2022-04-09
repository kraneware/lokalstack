package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/kraneware/core-go/awsutil/services"
)

// NewKeySchema creates a new array of KeySchemaElement for DynamoDB table creation
func NewKeySchema(hashAttr string, rangeAttr *string) []*dynamodb.KeySchemaElement {
	result := []*dynamodb.KeySchemaElement{
		{
			AttributeName: aws.String(hashAttr),
			KeyType:       aws.String("HASH"),
		},
	}
	if rangeAttr != nil {
		result = append(result, &dynamodb.KeySchemaElement{
			AttributeName: rangeAttr,
			KeyType:       aws.String("RANGE"),
		})
	}

	return result
}

// NewAttributeDefinition creates a new AttributeDefinition
func NewAttributeDefinition(attrName string, attrType string) *dynamodb.AttributeDefinition {
	return &dynamodb.AttributeDefinition{
		AttributeName: aws.String(attrName),
		AttributeType: aws.String(attrType),
	}
}

// NewGlobalSecondaryIndex creates a new global secondary index
func NewGlobalSecondaryIndex(name string, hashAttr string, rangeAttr *string) *dynamodb.GlobalSecondaryIndex {
	return &dynamodb.GlobalSecondaryIndex{
		IndexName: aws.String(name),
		KeySchema: NewKeySchema(hashAttr, rangeAttr),
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(2),
			WriteCapacityUnits: aws.Int64(2),
		},
		Projection: &dynamodb.Projection{
			ProjectionType: aws.String("ALL"),
		},
	}
}

// NewLocalSecondaryIndex creates a new global secondary index
func NewLocalSecondaryIndex(name string, hashAttr string, rangeAttr string) *dynamodb.LocalSecondaryIndex {
	return &dynamodb.LocalSecondaryIndex{
		IndexName: aws.String(name),
		KeySchema: NewKeySchema(hashAttr, &rangeAttr),
		Projection: &dynamodb.Projection{
			ProjectionType: aws.String("ALL"),
		},
	}
}

// NewTable creates a new table
func NewTable(
	ctx context.Context,
	tableName string,
	attrDefs []*dynamodb.AttributeDefinition,
	keySchema []*dynamodb.KeySchemaElement,
	global []*dynamodb.GlobalSecondaryIndex,
	local []*dynamodb.LocalSecondaryIndex,
	ttlAttr *string,
) (
	err error,
) {
	fmt.Println("  - Creating " + tableName + " table for testing")

	input := &dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		AttributeDefinitions: attrDefs,
		KeySchema:            keySchema,
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(2),
			WriteCapacityUnits: aws.Int64(2),
		},
	}

	if len(global) > 0 {
		input.GlobalSecondaryIndexes = global
	}
	if len(local) > 0 {
		input.LocalSecondaryIndexes = local
	}

	_, err = services.DynamoDbClient().CreateTableWithContext(ctx, input)

	if err == nil && ttlAttr != nil {
		err = AddTTL(ctx, tableName, *ttlAttr)
	}

	return err
}

func newLambdaZip(pythonCode string) (r *bytes.Buffer, err error) {
	pythonEmptyLambda := fmt.Sprintf(
		"def handler(event, context):\n"+
			"  %s\n", pythonCode)

	r = new(bytes.Buffer)
	writer := zip.NewWriter(r)
	defer writer.Close()

	var f io.Writer
	f, err = writer.Create("handler.py")
	if err == nil {
		_, err = f.Write([]byte(pythonEmptyLambda))
	}

	return r, err
}

// NewLambda creates a new lambda with the given Python code and deploys to localstack
func NewLambda(
	ctx context.Context,
	functionName string,
	pythonCode string,
) (err error) {
	fmt.Println("  - Creating " + functionName + " lambda function for testing")

	var zipContents *bytes.Buffer
	zipContents, err = newLambdaZip(pythonCode)

	if err == nil {
		input := &lambda.CreateFunctionInput{
			Code: &lambda.FunctionCode{
				ZipFile: zipContents.Bytes(),
			},
			FunctionName: aws.String(functionName),
			Handler:      aws.String("handler.handler"),
			Runtime:      aws.String("python3.6"),
			Role:         aws.String("test"),
			Publish:      aws.Bool(true),
		}
		_, err = services.LambdaClient().
			CreateFunctionWithContext(ctx, input)
	}

	return err
}

// AddTTL adds a TTL to a DynamoDB table
func AddTTL(
	ctx context.Context,
	tableName string,
	attrName string,
) (err error) {
	fmt.Println("  - Adding TTL for", tableName, " table")

	ttlInput := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &dynamodb.TimeToLiveSpecification{
			AttributeName: aws.String(attrName),
			Enabled:       aws.Bool(true),
		},
	}

	_, err = services.DynamoDbClient().UpdateTimeToLiveWithContext(ctx, ttlInput)

	return err
}

// NewAPIGW creates a new apigw  for testing
func NewAPIGW(
	ctx context.Context,
	endpoint string,
) (err error) {
	fmt.Println("  - Creating " + endpoint + " APIGW for testing")

	input := &apigateway.GetDomainNamesInput{}

	_, err = services.APIGWClient().GetDomainNamesWithContext(ctx, input)

	return err
}

// NewS3Bucket creates a new S3 bucket for testing
func NewS3Bucket(
	ctx context.Context,
	bucketName string,
) (err error) {
	fmt.Println("  - Creating " + bucketName + " S3 Bucket for testing")

	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}
	_, err = services.S3Client().CreateBucketWithContext(ctx, input)

	return err
}

func NewS3BucketObject(ctx context.Context, bucketName string, key string, content []byte) error {
	fmt.Println("  - Creating object for " + bucketName + " S3 Bucket for testing")

	testString := "Test s3 bucket object content"
	reader := strings.NewReader(testString)

	input := &s3.PutObjectInput{
		ACL:      nil,
		Body:     reader,
		Bucket:   aws.String(bucketName),
		Key:      aws.String(key),
		Metadata: nil,
	}

	_, err := services.S3Client().PutObject(input)

	return err
}

// NewSQS creates a new sqs queue
func NewSQS(
	ctx context.Context,
	queueName string,
	attributes map[string]*string,
) (queueOutput *sqs.CreateQueueOutput, err error) {
	queueInput := &sqs.CreateQueueInput{
		Attributes: attributes,
		QueueName:  aws.String(queueName),
	}

	queueOutput, err = services.SQSClient().CreateQueueWithContext(ctx, queueInput)

	return
}

// NewSNSTopic creates a new sns topic
func NewSNSTopic(
	ctx context.Context,
	topicName string,
	attributes map[string]*string,
) (topicOutput *sns.CreateTopicOutput, err error) {
	topicInput := &sns.CreateTopicInput{
		Attributes: attributes,
		Name:       aws.String(topicName),
	}

	topicOutput, err = services.SNSClient().CreateTopicWithContext(ctx, topicInput)

	return
}

func NewEC2Instance(ctx context.Context, endpoint string) (res *ec2.Reservation, err error) {
	fmt.Println("  - Creating " + endpoint + " ec2 instance for testing")

	res, err = services.EC2Client().RunInstancesWithContext(ctx, &ec2.RunInstancesInput{
		AdditionalInfo:                    nil,
		BlockDeviceMappings:               nil,
		CapacityReservationSpecification:  nil,
		ClientToken:                       nil,
		CpuOptions:                        nil,
		CreditSpecification:               nil,
		DisableApiTermination:             nil,
		DryRun:                            nil,
		EbsOptimized:                      nil,
		ElasticGpuSpecification:           nil,
		ElasticInferenceAccelerators:      nil,
		EnclaveOptions:                    nil,
		HibernationOptions:                nil,
		IamInstanceProfile:                nil,
		ImageId:                           nil,
		InstanceInitiatedShutdownBehavior: nil,
		InstanceMarketOptions:             nil,
		InstanceType:                      aws.String("t2.medium"),
		Ipv6AddressCount:                  nil,
		Ipv6Addresses:                     nil,
		KernelId:                          nil,
		KeyName:                           nil,
		LaunchTemplate:                    nil,
		LicenseSpecifications:             nil,
		MaxCount:                          aws.Int64(1),
		MetadataOptions:                   nil,
		MinCount:                          aws.Int64(1),
		Monitoring:                        nil,
		NetworkInterfaces:                 nil,
		Placement:                         nil,
		PrivateDnsNameOptions:             nil,
		PrivateIpAddress:                  nil,
		RamdiskId:                         nil,
		SecurityGroupIds:                  nil,
		SecurityGroups:                    nil,
		SubnetId:                          nil,
		TagSpecifications:                 nil,
		UserData:                          nil,
	})

	return res, err
}

package main_test

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/kraneware/core-go/awsutil/services"
	. "github.com/kraneware/lokalstack/main"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAWSServices(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AWS Services Test Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("AWS_XRAY_CONTEXT_MISSING", "LOG_ERROR")).To(BeNil())
	Expect(StartContainer()).Should(BeNil())
})

var _ = AfterSuite(func() {
	Expect(StopContainer()).Should(BeNil())
})

var _ = Describe("AWS Testing Services", func() {
	Context("Functions Test", func() {
		It("should create test DynamoDB using support functions", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()

			err := NewTable(
				testCtx,
				"testTable",
				[]*dynamodb.AttributeDefinition{
					NewAttributeDefinition("testAttr1", "S"),
					NewAttributeDefinition("testAttr2", "S"),
					NewAttributeDefinition("testAttr3", "S"),
				},
				NewKeySchema("testAttr1", aws.String("testAttr2")),
				[]*dynamodb.GlobalSecondaryIndex{
					NewGlobalSecondaryIndex("testIndex", "testAttr2", nil),
				},
				[]*dynamodb.LocalSecondaryIndex{
					NewLocalSecondaryIndex("testIndex1", "testAttr1", "testAttr3"),
				},
				nil)

			Expect(err).Should(BeNil())
		})
		It("should add TTL to the test table", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()
			Expect(AddTTL(testCtx, "testTable", "ttl")).Should(BeNil())

			output, err := services.DynamoDbClient().DescribeTimeToLiveWithContext(
				testCtx,
				&dynamodb.DescribeTimeToLiveInput{
					TableName: aws.String("testTable"),
				},
			)
			Expect(err).Should(BeNil())
			Expect(*output.TimeToLiveDescription.AttributeName).Should(Equal("ttl"))
		})
		It("should create test DynamoDB using support functions with TTL", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()
			err := NewTable(
				testCtx,
				"testTable1",
				[]*dynamodb.AttributeDefinition{
					NewAttributeDefinition("testAttr1", "S"),
					NewAttributeDefinition("testAttr2", "S"),
					NewAttributeDefinition("testAttr3", "S"),
				},
				NewKeySchema("testAttr1", aws.String("testAttr2")),
				[]*dynamodb.GlobalSecondaryIndex{
					NewGlobalSecondaryIndex("testIndex", "testAttr2", nil),
				},
				[]*dynamodb.LocalSecondaryIndex{
					NewLocalSecondaryIndex("testIndex1", "testAttr1", "testAttr3"),
				},
				aws.String("ttl"))

			Expect(err).Should(BeNil())

			var output *dynamodb.DescribeTimeToLiveOutput

			output, err = services.DynamoDbClient().DescribeTimeToLiveWithContext(
				testCtx,
				&dynamodb.DescribeTimeToLiveInput{
					TableName: aws.String("testTable"),
				},
			)

			Expect(err).Should(BeNil())
			Expect(*output.TimeToLiveDescription.AttributeName).Should(Equal("ttl"))
		})
		It("should create a lambda with Python code", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()
			Expect(NewLambda(testCtx, "myFunction", "return {}")).Should(BeNil())

			output, err := services.LambdaClient().GetFunctionWithContext(
				testCtx,
				&lambda.GetFunctionInput{
					FunctionName: aws.String("myFunction"),
				},
			)
			Expect(err).Should(BeNil())
			Expect(*output.Configuration.Runtime).Should(Equal("python3.6"))
		})
		It("should create a test S3 bucket", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()
			Expect(NewS3Bucket(testCtx, "testBucket")).Should(BeNil())
		})
		It("should create new sqs queue", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()

			attributes := map[string]*string{
				"DelaySeconds":              aws.String("5"),
				"VisibilityTimeout":         aws.String("10"),
				"ContentBasedDeduplication": aws.String("true"),
			}
			output, err := NewSQS(testCtx, "testQueue.fifo", attributes)
			Expect(err).Should(BeNil())
			Expect(output).ShouldNot(BeNil())
		})
		It("should create new sns topic", func() {
			testCtx, td := services.NewTestDaemon()
			defer td.Close()

			var attributes map[string]*string
			output, err := NewSNSTopic(testCtx, "testTopic", attributes)
			Expect(err).Should(BeNil())
			Expect(output).ShouldNot(BeNil())
			Expect(output.TopicArn).ShouldNot(BeNil())
		})
		//It("should create new ec2 instance", func() {
		//	testCtx, td := services.NewTestDaemon()
		//	defer td.Close()
		//
		//	//var attributes map[string]*string
		//	output, err := NewEC2Instance(testCtx, "testTopic")
		//	Expect(err).Should(BeNil())
		//	Expect(output).ShouldNot(BeNil())
		//	//Expect(output.TopicArn).ShouldNot(BeNil())
		//})
	})
})

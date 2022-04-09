package lokalstack

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/kraneware/kws/config"
	"github.com/kraneware/kws/services"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/aws/aws-sdk-go/aws/endpoints"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/ory/dockertest"
	"github.com/pkg/errors"

	dc "github.com/ory/dockertest/docker"
)

var resource *dockertest.Resource // nolint:gochecknoglobals
var resourcePool *dockertest.Pool // nolint:gochecknoglobals

const (
	// GenericEmptyLambda is the name of the testing lambda for the sake of routing
	GenericEmptyLambda = "generic_empty_lambda"
	TestRegion         = endpoints.UsEast1RegionID
)

func xrayInit() (err error) { // nolint:gochecknoinits
	if config.Endpoints.XRay != "" {
		fmt.Println("Configuring test xray context missing strategy")
		cms := &services.TestContextMissingStrategy{}
		err = xray.Configure(xray.Config{ContextMissingStrategy: cms})
	}
	return
}

func dynamoClientReady(ctx context.Context) (err error) {
	_, err = services.DynamoDbClient().ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
	return
}

func lambdaClientReady(ctx context.Context) (err error) {
	_, err = services.LambdaClient().ListFunctionsWithContext(ctx, &lambda.ListFunctionsInput{
		Marker:   aws.String(""),
		MaxItems: aws.Int64(128),
	})
	return err
}

func snsClientReady(ctx context.Context) (err error) {
	_, err = services.SNSClient().ListTopicsWithContext(ctx, &sns.ListTopicsInput{})
	return err
}

func sqsClientReady(ctx context.Context) (err error) {
	_, err = services.SQSClient().ListQueuesWithContext(ctx, &sqs.ListQueuesInput{})
	return err
}

func s3ClientReady(ctx context.Context) (err error) {
	_, err = services.S3Client().ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
	return err
}

func apigwClientReady(ctx context.Context) (err error) {
	_, err = services.APIGWClient().GetDomainNamesWithContext(ctx, &apigateway.GetDomainNamesInput{})
	return nil
}

func checkContainerReady() (err error) {
	fmt.Println(fmt.Sprintf("Checking if container is ready with aws region: %s ...", config.Region))
	err = xrayInit()
	if err == nil {
		testCtx, td := services.NewTestDaemon()
		defer td.Close()

		fmt.Println("Initialized test xray")
		var errGroup errgroup.Group

		// Test availability of services
		errGroup.Go(func() error {
			return dynamoClientReady(testCtx)
		})
		errGroup.Go(func() error {
			return lambdaClientReady(testCtx)
		})
		errGroup.Go(func() error {
			return snsClientReady(testCtx)
		})
		errGroup.Go(func() error {
			return sqsClientReady(testCtx)
		})
		errGroup.Go(func() error {
			return s3ClientReady(testCtx)
		})
		errGroup.Go(func() error {
			return apigwClientReady(testCtx)
		})

		err = errGroup.Wait()
	}

	return err
}

// CreatePortBindings converts port numbers to bindings
func CreatePortBindings(ports ...string) (res map[dc.Port][]dc.PortBinding) {
	res = map[dc.Port][]dc.PortBinding{}
	for _, port := range ports {
		res[dc.Port(port+"/tcp")] = []dc.PortBinding{{HostIP: "localhost", HostPort: port}}
	}
	return
}
func DefaultPortBindings() (res map[dc.Port][]dc.PortBinding) {
	res = CreatePortBindings(
		"4566", // Universal port
		"4572", // S3
		"4569", // Dynamodb
		"4574", // Lambda
		"4575", // SNS
		"4576", // SQS
		"4582", // CloudWatch
		"4586", // CloudWatchLogs
		"4603", // XRay
		"4594", // RDS
		"4583", // SSM
		"4597", //EC2
	)
	return
}

// GetDefaultLocalstackCredentials might be used to make sure that
// we use the same credentials in our test code.
func GetDefaultLocalstackCredentials() *credentials.Credentials {
	return credentials.NewStaticCredentials("foo", "bar", "")
}

// StartContainer starts a localstack container for testing purposes and builds out needed infrastructure
func StartContainer() (err error) { // nolint:funlen
	fmt.Println("Starting localstack container ... ")

	if resourcePool == nil {
		resourcePool, err = dockertest.NewPool("")
	}

	if err == nil {
		// Starting localstack docker container with port mappings
		// Lambdas in golang require 'LAMBDA_EXECUTOR=docker'
		// Privileged access is required to start docker inside the container
		resource, err = resourcePool.RunWithOptions(
			&dockertest.RunOptions{
				Repository:   "localstack/localstack",
				Tag:          "0.11.3", //"0.14.1",
				PortBindings: DefaultPortBindings(),
				// Env should be []string{} for python lambdas
				// should be []string{"LAMBDA_EXECUTOR=docker"}, for non-python lambdas
				// Using python for test downstream lambdas
				Env: []string{
					"DEBUG=1",
					"LOCALSTACK_API_KEY=" + os.Getenv("LOCALSTACK_API_KEY"), // Will fail if Pro is not activated. Lambdas only available in Pro
					"EXTERNAL_SERVICE_PORTS_START=4510",
					"EXTERNAL_SERVICE_PORTS_END=4597",
				},
				Privileged: true,
			},
		)

		if err == nil {
			fmt.Println("Setting localstack region: " + TestRegion)
			config.Region = TestRegion
			config.Credentials = GetDefaultLocalstackCredentials()
			config.Endpoints = config.AwsEndpointSet{
				DynamoDB:       "http://localhost:4569",
				Lambda:         "http://localhost:4574",
				S3:             "http://localhost:4572",
				SNS:            "http://localhost:4575",
				SQS:            "http://localhost:4576",
				CloudWatch:     "http://localhost:4582",
				CloudWatchLogs: "http://localhost:4586",
				XRay:           "http://localhost:4603",
				RDS:            "http://localhost:4594",
				SSM:            "http://localhost:4583",
				APIGateway:     "http://localhost:4566",
				EC2:            "http://localhost:4597",
			}

			// Ensuring container is ready to accept requests
			if err = resourcePool.Retry(checkContainerReady); err == nil {
				fmt.Println("Started localstack container ... ")

				err = buildTestingInfrastructure()
			}
		}
	}

	return err
}

// StopContainer stops the localstack container used for testing
func StopContainer() (err error) {
	if resource == nil {
		err = errors.New("Container not started")
	} else {
		fmt.Println("Stopping localstack container ... ")
		// Once tests are done, kill and remove the container
		if err = resourcePool.Retry(func() error {
			err := resourcePool.Purge(resource)
			if err != nil {
				return errors.New("could not stop localstack container")
			}
			return err
		}); err == nil {
			resource = nil
			fmt.Println("Stopped localstack container")
		}
	}

	return err
}

func createGenericLambda() error {
	testCtx, td := services.NewTestDaemon()
	defer td.Close()
	return NewLambda(testCtx, GenericEmptyLambda, "return {}")
}

func buildTestingInfrastructure() (err error) {
	fmt.Println("Initializing base testing infrastructure ... ")

	return createGenericLambda()
}

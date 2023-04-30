package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

type lambdaClient interface {
	Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
}

type newLambdaClientFunc func(queueURL, roleArn, roleSessionName, roleExternalID string) (lambdaClient, error)

// newLambdaClient creates real lambda client.
func newLambdaClient(lambdaArn, roleArn, roleSessionName, roleExternalID string) (lambdaClient, error) {
	region, errRegion := getARNRegion(lambdaArn)
	if errRegion != nil {
		return nil, errRegion
	}

	cfg, _, errConfig := awsConfig(region, roleArn, roleExternalID, roleSessionName)
	if errConfig != nil {
		return nil, errConfig
	}

	client := lambda.NewFromConfig(cfg)

	return client, nil
}

package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type sqsClient interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

type newSqsClientFunc func(queueURL, roleArn, roleSessionName, roleExternalID string) (sqsClient, error)

// newSqsClient creates real sqs client.
func newSqsClient(queueURL, roleArn, roleSessionName, roleExternalID string) (sqsClient, error) {
	region, errRegion := getRegion(queueURL)
	if errRegion != nil {
		return nil, errRegion
	}

	cfg, _, errConfig := awsConfig(region, roleArn, roleExternalID, roleSessionName)
	if errConfig != nil {
		return nil, errConfig
	}

	client := sqs.NewFromConfig(cfg)

	return client, nil
}

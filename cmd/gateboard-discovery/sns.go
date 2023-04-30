package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type snsClient interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type newSnsClientFunc func(queueURL, roleArn, roleSessionName, roleExternalID string) (snsClient, error)

// newSnsClient creates real sns client.
func newSnsClient(topicArn, roleArn, roleSessionName, roleExternalID string) (snsClient, error) {
	region, errRegion := getARNRegion(topicArn)
	if errRegion != nil {
		return nil, errRegion
	}

	cfg, _, errConfig := awsConfig(region, roleArn, roleExternalID, roleSessionName)
	if errConfig != nil {
		return nil, errConfig
	}

	client := sns.NewFromConfig(cfg)

	return client, nil
}

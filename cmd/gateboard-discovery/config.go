package main

import (
	"time"

	"github.com/udhos/gateboard/env"
)

type appConfig struct {
	accountsFile         string
	interval             time.Duration
	gateboardServerURL   string
	debug                bool
	dryRun               bool
	save                 string
	saveRetry            int
	saveRetryInterval    time.Duration
	webhookToken         string
	webhookURL           string
	webhookMethod        string
	queueURL             string
	queueRoleARN         string
	queueRoleExternalID  string
	topicARN             string
	topicRoleARN         string
	topicRoleExternalID  string
	lambdaARN            string
	lambdaRoleARN        string
	lambdaRoleExternalID string
}

func newConfig() appConfig {
	return appConfig{
		accountsFile:         env.String("ACCOUNTS", "discovery-accounts.yaml"),
		interval:             env.Duration("INTERVAL", 0),
		gateboardServerURL:   env.String("GATEBOARD_SERVER_URL", "http://localhost:8080/gateway"),
		debug:                env.Bool("DEBUG", true),
		dryRun:               env.Bool("DRY_RUN", true),
		save:                 env.String("SAVE", "server"), // server, webhook, sqs, sns, lambda
		saveRetry:            env.Int("SAVE_RETRY", 3),
		saveRetryInterval:    env.Duration("SAVE_RETRY_INTERVAL", 1*time.Second),
		webhookToken:         env.String("WEBHOOK_TOKEN", "secret"),
		webhookURL:           env.String("WEBHOOK_URL", ""), // https://xxxxxxxxxxxxxxxx.lambda-url.us-east-1.on.aws/
		webhookMethod:        env.String("WEBHOOK_METHOD", "PUT"),
		queueURL:             env.String("QUEUE_URL", ""), // https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
		queueRoleARN:         env.String("QUEUE_ROLE_ARN", ""),
		queueRoleExternalID:  env.String("QUEUE_ROLE_EXTERNAL_ID", ""),
		topicARN:             env.String("TOPIC_ARN", ""),
		topicRoleARN:         env.String("TOPIC_ROLE_ARN", ""),
		topicRoleExternalID:  env.String("TOPIC_ROLE_EXTERNAL_ID", ""),
		lambdaARN:            env.String("LAMBDA_ARN", ""),
		lambdaRoleARN:        env.String("LAMBDA_ROLE_ARN", ""),
		lambdaRoleExternalID: env.String("LAMBDA_ROLE_EXTERNAL_ID", ""),
	}
}

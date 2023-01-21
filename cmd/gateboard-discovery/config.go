package main

import (
	"time"

	"github.com/udhos/gateboard/env"
)

type appConfig struct {
	accountsFile        string
	interval            time.Duration
	gateboardServerURL  string
	debug               bool
	dryRun              bool
	save                string
	webhookToken        string
	webhookURL          string
	queueURL            string
	queueRoleARN        string
	queueRoleExternalID string
}

func newConfig() appConfig {
	return appConfig{
		accountsFile:        env.String("ACCOUNTS", "discovery-accounts.yaml"),
		interval:            env.Duration("INTERVAL", 0),
		gateboardServerURL:  env.String("GATEBOARD_SERVER_URL", "http://localhost:8080/gateway"),
		debug:               env.Bool("DEBUG", true),
		dryRun:              env.Bool("DRY_RUN", true),
		save:                env.String("SAVE", "server"), // server, webhook, sqs
		webhookToken:        env.String("WEBHOOK_TOKEN", "secret"),
		webhookURL:          env.String("WEBHOOK_URL", ""), // https://xxxxxxxxxxxxxxxx.lambda-url.us-east-1.on.aws/
		queueURL:            env.String("QUEUE_URL", ""),   // https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
		queueRoleARN:        env.String("QUEUE_ROLE_ARN", ""),
		queueRoleExternalID: env.String("QUEUE_ROLE_EXTERNAL_ID", ""),
	}
}

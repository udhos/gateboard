package main

import (
	"time"

	"github.com/udhos/gateboard/env"
)

type appConfig struct {
	accountsFile       string
	interval           time.Duration
	gateboardServerURL string
	debug              bool
	dryRun             bool
	save               string
	webhookToken       string
}

func newConfig() appConfig {
	return appConfig{
		accountsFile:       env.String("ACCOUNTS", "discovery-accounts.yaml"),
		interval:           env.Duration("INTERVAL", 0),
		gateboardServerURL: env.String("GATEBOARD_SERVER_URL", "http://localhost:8080/gateway"),
		debug:              env.Bool("DEBUG", true),
		dryRun:             env.Bool("DRY_RUN", true),
		save:               env.String("SAVE", "server"),
		webhookToken:       env.String("WEBHOOK_TOKEN", "secret"),
	}
}

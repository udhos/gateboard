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
}

func newConfig() appConfig {
	return appConfig{
		accountsFile:       env.String("ACCOUNTS", "discovery-accounts.yaml"),
		interval:           env.Duration("INTERVAL", 1*time.Minute),
		gateboardServerURL: env.String("GATEBOARD_SERVER_URL", "http://localhost:8080/gateway"),
		debug:              env.Bool("DEBUG", true),
		dryRun:             env.Bool("DRY_RUN", true),
	}
}

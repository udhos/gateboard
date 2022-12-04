package main

import "github.com/udhos/gateboard/env"

type appConfig struct {
	queueURL        string
	TTL             int
	repoType        string
	mongoURI        string
	mongoDatabase   string
	mongoCollection string
	sqsRoleARN      string
	applicationAddr string
	healthAddr      string
	healthPath      string
	metricsAddr     string
	metricsPath     string
	jaegerURL       string
}

func newConfig() appConfig {
	return appConfig{
		queueURL:        env.String("QUEUE_URL", ""),
		TTL:             env.Int("TTL", 120),
		repoType:        env.String("REPO", "mongo"),
		mongoURI:        env.String("MONGO_URL", "mongodb://localhost:27017"),
		mongoDatabase:   env.String("MONGO_DATABASE", "gateboard"),
		mongoCollection: env.String("MONGO_COLLECTION", "gateboard"),
		sqsRoleARN:      env.String("ROLE_ARN", ""),
		applicationAddr: env.String("LISTEN_ADDR", ":8080"),
		healthAddr:      env.String("HEALTH_ADDR", ":8888"),
		healthPath:      env.String("HEALTH_PATH", "/health"),
		metricsAddr:     env.String("METRICS_ADDR", ":3000"),
		metricsPath:     env.String("METRICS_PATH", "/metrics"),
		jaegerURL:       env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),
	}
}

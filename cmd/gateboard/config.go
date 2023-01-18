package main

import (
	"time"

	"github.com/udhos/gateboard/env"
)

type appConfig struct {
	debug              bool
	queueURL           string
	sqsRoleARN         string
	TTL                int
	repoType           string
	mongoURI           string
	mongoDatabase      string
	mongoCollection    string
	applicationAddr    string
	healthAddr         string
	healthPath         string
	metricsAddr        string
	metricsPath        string
	jaegerURL          string
	dynamoDBTable      string
	dynamoDBRegion     string
	dynamoDBRoleARN    string
	writeToken         bool
	redisAddr          string
	redisPassword      string
	redisKey           string
	writeRetry         int
	writeRetryInterval time.Duration
}

func newConfig() appConfig {
	return appConfig{
		debug:              env.Bool("DEBUG", true),
		queueURL:           env.String("QUEUE_URL", ""),
		sqsRoleARN:         env.String("SQS_ROLE_ARN", ""),
		TTL:                env.Int("TTL", 120),
		repoType:           env.String("REPO", "mongo"),
		mongoURI:           env.String("MONGO_URL", "mongodb://localhost:27017"),
		mongoDatabase:      env.String("MONGO_DATABASE", "gateboard"),
		mongoCollection:    env.String("MONGO_COLLECTION", "gateboard"),
		applicationAddr:    env.String("LISTEN_ADDR", ":8080"),
		healthAddr:         env.String("HEALTH_ADDR", ":8888"),
		healthPath:         env.String("HEALTH_PATH", "/health"),
		metricsAddr:        env.String("METRICS_ADDR", ":3000"),
		metricsPath:        env.String("METRICS_PATH", "/metrics"),
		jaegerURL:          env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),
		dynamoDBTable:      env.String("DYNAMODB_TABLE", "gateboard"),
		dynamoDBRegion:     env.String("DYNAMODB_REGION", "us-east-1"),
		dynamoDBRoleARN:    env.String("DYNAMODB_ROLE_ARN", ""),
		writeToken:         env.Bool("WRITE_TOKEN", false),
		redisAddr:          env.String("REDIS_ADDR", "localhost:6379"),
		redisPassword:      env.String("REDIS_PASSWORD", ""),
		redisKey:           env.String("REDIS_KEY", "gateboard"),
		writeRetry:         env.Int("WRITE_RETRY", 3),
		writeRetryInterval: env.Duration("WRITE_RETRY_INTERVAL", 1*time.Second),
	}
}

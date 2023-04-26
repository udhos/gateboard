package main

import (
	"time"

	"github.com/udhos/gateboard/gateboard"
)

type appConfig struct {
	debug                  bool
	queueURL               string
	sqsRoleARN             string
	sqsConsumeBadMessage   bool
	sqsConsumeInvalidToken bool
	TTL                    int
	repoType               string
	mongoURI               string
	mongoDatabase          string
	mongoCollection        string
	mongoUsername          string
	mongoPassword          string
	mongoTLSCaFile         string
	mongoMinPool           uint64
	applicationAddr        string
	healthAddr             string
	healthPath             string
	metricsAddr            string
	metricsPath            string
	metricsMaskPath        bool
	metricsNamespace       string
	jaegerURL              string
	dynamoDBTable          string
	dynamoDBRegion         string
	dynamoDBRoleARN        string
	writeToken             bool
	redisAddr              string
	redisPassword          string
	redisKey               string
	writeRetry             int
	writeRetryInterval     time.Duration
	s3BucketName           string
	s3BucketRegion         string
	s3Prefix               string
	s3RoleArn              string
}

func newConfig(roleSessionName string) appConfig {

	env := gateboard.NewEnv(roleSessionName)

	return appConfig{
		debug:                  env.Bool("DEBUG", true),
		queueURL:               env.String("QUEUE_URL", ""),
		sqsRoleARN:             env.String("SQS_ROLE_ARN", ""),
		sqsConsumeBadMessage:   env.Bool("SQS_CONSUME_BAD_MESSAGE", false),
		sqsConsumeInvalidToken: env.Bool("SQS_CONSUME_INVALID_TOKEN", true),
		TTL:                    env.Int("TTL", 300), // seconds
		repoType:               env.String("REPO", "mongo"),
		mongoURI:               env.String("MONGO_URL", "mongodb://localhost:27017"),
		mongoDatabase:          env.String("MONGO_DATABASE", "gateboard"),
		mongoCollection:        env.String("MONGO_COLLECTION", "gateboard"),
		mongoUsername:          env.String("MONGO_USERNAME", ""),
		mongoPassword:          env.String("MONGO_PASSWORD", ""),
		mongoTLSCaFile:         env.String("MONGO_TLS_CA_FILE", ""),
		mongoMinPool:           env.Uint64("MONGO_MIN_POOL", 1),
		applicationAddr:        env.String("LISTEN_ADDR", ":8080"),
		healthAddr:             env.String("HEALTH_ADDR", ":8888"),
		healthPath:             env.String("HEALTH_PATH", "/health"),
		metricsAddr:            env.String("METRICS_ADDR", ":3000"),
		metricsPath:            env.String("METRICS_PATH", "/metrics"),
		metricsMaskPath:        env.Bool("METRICS_MASK_PATH", true),
		metricsNamespace:       env.String("METRICS_NAMESPACE", ""),
		jaegerURL:              env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),
		dynamoDBTable:          env.String("DYNAMODB_TABLE", "gateboard"),
		dynamoDBRegion:         env.String("DYNAMODB_REGION", "us-east-1"),
		dynamoDBRoleARN:        env.String("DYNAMODB_ROLE_ARN", ""),
		writeToken:             env.Bool("WRITE_TOKEN", false),
		redisAddr:              env.String("REDIS_ADDR", "localhost:6379"),
		redisPassword:          env.String("REDIS_PASSWORD", ""),
		redisKey:               env.String("REDIS_KEY", "gateboard"),
		writeRetry:             env.Int("WRITE_RETRY", 3),
		writeRetryInterval:     env.Duration("WRITE_RETRY_INTERVAL", 1*time.Second),
		s3BucketName:           env.String("S3_BUCKET_NAME", ""),
		s3BucketRegion:         env.String("S3_BUCKET_REGION", "us-east-1"),
		s3Prefix:               env.String("S3_PREFIX", "gateboard"),
		s3RoleArn:              env.String("S3_ROLE_ARN", ""),
	}
}

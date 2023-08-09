package main

import (
	"log"
	"os"
	"time"

	"github.com/udhos/gateboard/gateboard"
)

type appConfig struct {
	secretRoleArn             string
	logDriver                 string
	debug                     bool
	queueURL                  string
	sqsRoleARN                string
	sqsConsumeBadMessage      bool
	sqsConsumeInvalidToken    bool
	TTL                       int
	repoList                  string
	applicationAddr           string
	healthAddr                string
	healthPath                string
	metricsAddr               string
	metricsPath               string
	metricsMaskPath           bool
	metricsNamespace          string
	metricsBucketsLatencyHTTP []float64
	metricsBucketsLatencyRepo []float64
	jaegerURL                 string
	writeRetry                int
	writeRetryInterval        time.Duration
	writeToken                bool
	tokens                    string
}

func newConfig(roleSessionName string) appConfig {

	env := gateboard.NewEnv(roleSessionName)

	return appConfig{
		secretRoleArn:             envString("SECRET_ROLE_ARN", ""),
		logDriver:                 env.String("LOG_DRIVER", ""), // anything other than "zap" enables gin default logger
		debug:                     env.Bool("DEBUG", true),
		queueURL:                  env.String("QUEUE_URL", ""),
		sqsRoleARN:                env.String("SQS_ROLE_ARN", ""),
		sqsConsumeBadMessage:      env.Bool("SQS_CONSUME_BAD_MESSAGE", false),
		sqsConsumeInvalidToken:    env.Bool("SQS_CONSUME_INVALID_TOKEN", true),
		TTL:                       env.Int("TTL", 300), // seconds
		repoList:                  env.String("REPO_LIST", "repo.yaml"),
		applicationAddr:           env.String("LISTEN_ADDR", ":8080"),
		healthAddr:                env.String("HEALTH_ADDR", ":8888"),
		healthPath:                env.String("HEALTH_PATH", "/health"),
		metricsAddr:               env.String("METRICS_ADDR", ":3000"),
		metricsPath:               env.String("METRICS_PATH", "/metrics"),
		metricsMaskPath:           env.Bool("METRICS_MASK_PATH", true),
		metricsNamespace:          env.String("METRICS_NAMESPACE", ""),
		metricsBucketsLatencyHTTP: env.Float64Slice("METRICS_BUCKETS_LATENCY_HTTP", []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10}),
		metricsBucketsLatencyRepo: env.Float64Slice("METRICS_BUCKETS_LATENCY_REPO", []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5}),
		jaegerURL:                 env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),
		writeRetry:                env.Int("WRITE_RETRY", 3),
		writeRetryInterval:        env.Duration("WRITE_RETRY_INTERVAL", 1*time.Second),
		writeToken:                env.Bool("WRITE_TOKEN", false), // require write token in PUT payload
		tokens:                    env.String("TOKENS", ""),       // preload write tokens from this file "tokens.yaml"
	}
}

// envString extracts string from env var.
// It returns the provided defaultValue if the env var is empty.
// The string returned is also recorded in logs.
func envString(name string, defaultValue string) string {
	str := os.Getenv(name)
	if str != "" {
		log.Printf("%s=[%s] using %s=%s default=%s", name, str, name, str, defaultValue)
		return str
	}
	log.Printf("%s=[%s] using %s=%s default=%s", name, str, name, defaultValue, defaultValue)
	return defaultValue
}

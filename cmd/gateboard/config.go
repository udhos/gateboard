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
	repoTimeout               time.Duration
	applicationAddr           string
	healthAddr                string
	healthPath                string
	metricsAddr               string
	metricsPath               string
	metricsMaskPath           bool
	metricsNamespace          string
	metricsBucketsLatencyHTTP []float64
	metricsBucketsLatencyRepo []float64
	prometheusEnable          bool
	dogstatsdEnable           bool
	dogstatsdClientTTL        time.Duration
	dogstatsdDebug            bool
	dogstatsdExportInterval   time.Duration
	otelTraceEnable           bool
	writeRetry                int
	writeRetryInterval        time.Duration
	writeToken                bool
	tokens                    string
	groupCache                bool
	groupCachePort            string
	groupCacheExpire          time.Duration
	groupCacheSizeBytes       int64
	kubegroupDebug            bool
	kubegroupLabelSelector    string
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
		repoTimeout:               env.Duration("REPO_TIMEOUT", 15*time.Second),
		applicationAddr:           env.String("LISTEN_ADDR", ":8080"),
		healthAddr:                env.String("HEALTH_ADDR", ":8888"),
		healthPath:                env.String("HEALTH_PATH", "/health"),
		metricsAddr:               env.String("METRICS_ADDR", ":3000"),
		metricsPath:               env.String("METRICS_PATH", "/metrics"),
		metricsMaskPath:           env.Bool("METRICS_MASK_PATH", true),
		metricsNamespace:          env.String("METRICS_NAMESPACE", ""),
		metricsBucketsLatencyHTTP: env.Float64Slice("METRICS_BUCKETS_LATENCY_HTTP", []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10}),
		metricsBucketsLatencyRepo: env.Float64Slice("METRICS_BUCKETS_LATENCY_REPO", []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5}),
		prometheusEnable:          env.Bool("PROMETHEUS_ENABLE", true),
		dogstatsdEnable:           env.Bool("DOGSTATSD_ENABLE", true),
		dogstatsdClientTTL:        env.Duration("DOGSTATSD_CLIENT_TTL", time.Minute),
		dogstatsdDebug:            env.Bool("DOGSTATSD_DEBUG", false),
		dogstatsdExportInterval:   env.Duration("DOGSTATSD_EXPORT_INTERVAL", 20*time.Second),
		otelTraceEnable:           env.Bool("OTEL_TRACE_ENABLE", true),
		writeRetry:                env.Int("WRITE_RETRY", 3),
		writeRetryInterval:        env.Duration("WRITE_RETRY_INTERVAL", 1*time.Second),
		writeToken:                env.Bool("WRITE_TOKEN", false), // require write token in PUT payload
		tokens:                    env.String("TOKENS", ""),       // preload write tokens from this file "tokens.yaml"
		groupCache:                env.Bool("GROUP_CACHE", false),
		groupCachePort:            env.String("GROUP_CACHE_PORT", ":5000"),
		groupCacheExpire:          env.Duration("GROUP_CACHE_EXPIRE", 180*time.Second),
		groupCacheSizeBytes:       env.Int64("GROUP_CACHE_SIZE_BYTES", 10_000),
		kubegroupDebug:            env.Bool("KUBEGROUP_DEBUG", true),
		kubegroupLabelSelector:    env.String("KUBEGROUP_LABEL_SELECTOR", "app=gateboard"),
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

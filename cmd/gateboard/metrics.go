package main

import (
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/udhos/dogstatsdclient/dogstatsdclient"
)

const (
	repoStatusOK       = "success"
	repoStatusError    = "error"
	repoStatusNotFound = "not-found"
)

type metrics struct {
	latencySpring   *prometheus.HistogramVec
	latencyRepo     *prometheus.HistogramVec
	dogstatsdClient *dogstatsdclient.Client
}

func recordRepositoryLatency(method, status, repo string, elapsed time.Duration) {
	if metric == nil {
		return
	}
	if metric.latencyRepo != nil {
		metric.latencyRepo.WithLabelValues(method, status, repo).Observe(elapsed.Seconds())
	}
	if metric.dogstatsdClient != nil {
		tags := []string{"method:" + method, "status:" + status, "repo:" + repo}
		metric.dogstatsdClient.TimeInMilliseconds("repository_requests_milliseconds",
			float64(elapsed.Milliseconds()), tags, 1)
	}
}

var (
	dimensionsSpring     = []string{"method", "status", "uri"}
	dimensionsRepository = []string{"method", "status", "repo"}
	metric               *metrics
)

func initMetrics(namespace string, latencyBucketsHTTP,
	latencyBucketsRepo []float64, prometheusEnable,
	dogstatsdEnable bool) {
	if metric != nil {
		return
	}
	metric = newMetrics(namespace, latencyBucketsHTTP, latencyBucketsRepo,
		prometheusEnable, dogstatsdEnable)
}

func newMetrics(namespace string, latencyBucketsHTTP,
	latencyBucketsRepo []float64, prometheusEnable,
	dogstatsdEnable bool) *metrics {

	m := &metrics{}

	if prometheusEnable {

		m.latencySpring = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_server_requests_seconds",
				Help:      "Spring-like server request duration in seconds.",
				Buckets:   latencyBucketsHTTP,
			},
			dimensionsSpring,
		)

		m.latencyRepo = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "repository_requests_seconds",
				Help:      "Repository request duration in seconds.",
				Buckets:   latencyBucketsRepo,
			},
			dimensionsRepository,
		)
	}

	if dogstatsdEnable {
		client, errClient := dogstatsdclient.New(dogstatsdclient.Options{
			Namespace: "gateboard",
		})
		if errClient != nil {
			log.Fatalf("Dogstatsd client error: %v", errClient)
		}
		m.dogstatsdClient = client
	}

	return m
}

// middleware provides a gin middleware for exposing prometheus metrics.
func middlewareMetrics(metricsMaskPath bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		if metric == nil {
			return
		}

		if metric.latencySpring != nil {

			elap := time.Since(start)

			status := strconv.Itoa(c.Writer.Status())
			elapsedSeconds := float64(elap) / float64(time.Second)

			var path string
			if metricsMaskPath {
				path = c.FullPath() // masked path: GET /gateway/*gateway_name
				// FullPath returns a matched route full path. For not found routes returns an empty string.
				if path == "" {
					path = "NO_ROUTE_HANDLER"
				}
			} else {
				path = c.Request.URL.Path // actual path: GET /gateway/actual_gateway_name
			}

			metric.latencySpring.WithLabelValues(c.Request.Method, status, path).Observe(elapsedSeconds)
		}
	}
}

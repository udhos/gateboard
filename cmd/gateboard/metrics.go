package main

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	repoStatusOK       = "success"
	repoStatusError    = "error"
	repoStatusNotFound = "not-found"
)

type metrics struct {
	latencySpring *prometheus.HistogramVec
	latencyRepo   *prometheus.HistogramVec
}

func recordRepositoryLatency(method, status, repo string, elapsed time.Duration) {
	sec := float64(elapsed) / float64(time.Second)
	metric.latencyRepo.WithLabelValues(method, status, repo).Observe(sec)
}

var (
	dimensionsSpring     = []string{"method", "status", "uri"}
	dimensionsRepository = []string{"method", "status", "repo"}
	metric               *metrics
)

func initMetrics(registerer prometheus.Registerer, namespace string, latencyBucketsHTTP, latencyBucketsRepo []float64) {
	if metric != nil {
		return
	}
	metric = newMetrics(registerer, namespace, latencyBucketsHTTP, latencyBucketsRepo)
}

func newMetrics(registerer prometheus.Registerer, namespace string, latencyBucketsHTTP, latencyBucketsRepo []float64) *metrics {
	return &metrics{

		latencySpring: newHistogramVec(
			registerer,
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_server_requests_seconds",
				Help:      "Spring-like server request duration in seconds.",
				Buckets:   latencyBucketsHTTP,
			},
			dimensionsSpring,
		),

		latencyRepo: newHistogramVec(
			registerer,
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "repository_requests_seconds",
				Help:      "Repository request duration in seconds.",
				Buckets:   latencyBucketsRepo,
			},
			dimensionsRepository,
		),
	}
}

func newHistogramVec(registerer prometheus.Registerer,
	opts prometheus.HistogramOpts,
	labelValues []string) *prometheus.HistogramVec {
	return promauto.With(registerer).NewHistogramVec(opts, labelValues)
}

// middleware provides a gin middleware for exposing prometheus metrics.
func middlewareMetrics(metricsMaskPath bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

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

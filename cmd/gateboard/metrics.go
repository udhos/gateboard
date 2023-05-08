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

func recordRepositoryLatency(method, status string, elapsed time.Duration) {
	sec := float64(elapsed) / float64(time.Second)
	metric.latencyRepo.WithLabelValues(method, status).Observe(sec)
}

var (
	dimensionsSpring     = []string{"method", "status", "uri"}
	dimensionsRepository = []string{"method", "status"}
	metric               *metrics
)

func initMetrics(namespace string) {
	if metric != nil {
		return
	}
	metric = newMetrics(namespace)
}

func newMetrics(namespace string) *metrics {
	return &metrics{

		latencySpring: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_server_requests_seconds",
				Help:      "Spring-like server request duration in seconds.",
				Buckets:   []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10},
			},
			dimensionsSpring,
		),

		latencyRepo: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "repository_requests_seconds",
				Help:      "Repository request duration in seconds.",
				Buckets:   []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
			},
			dimensionsRepository,
		),
	}
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

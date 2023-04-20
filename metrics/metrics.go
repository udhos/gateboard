/*
Package metrics provides utilities for exposing prometheus metrics.
*/
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	dimensionsSpring = []string{"method", "status", "uri"}

	latencySpring = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_server_requests_seconds",
			Help:    "Spring-like request durations in seconds.",
			Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10},
		},
		dimensionsSpring,
	)
)

// Middleware provides a gin middleware for exposing prometheus metrics.
func Middleware(metricsMaskPath bool) gin.HandlerFunc {
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

		latencySpring.WithLabelValues(c.Request.Method, status, path).Observe(elapsedSeconds)
	}
}

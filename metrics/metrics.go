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

	springRequestsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_seconds_count",
		Help: "Total number of requests",
	}, dimensionsSpring)

	springRequestsDuration = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_seconds_sum",
		Help: "Sum of the the duration of every request",
	}, dimensionsSpring)
)

func MetricsMiddleware() gin.HandlerFunc {
	return handlerMetrics
}

func handlerMetrics(c *gin.Context) {
	start := time.Now()

	c.Next()

	elap := time.Since(start)

	status := strconv.Itoa(c.Writer.Status())
	elapsedSeconds := float64(elap) / float64(time.Second)
	//path := c.FullPath()
	path := c.Request.URL.Path

	springRequestsCount.WithLabelValues(c.Request.Method, status, path).Inc()
	springRequestsDuration.WithLabelValues(c.Request.Method, status, path).Add(elapsedSeconds)
}

// Package zlog provides logging services.
package zlog

import (
	"log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger exposes the zap logger.
var Logger = initLogger()

func initLogger() *zap.Logger {
	logConfig := zap.NewProductionConfig()

	logConfig.Encoding = "json"

	logConfig.EncoderConfig = zapcore.EncoderConfig{
		LevelKey:     "level",
		TimeKey:      "zap_time", // gin logs a "time" field already
		MessageKey:   "message",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}

	l, err := logConfig.Build()
	if err != nil {
		log.Fatalf("initLogger: %v", err)
	}

	return l
}

// GinContext provides a log context for ginzap log middleware.
func GinContext(c *gin.Context) []zapcore.Field {
	fields := []zapcore.Field{}

	// log request ID
	/*
		if requestID := c.Writer.Header().Get("X-Request-Id"); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
	*/

	// log trace and span ID
	if trace.SpanFromContext(c.Request.Context()).SpanContext().IsValid() {
		fields = append(fields, zap.String("traceId", trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()))
		//fields = append(fields, zap.String("span_id", trace.SpanFromContext(c.Request.Context()).SpanContext().SpanID().String()))
	}

	// log request body
	/*
		var body []byte
		var buf bytes.Buffer
		tee := io.TeeReader(c.Request.Body, &buf)
		body, _ = io.ReadAll(tee)
		c.Request.Body = io.NopCloser(&buf)
		fields = append(fields, zap.String("body", string(body)))
	*/

	return fields
}

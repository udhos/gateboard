// Package zlog provides logging services.
package zlog

import (
	"context"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger = initLogger()   // Logger exposes the zap logger
var LoggerConfig zap.Config // LoggerConfig can be used to SetLevel: LoggerConfig.Level.SetLevel(zap.DebugLevel)

func initLogger() *zap.Logger {
	LoggerConfig = zap.NewProductionConfig()

	LoggerConfig.Encoding = "json"
	LoggerConfig.EncoderConfig = zapcore.EncoderConfig{
		LevelKey:     "level",
		TimeKey:      "zap_time", // gin logs a "time" field already
		MessageKey:   "message",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}

	l, err := LoggerConfig.Build()
	if err != nil {
		log.Fatalf("initLogger: %v", err)
	}

	return l
}

// GinzapFields provides fields for ginzap log middleware.
func GinzapFields(c *gin.Context) []zapcore.Field {
	fields := []zapcore.Field{}

	// log request ID
	/*
		if requestID := c.Writer.Header().Get("X-Request-Id"); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
	*/

	// log trace and span ID
	if spanCtx := trace.SpanFromContext(c.Request.Context()).SpanContext(); spanCtx.IsValid() {
		fields = append(fields, zap.String("traceId", spanCtx.TraceID().String()))
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

// Debugf logs at debug level with Printf-like formatting.
func Debugf(debug bool, format string, v ...any) {
	if debug {
		Logger.Debug(fmt.Sprintf(format, v...))
	}
}

// Infof logs at info level with Printf-like formatting.
func Infof(format string, v ...any) {
	Logger.Info(fmt.Sprintf(format, v...))
}

// Errorf logs at error level with Printf-like formatting.
func Errorf(format string, v ...any) {
	Logger.Error(fmt.Sprintf(format, v...))
}

// Fatalf logs at fatal level with Printf-like formatting, then exits.
func Fatalf(format string, v ...any) {
	Logger.Fatal(fmt.Sprintf(format, v...))
}

// CtxDebugf logs at debug level with trace context and Printf-like formatting.
func CtxDebugf(ctx context.Context, debug bool, format string, v ...any) {
	if debug {
		Logger.Debug(fmt.Sprintf(format, v...), zap.String("traceId", traceIDFromContext(ctx)))
	}
}

// CtxInfof logs at info level with trace context and Printf-like formatting.
func CtxInfof(ctx context.Context, format string, v ...any) {
	Logger.Info(fmt.Sprintf(format, v...), zap.String("traceId", traceIDFromContext(ctx)))
}

// CtxErrorf logs at error level with trace context and Printf-like formatting.
func CtxErrorf(ctx context.Context, format string, v ...any) {
	Logger.Error(fmt.Sprintf(format, v...), zap.String("traceId", traceIDFromContext(ctx)))
}

// GinInfof logs at info level with gin trace context and Printf-like formatting.
func GinInfof(c *gin.Context, format string, v ...any) {
	CtxInfof(c.Request.Context(), fmt.Sprintf(format, v...))
}

// GinErrorf logs at error level with gin trace context and Printf-like formatting.
func GinErrorf(c *gin.Context, format string, v ...any) {
	CtxErrorf(c.Request.Context(), fmt.Sprintf(format, v...))
}

func traceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return "<nil-span>"
	}
	return span.SpanContext().TraceID().String()
}

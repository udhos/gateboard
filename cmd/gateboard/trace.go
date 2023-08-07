package main

import (
	"context"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newSpanGin(c *gin.Context, caller string, tracer trace.Tracer) (context.Context, trace.Span) {
	ctx := c.Request.Context()
	return newSpan(ctx, caller, tracer)
}

func newSpan(ctx context.Context, caller string, tracer trace.Tracer) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, nil
	}
	newCtx, span := tracer.Start(ctx, caller)
	return newCtx, span
}

func getTraceID(span trace.Span) string {
	if span == nil {
		return "tracing-disabled"
	}
	return span.SpanContext().TraceID().String()
}

func traceError(span trace.Span, description string) {
	if span != nil {
		span.SetStatus(codes.Error, description)
	}
}

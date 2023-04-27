package main

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newSpan(ctx context.Context, caller string, tracer trace.Tracer) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, nil
	}
	newCtx, span := tracer.Start(ctx, caller)
	return newCtx, span
}

func traceError(span trace.Span, description string) {
	if span != nil {
		span.SetStatus(codes.Error, description)
	}
}

func httpClient() http.Client {
	return http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
}

package gateboard

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

func newSpan(ctx context.Context, caller string, tracer trace.Tracer) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, nil
	}
	newCtx, span := tracer.Start(ctx, caller)
	return newCtx, span
}

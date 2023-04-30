package main

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type scanner interface {
	list(ctx context.Context, tracer trace.Tracer) []item
}

type item struct {
	name string
	id   string
}

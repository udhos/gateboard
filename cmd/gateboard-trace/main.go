// Package main implements the program.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/udhos/otelconfig/oteltrace"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	me := filepath.Base(os.Args[0])

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tr, cancel, errTracer := oteltrace.TraceStart(options)

		if errTracer != nil {
			log.Fatalf("tracer: %v", errTracer)
		}

		defer cancel()

		tracer = tr
	}

	for i := 0; i < 5; i++ {
		work(context.TODO(), tracer)
	}
}

func work(ctx context.Context, tracer trace.Tracer) {
	_, span := tracer.Start(ctx, "work")
	defer span.End()
	log.Printf("work: working")
	time.Sleep(500 * time.Millisecond)
}

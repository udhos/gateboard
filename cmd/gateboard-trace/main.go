// Package main implements the program.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/udhos/gateboard/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	jaegerURL := "http://localhost:14268/api/traces"

	me := filepath.Base(os.Args[0])

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		tp, errTracer := tracing.TracerProvider(me, jaegerURL)
		if errTracer != nil {
			log.Fatal(errTracer)
		}

		// Register our TracerProvider as the global so any imported
		// instrumentation in the future will default to using it.
		otel.SetTracerProvider(tp)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Cleanly shutdown and flush telemetry when the application exits.
		defer func(ctx context.Context) {
			log.Printf("shutting down trace provider")
			// Do not make the application hang when it is shutdown.
			ctx, cancel = context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Print(err)
			}
		}(ctx)

		tracing.TracePropagation()

		tracer = tp.Tracer(fmt.Sprintf("%s-main", me))
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

/*
This is the main package for the example client.
*/
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/udhos/gateboard/gateboard"
	"github.com/udhos/gateboard/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

const version = "0.0.0"

const tryAgain = http.StatusServiceUnavailable
const internalError = http.StatusInternalServerError

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
}

type application struct {
	jaegerURL string
	tracer    trace.Tracer
	client    *gateboard.Client
}

func main() {

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	{
		v := getVersion(me)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	env := gateboard.NewEnv(me)

	app := &application{
		jaegerURL: env.String("JAEGER_URL", "http://localhost:14268/api/traces"),
	}

	//
	// initialize tracing
	//

	{
		tp, errTracer := tracing.TracerProvider(me, app.jaegerURL)
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
			// Do not make the application hang when it is shutdown.
			ctx, cancel = context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Fatal(err)
			}
		}(ctx)

		tracing.TracePropagation()

		app.tracer = tp.Tracer(fmt.Sprintf("%s-main", me))
	}

	app.client = gateboard.NewClient(gateboard.ClientOptions{
		ServerURL: env.String("MAIN_URL", "http://localhost:8080/gateway"),
		Debug:     env.Bool("DEBUG", true),
		Tracer:    app.tracer,
	})

	log.Printf("reading gateway name from stdin...")
	for {
		reader := bufio.NewReader(os.Stdin)
		txt, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("stdin: %v", err)
			break
		}
		gatewayName := strings.TrimSpace(txt)
		if gatewayName == "" {
			log.Printf("ignoring empty gateway name")
			continue
		}
		status, body := invokeBackend(app, gatewayName)
		log.Printf("RESULT for incomingCall: gateway_name=%s status=%d body:%s",
			gatewayName, status, body)
		fmt.Println("------------------------------")
	}
}

// invokeBackend implements Recommended Usage from
// https://pkg.go.dev/github.com/udhos/gateboard@main/gateboard#hdr-Recommended_Usage
func invokeBackend(app *application, gatewayName string) (int, string) {
	const me = "invokeBackend"

	ctx, span := newSpan(context.TODO(), me, app.tracer)
	if span != nil {
		defer span.End()
	}

	gatewayID := app.client.GatewayID(ctx, gatewayName)
	if gatewayID == "" {
		log.Printf("%s: GatewayID: gateway_name=%s starting Refresh() async update",
			me, gatewayName)
		return tryAgain, "missing gateway_id"
	}

	log.Printf("%s: mockAwsApiGatewayCall: gateway_name=%s gateway_id=%s",
		me, gatewayName, gatewayID)

	status, body := mockAwsAPIGatewayCall(ctx, app.tracer, gatewayID)
	if status == 403 {
		msg := fmt.Sprintf("%s: mockAwsApiGatewayCall: gateway_name=%s gateway_id=%s status=%d body:%v - starting Refresh() async update",
			me, gatewayName, gatewayID, status, body)
		log.Print(msg)
		traceError(span, msg)
		app.client.Refresh(ctx, gatewayName) // async update
		return tryAgain, "refreshing gateway_id"
	}

	return status, body
}

func mockAwsAPIGatewayCall(ctx context.Context, tracer trace.Tracer, gatewayID string) (int, string) {
	const me = "mockAwsAPIGatewayCall"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	filename := "samples/http_mock.yaml"
	data, errFile := os.ReadFile(filename)
	if errFile != nil {
		msg := fmt.Sprintf("%s: %s: file error: %v", me, filename, errFile)
		log.Print(msg)
		traceError(span, msg)
		return internalError, "bad file"
	}

	type response struct {
		Code int    `yaml:"code"`
		Body string `yaml:"body"`
	}

	table := map[string]response{}

	errYaml := yaml.Unmarshal(data, &table)
	if errYaml != nil {
		msg := fmt.Sprintf("%s: %s: yaml error: %v", me, filename, errYaml)
		log.Print(msg)
		traceError(span, msg)
		return internalError, "bad file yaml"
	}

	//log.Printf("%s: loaded %s: %s", me, filename, string(data))

	r, found := table[gatewayID]
	if found {
		if r.Code != 200 {
			msg := fmt.Sprintf("%s: %s: mock: status:%d body:%s", me, filename, r.Code, r.Body)
			log.Print(msg)
			traceError(span, msg)
		}
		return r.Code, r.Body
	}

	msg := fmt.Sprintf("%s: %s: id not found: %s", me, filename, gatewayID)
	log.Print(msg)
	traceError(span, msg)

	return internalError, "missing gateway id from file"
}

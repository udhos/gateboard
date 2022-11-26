package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/udhos/gateboard/env"
	"github.com/udhos/gateboard/metrics"
	"github.com/udhos/gateboard/tracing"
)

const version = "0.0.0"

type application struct {
	serverMain    *serverGin
	serverHealth  *serverGin
	serverMetrics *serverGin
	me            string
	tracer        trace.Tracer
	repo          repository
	sqsClient     clientConfig
}

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
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

	queueURL := env.String("QUEUE_URL", "")

	app := &application{
		me: me,
	}

	//
	// pick repo type
	//

	const debug = true

	{
		repoType := env.String("REPO", "mongo")
		switch repoType {
		case "mongo":
			var errMongo error
			app.repo, errMongo = newRepoMongo(repoMongoOptions{
				debug:      debug,
				URI:        env.String("MONGO_URL", "mongodb://localhost:27017"),
				database:   "gateboard",
				collection: env.String("MONGO_COLLECTION", "gateboard"),
				timeout:    time.Second * 10,
			})
			if errMongo != nil {
				log.Fatalf("repo mongo: %v", errMongo)
			}
		case "mem":
			app.repo = newRepoMem()
		default:
			log.Fatalf("unsuppported repo type: %s (supported types: mongo, mem)", repoType)
		}
	}

	//
	// sqs listener
	//

	if queueURL != "" {
		app.sqsClient = initClient("main", queueURL, env.String("ROLE_ARN", ""), me)
		go sqsListener(app)
	}

	applicationAddr := env.String("LISTEN_ADDR", ":8080")
	healthAddr := env.String("HEALTH_ADDR", ":8888")
	healthPath := env.String("HEALTH_PATH", "/health")
	metricsAddr := env.String("METRICS_ADDR", ":3000")
	metricsPath := env.String("METRICS_PATH", "/metrics")
	jaegerURL := env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces")

	//
	// initialize tracing
	//

	{
		tp, errTracer := tracing.TracerProvider(app.me, jaegerURL)
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

		app.tracer = tp.Tracer("component-main")
	}

	//
	// register application routes
	//

	app.serverMain = newServerGin(applicationAddr)
	app.serverMain.router.Use(metrics.MetricsMiddleware())
	app.serverMain.router.Use(gin.Logger())
	app.serverMain.router.Use(otelgin.Middleware(app.me))

	const pathGateway = "/gateway/:gateway_name"
	log.Printf("registering route: %s %s", applicationAddr, pathGateway)
	app.serverMain.router.GET(pathGateway, func(c *gin.Context) { gatewayGet(c, app) })
	app.serverMain.router.PUT(pathGateway, func(c *gin.Context) { gatewayPut(c, app) })

	//
	// start application server
	//

	go func() {
		log.Printf("application server: listening on %s", applicationAddr)
		err := app.serverMain.server.ListenAndServe()
		log.Printf("application server: exited: %v", err)
	}()

	//
	// start health server
	//

	app.serverHealth = newServerGin(healthAddr)

	log.Printf("registering route: %s %s", healthAddr, healthPath)
	app.serverHealth.router.GET(healthPath, func(c *gin.Context) {
		c.String(http.StatusOK, "health ok")
	})

	go func() {
		log.Printf("health server: listening on %s", healthAddr)
		err := app.serverHealth.server.ListenAndServe()
		log.Printf("health server: exited: %v", err)
	}()

	//
	// start metrics server
	//

	app.serverMetrics = newServerGin(metricsAddr)

	go func() {
		prom := promhttp.Handler()
		app.serverMetrics.router.GET(metricsPath, func(c *gin.Context) {
			prom.ServeHTTP(c.Writer, c.Request)
		})
		log.Printf("metrics server: listening on %s %s", metricsAddr, metricsPath)
		err := app.serverMetrics.server.ListenAndServe()
		log.Printf("metrics server: exited: %v", err)
	}()

	//
	// handle graceful shutdown
	//

	shutdown(app)
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second
	app.serverHealth.shutdown(timeout)
	app.serverMetrics.shutdown(timeout)
	app.serverMain.shutdown(timeout)

	log.Print("exiting")
}

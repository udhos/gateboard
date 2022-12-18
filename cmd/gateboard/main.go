/*
This is the main package for gateboard service.
*/
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

	"github.com/udhos/gateboard/metrics"
	"github.com/udhos/gateboard/tracing"
)

const version = "0.0.6"

type application struct {
	serverMain    *serverGin
	serverHealth  *serverGin
	serverMetrics *serverGin
	me            string
	tracer        trace.Tracer
	repo          repository
	sqsClient     queue
	config        appConfig
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

	app := &application{
		me:     me,
		config: newConfig(),
	}

	queueURL := app.config.queueURL

	//
	// pick repo type
	//

	{
		switch app.config.repoType {
		case "mongo":
			repo, errMongo := newRepoMongo(repoMongoOptions{
				debug:      app.config.debug,
				URI:        app.config.mongoURI,
				database:   app.config.mongoDatabase,
				collection: app.config.mongoCollection,
				timeout:    time.Second * 10,
			})
			if errMongo != nil {
				log.Fatalf("repo mongo: %v", errMongo)
			}
			app.repo = repo
		case "dynamodb":
			repo, errDynamo := newRepoDynamo(repoDynamoOptions{
				debug:       app.config.debug,
				table:       app.config.dynamoDBTable,
				region:      app.config.dynamoDBRegion,
				roleArn:     app.config.dynamoDBRoleARN,
				sessionName: me,
			})
			if errDynamo != nil {
				log.Fatalf("repo dynamodb: %v", errDynamo)
			}
			app.repo = repo
		case "redis":
			repo, errRedis := newRepoRedis(repoRedisOptions{
				debug:    app.config.debug,
				addr:     app.config.redisAddr,
				password: app.config.redisPassword,
				key:      app.config.redisKey,
			})
			if errRedis != nil {
				log.Fatalf("repo redis: %v", errRedis)
			}
			app.repo = repo
		case "mem":
			app.repo = newRepoMem()
		default:
			log.Fatalf("unsuppported repo type: %s (supported types: mongo, dynamodb, mem)", app.config.repoType)
		}
	}

	//
	// sqs listener
	//

	if queueURL != "" {
		app.sqsClient = initClient("main", queueURL, app.config.sqsRoleARN, me)
		go sqsListener(app)
	}

	//
	// initialize tracing
	//

	{
		tp, errTracer := tracing.TracerProvider(app.me, app.config.jaegerURL)
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
	// init application
	//
	initApplication(app, app.config.applicationAddr)

	//
	// start application server
	//

	go func() {
		log.Printf("application server: listening on %s", app.config.applicationAddr)
		err := app.serverMain.server.ListenAndServe()
		log.Printf("application server: exited: %v", err)
	}()

	//
	// start health server
	//

	app.serverHealth = newServerGin(app.config.healthAddr)

	log.Printf("registering route: %s %s", app.config.healthAddr, app.config.healthPath)
	app.serverHealth.router.GET(app.config.healthPath, func(c *gin.Context) {
		c.String(http.StatusOK, "health ok")
	})

	go func() {
		log.Printf("health server: listening on %s", app.config.healthAddr)
		err := app.serverHealth.server.ListenAndServe()
		log.Printf("health server: exited: %v", err)
	}()

	//
	// start metrics server
	//

	app.serverMetrics = newServerGin(app.config.metricsAddr)

	go func() {
		prom := promhttp.Handler()
		app.serverMetrics.router.GET(app.config.metricsPath, func(c *gin.Context) {
			prom.ServeHTTP(c.Writer, c.Request)
		})
		log.Printf("metrics server: listening on %s %s", app.config.metricsAddr, app.config.metricsPath)
		err := app.serverMetrics.server.ListenAndServe()
		log.Printf("metrics server: exited: %v", err)
	}()

	//
	// handle graceful shutdown
	//

	shutdown(app)
}

func initApplication(app *application, addr string) {
	//
	// register application routes
	//

	app.serverMain = newServerGin(addr)
	app.serverMain.router.Use(metrics.Middleware())
	app.serverMain.router.Use(gin.Logger())
	app.serverMain.router.Use(otelgin.Middleware(app.me))

	const pathGateway = "/gateway/*gateway_name"
	log.Printf("registering route: %s %s", addr, pathGateway)
	app.serverMain.router.GET(pathGateway, func(c *gin.Context) { gatewayGet(c, app) })
	app.serverMain.router.PUT(pathGateway, func(c *gin.Context) { gatewayPut(c, app) })
	app.serverMain.router.GET("/dump", func(c *gin.Context) { gatewayDump(c, app) })
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

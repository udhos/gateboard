/*
This is the main package for gateboard service.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"

	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/tracing"
)

const version = "0.10.0"

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

func main() {

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	app := &application{
		me:     me,
		config: newConfig(me),
	}

	zlog.Init(app.config.debug)

	queueURL := app.config.queueURL

	//
	// pick repo type
	//

	app.repo = pickRepo(me, app.config)

	//
	// preload write tokens
	//

	if app.config.tokens != "" {
		tokens, errTokens := loadTokens(app.config.tokens)
		if errTokens != nil {
			log.Fatalf("error loading tokens from file %s: %v", app.config.tokens, errTokens)
		}
		for gw, id := range tokens {
			errPut := app.repo.putToken(gw, id)
			if errPut != nil {
				log.Fatalf("error preloading token for gateway '%s' into repo: %v",
					gw, errPut)
			}
		}
		log.Printf("preloaded %d tokens from file: %s", len(tokens), app.config.tokens)
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

		app.tracer = tp.Tracer(fmt.Sprintf("%s-main", me))
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

func pickRepo(sessionName string, config appConfig) repository {

	switch config.repoType {
	case "mongo":
		repo, errMongo := newRepoMongo(repoMongoOptions{
			debug:      config.debug,
			URI:        config.mongoURI,
			database:   config.mongoDatabase,
			collection: config.mongoCollection,
			username:   config.mongoUsername,
			password:   config.mongoPassword,
			tlsCAFile:  config.mongoTLSCaFile,
			minPool:    config.mongoMinPool,
			timeout:    time.Second * 10,
		})
		if errMongo != nil {
			log.Fatalf("repo mongo: %v", errMongo)
		}
		return repo
	case "dynamodb":
		repo, errDynamo := newRepoDynamo(repoDynamoOptions{
			debug:       config.debug,
			table:       config.dynamoDBTable,
			region:      config.dynamoDBRegion,
			roleArn:     config.dynamoDBRoleARN,
			sessionName: sessionName,
		})
		if errDynamo != nil {
			log.Fatalf("repo dynamodb: %v", errDynamo)
		}
		return repo
	case "redis":
		repo, errRedis := newRepoRedis(repoRedisOptions{
			debug:    config.debug,
			addr:     config.redisAddr,
			password: config.redisPassword,
			key:      config.redisKey,
		})
		if errRedis != nil {
			log.Fatalf("repo redis: %v", errRedis)
		}
		return repo
	case "mem":
		return newRepoMem()
	case "s3":
		repo, errS3 := newRepoS3(repoS3Options{
			debug:       config.debug,
			bucket:      config.s3BucketName,
			region:      config.s3BucketRegion,
			prefix:      config.s3Prefix,
			roleArn:     config.s3RoleArn,
			sessionName: sessionName,
		})
		if errS3 != nil {
			log.Fatalf("repo s3: %v", errS3)
		}
		return repo
	}

	log.Fatalf("unsuppported repo type: %s (supported types: mongo, dynamodb, mem)", config.repoType)

	return nil
}

func initApplication(app *application, addr string) {

	initMetrics(app.config.metricsNamespace)

	//
	// register application routes
	//

	app.serverMain = newServerGin(addr)
	app.serverMain.router.Use(middlewareMetrics(app.config.metricsMaskPath))
	app.serverMain.router.Use(otelgin.Middleware(app.me))

	// anything other than "zap" enables gin default logger
	if app.config.logDriver == "zap" {
		app.serverMain.router.Use(ginzap.GinzapWithConfig(zlog.Logger, &ginzap.Config{
			UTC:        true,
			TimeFormat: time.RFC3339,
			Context:    zlog.GinzapFields,
		}))
	} else {
		app.serverMain.router.Use(gin.Logger())
	}

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

func loadTokens(input string) (map[string]string, error) {

	const me = "loadTokens"

	reader, errOpen := os.Open(input)
	if errOpen != nil {
		return nil, fmt.Errorf("%s: open file: %s: %v", me, input, errOpen)
	}

	buf, errRead := io.ReadAll(reader)
	if errRead != nil {
		return nil, fmt.Errorf("%s: read file: %s: %v", me, input, errRead)
	}

	tokens := map[string]string{}

	errYaml := yaml.Unmarshal(buf, tokens)
	if errYaml != nil {
		return tokens, fmt.Errorf("%s: parse yaml: %s: %v", me, input, errYaml)
	}

	return tokens, nil
}

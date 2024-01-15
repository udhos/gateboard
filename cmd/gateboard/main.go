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
	"github.com/mailgun/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/otelconfig/oteltrace"
)

type application struct {
	serverMain       *serverGin
	serverHealth     *serverGin
	serverMetrics    *http.Server
	serverGroupCache *http.Server
	cache            *groupcache.Group
	me               string
	tracer           trace.Tracer
	sqsClient        queue
	config           appConfig
	repoConf         []repoConfig
	repoList         []repository
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

	if app.config.debug {
		zlog.LoggerConfig.Level.SetLevel(zap.DebugLevel)
	}

	queueURL := app.config.queueURL

	//
	// preload write tokens
	//

	if app.config.tokens != "" {
		tokens, errTokens := loadTokens(app.config.tokens)
		if errTokens != nil {
			zlog.Fatalf("error loading tokens from file %s: %v", app.config.tokens, errTokens)
		}
		for gw, tk := range tokens {
			errPut := repoPutTokenMultiple(context.TODO(), app, gw, tk)
			if errPut != nil {
				zlog.Fatalf("error preloading token for gateway '%s' into repo: %v",
					gw, errPut)
			}
		}
		zlog.Infof("preloaded %d tokens from file: %s", len(tokens), app.config.tokens)
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
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tracer, cancel, errTracer := oteltrace.TraceStart(options)

		if errTracer != nil {
			log.Fatalf("tracer: %v", errTracer)
		}

		defer cancel()

		app.tracer = tracer
	}

	//
	// init application
	//
	initApplication(app, app.config.applicationAddr)

	//
	// start application server
	//

	go func() {
		zlog.Infof("application server: listening on %s", app.config.applicationAddr)
		err := app.serverMain.server.ListenAndServe()
		zlog.Infof("application server: exited: %v", err)
	}()

	//
	// start health server
	//

	app.serverHealth = newServerGin(app.config.healthAddr)

	zlog.Infof("registering route: %s %s", app.config.healthAddr, app.config.healthPath)
	app.serverHealth.router.GET(app.config.healthPath, func(c *gin.Context) {
		c.String(http.StatusOK, "health ok")
	})

	go func() {
		zlog.Infof("health server: listening on %s", app.config.healthAddr)
		err := app.serverHealth.server.ListenAndServe()
		zlog.Infof("health server: exited: %v", err)
	}()

	//
	// start metrics server
	//

	{
		mux := http.NewServeMux()
		app.serverMetrics = &http.Server{Addr: app.config.metricsAddr, Handler: mux}
		mux.Handle(app.config.metricsPath, promhttp.Handler())

		go func() {
			zlog.Infof("metrics server: listening on %s %s", app.config.metricsAddr, app.config.metricsPath)
			err := app.serverMetrics.ListenAndServe()
			zlog.Infof("metrics server: exited: %v", err)
		}()
	}

	//
	// handle graceful shutdown
	//

	shutdown(app)
}

func initApplication(app *application, addr string) {

	const me = "initApplication"

	initMetrics(app.config.metricsNamespace, app.config.metricsBucketsLatencyHTTP, app.config.metricsBucketsLatencyRepo)

	//
	// load multirepo config
	//

	{
		repoList, errRepo := loadRepoConf(app.config.repoList)
		if errRepo != nil {
			zlog.Fatalf("load repo list: error: %s: %v", app.config.repoList, errRepo)
		}
		if len(repoList) < 1 {
			zlog.Fatalf("load repo list: empty: %s", app.config.repoList)
		}
		app.repoConf = repoList
	}

	log.Printf("repo list: %s: %s", app.config.repoList, toJSON(context.TODO(), app.repoConf))

	for i, conf := range app.repoConf {
		log.Printf("initializing repository: [%d/%d]: %s", i+1, len(app.repoConf), conf.Kind)
		r := createRepo(me, app.config.secretRoleArn, conf, app.config.debug)
		app.repoList = append(app.repoList, r)
	}

	//
	// start group cache
	//

	if app.config.groupCache {
		startGroupcache(app)
	}

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
	zlog.Infof("registering route: %s %s", addr, pathGateway)
	app.serverMain.router.GET(pathGateway, func(c *gin.Context) { gatewayGet(c, app) })
	app.serverMain.router.PUT(pathGateway, func(c *gin.Context) { gatewayPut(c, app) })
	app.serverMain.router.GET("/dump", func(c *gin.Context) { gatewayDump(c, app) })
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	zlog.Infof("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second
	app.serverHealth.shutdown("health", timeout)
	app.serverMain.shutdown("main", timeout)
	httpShutdown(app.serverGroupCache, "groupCache", timeout)
	httpShutdown(app.serverMetrics, "metrics", timeout)

	zlog.Infof("exiting")
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

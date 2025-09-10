package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/udhos/groupcache_datadog/exporter"
	"github.com/udhos/groupcache_exporter"
	"github.com/udhos/groupcache_exporter/groupcache/modernprogram"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) func() {

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.config.groupCachePort)
	if errURL != nil {
		log.Fatalf("my URL: %v", errURL)
	}
	log.Printf("groupcache my URL: %s", myURL)

	workspace := groupcache.NewWorkspace()

	pool := groupcache.NewHTTPPoolOptsWithWorkspace(workspace, myURL,
		&groupcache.HTTPPoolOptions{})

	//
	// start groupcache server
	//

	app.serverGroupCache = &http.Server{Addr: app.config.groupCachePort, Handler: pool}

	go func() {
		log.Printf("groupcache server: listening on %s", app.config.groupCachePort)
		err := app.serverGroupCache.ListenAndServe()
		log.Printf("groupcache server: exited: %v", err)
	}()

	//
	// start watcher for addresses of peers
	//

	clientsetOpt := kubeclient.Options{DebugLog: app.config.kubegroupDebug}
	clientset, errClientset := kubeclient.New(clientsetOpt)
	if errClientset != nil {
		log.Fatalf("startGroupcache: kubeclient: %v", errClientset)
	}

	options := kubegroup.Options{
		Client:           clientset,
		LabelSelector:    app.config.kubegroupLabelSelector,
		Pool:             pool,
		GroupCachePort:   app.config.groupCachePort,
		MetricsNamespace: "",
		Debug:            app.config.kubegroupDebug,
		//MetricsRegisterer: prometheus.DefaultRegisterer, // see below
		//MetricsGatherer:   prometheus.DefaultGatherer, // see below
	}
	if app.config.prometheusEnable {
		options.MetricsRegisterer = prometheus.DefaultRegisterer
	}
	if app.config.dogstatsdEnable {
		options.DogstatsdClient = app.dogstatsdClientGroupcache
	}

	kg, errKg := kubegroup.UpdatePeers(options)
	if errKg != nil {
		log.Fatalf("kubegroup: %v", errKg)
	}
	stopDisc := func() {
		kg.Close()
	}

	//
	// create cache
	//

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage

	getter := groupcache.GetterFunc(
		func(ctx context.Context, gatewayName string, dest groupcache.Sink,
			_ *groupcache.Info) error {

			out, _, errID := repoGetMultiple(ctx, app, gatewayName)
			if errID != nil {
				return errID
			}

			var expire time.Time // zero value for expire means no expiration
			if app.config.groupCacheExpire != 0 {
				expire = time.Now().Add(app.config.groupCacheExpire)
			}

			return dest.SetString(out.GatewayID, expire)
		})

	cacheOptions := groupcache.Options{
		Workspace:       workspace,
		Name:            "gateways",
		CacheBytesLimit: app.config.groupCacheSizeBytes,
		Getter:          getter,
	}

	app.cache = groupcache.NewGroupWithWorkspace(cacheOptions)

	//
	// expose prometheus metrics for groupcache
	//

	listGroups := func() []groupcache_exporter.GroupStatistics {
		return modernprogram.ListGroups(workspace)
	}

	unregister := func() {}

	if app.config.prometheusEnable {
		log.Printf("starting groupcache metrics exporter for Prometheus")
		labels := map[string]string{
			//"app": appName,
		}
		namespace := ""
		collector := groupcache_exporter.NewExporter(groupcache_exporter.Options{
			Namespace:  namespace,
			Labels:     labels,
			ListGroups: listGroups,
		})
		prometheus.MustRegister(collector)
		unregister = func() { prometheus.Unregister(collector) }
	}

	closeExporterDogstatsd := func() {}

	if app.config.dogstatsdEnable {
		log.Printf("starting groupcache metrics exporter for Dogstatsd")
		exporter := exporter.New(exporter.Options{
			Client:         app.dogstatsdClientGroupcache,
			ListGroups:     listGroups,
			ExportInterval: app.config.dogstatsdExportInterval,
		})
		closeExporterDogstatsd = func() { exporter.Close() }
	}

	return func() {
		stopDisc()
		unregister()
		closeExporterDogstatsd()
	}
}

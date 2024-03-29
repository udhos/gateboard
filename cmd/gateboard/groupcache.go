package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mailgun/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/udhos/groupcache_exporter"
	"github.com/udhos/groupcache_exporter/groupcache/mailgun"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.config.groupCachePort)
	if errURL != nil {
		log.Fatalf("my URL: %v", errURL)
	}
	log.Printf("groupcache my URL: %s", myURL)

	pool := groupcache.NewHTTPPoolOpts(myURL, &groupcache.HTTPPoolOptions{})

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
		Client:            clientset,
		LabelSelector:     app.config.kubegroupLabelSelector,
		Pool:              pool,
		GroupCachePort:    app.config.groupCachePort,
		MetricsRegisterer: prometheus.DefaultRegisterer,
		MetricsGatherer:   prometheus.DefaultGatherer,
		MetricsNamespace:  "",
		Debug:             app.config.kubegroupDebug,
	}

	if _, errKg := kubegroup.UpdatePeers(options); errKg != nil {
		log.Fatalf("kubegroup: %v", errKg)
	}

	//
	// create cache
	//

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroup("gateways", app.config.groupCacheSizeBytes, groupcache.GetterFunc(
		func(ctx context.Context, gatewayName string, dest groupcache.Sink) error {

			out, _, errID := repoGetMultiple(ctx, app, gatewayName)
			if errID != nil {
				return errID
			}

			var expire time.Time // zero value for expire means no expiration
			if app.config.groupCacheExpire != 0 {
				expire = time.Now().Add(app.config.groupCacheExpire)
			}

			dest.SetString(out.GatewayID, expire)

			return nil
		}))

	//
	// expose prometheus metrics for groupcache
	//

	mailgun := mailgun.New(app.cache)
	labels := map[string]string{
		//"app": appName,
	}
	namespace := ""
	collector := groupcache_exporter.NewExporter(namespace, labels, mailgun)
	prometheus.MustRegister(collector)
}

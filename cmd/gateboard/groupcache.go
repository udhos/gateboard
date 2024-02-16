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
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

	//
	// create groupcache pool
	//

	var myURL string
	for myURL == "" {
		var errURL error
		myURL, errURL = kubegroup.FindMyURL(app.config.groupCachePort)
		if errURL != nil {
			log.Printf("my URL: %v", errURL)
		}
		if myURL == "" {
			const cooldown = 5 * time.Second
			log.Printf("could not find my URL, sleeping %v", cooldown)
			time.Sleep(cooldown)
		}
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

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.config.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		MetricsRegisterer: app.registry,
		MetricsGatherer:   app.registry,
		Debug:             app.config.kubegroupDebug,
		ListerInterval:    app.config.kubegroupListerInterval,
	}

	go kubegroup.UpdatePeers(options)

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

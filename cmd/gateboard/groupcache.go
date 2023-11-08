package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mailgun/groupcache"
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

	go kubegroup.UpdatePeers(pool, app.config.groupCachePort)

	//
	// create cache
	//

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroup("gateways", 64<<20, groupcache.GetterFunc(
		func(c groupcache.Context, gatewayName string, dest groupcache.Sink) error {

			var ctx context.Context
			if c == nil {
				ctx = context.Background()
			} else {
				ctx = c.(context.Context)
			}

			out, _, errID := repoGetMultiple(ctx, app, gatewayName)
			if errID != nil {
				return errID
			}

			var expire time.Time // zero value for expire means no expiration
			if app.config.TTL != 0 {
				expire = time.Now().Add(time.Second * time.Duration(app.config.TTL))
			}

			dest.SetString(out.GatewayID, expire)

			return nil
		}))

}

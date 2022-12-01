/*
Package gateboard provides library for clients.
*/
package gateboard

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// Client holds context for a gateboard client.
type Client struct {
	options ClientOptions
	cache   map[string]gatewayEntry
	lock    sync.Mutex
	TTL     time.Duration
}

const (
	CacheTTLMinimum = 60 * time.Second  // CacheTTLMinimum defines min limit for TTL
	CacheTTLMax     = 600 * time.Second // CacheTTLMax defines max limit for TTL
	CacheTTLDefault = 300 * time.Second // CacheTTLDefault defines default value for TTL
)

type gatewayEntry struct {
	gatewayID string
	creation  time.Time
}

// ClientOptions defines options for the client.
type ClientOptions struct {
	ServerURL   string // main centralized server
	FallbackURL string // local fallback server (data cached from main server)
	TTLMin      time.Duration
	TTLMax      time.Duration
	TTLDefault  time.Duration
	Debug       bool
}

// NewClient creates a new gateboard client.
func NewClient(options ClientOptions) *Client {
	if options.TTLMin == 0 {
		options.TTLMin = CacheTTLMinimum
	}
	if options.TTLMax == 0 {
		options.TTLMax = CacheTTLMax
	}
	if options.TTLDefault == 0 {
		options.TTLDefault = CacheTTLDefault
	}
	return &Client{
		options: options,
		cache:   map[string]gatewayEntry{},
		TTL:     options.TTLDefault,
	}
}

func (c *Client) updateTTL(TTL int) {
	if TTL < 1 {
		return
	}
	t := time.Second * time.Duration(TTL)
	switch {
	case t < c.options.TTLMin:
		t = c.options.TTLMin
	case t > c.options.TTLMax:
		t = c.options.TTLMax
	}
	if c.options.Debug {
		log.Printf("updateTTL: arg TTL=%d, cache TTL=%v", TTL, t)
	}
	c.lock.Lock()
	c.TTL = t
	c.lock.Unlock()
}

func (c *Client) getTTL() time.Duration {
	c.lock.Lock()
	TTL := c.TTL
	c.lock.Unlock()
	return TTL
}

// BodyGetReply defines the payload format for a GET request.
type BodyGetReply struct {
	GatewayName string `json:"gateway_name"    yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"      yaml:"gateway_id"`
	Error       string `json:"error,omitempty" yaml:"error,omitempty"`
	TTL         int    `json:"TTL,omitempty"   yaml:"TTL,omitempty"`
}

func (c *Client) cacheGet(gatewayName string) (gatewayEntry, bool) {
	c.lock.Lock()
	entry, found := c.cache[gatewayName]
	c.lock.Unlock()
	return entry, found
}

func (c *Client) cachePut(gatewayName, gatewayID string) {
	entry := gatewayEntry{gatewayID: gatewayID, creation: time.Now()}
	c.lock.Lock()
	c.cache[gatewayName] = entry
	c.lock.Unlock()
}

// GatewayID retrieves the gateway ID for a `gatewayName` from local fast cache.
// If the result is an empty string, the method `Refresh()` should be called to asynchronously update the cache.
func (c *Client) GatewayID(gatewayName string) string {
	const me = "gateboard.Client.GatewayID"

	// 1: local cache with TTL

	{
		entry, found := c.cacheGet(gatewayName)
		if found {
			elap := time.Since(entry.creation)
			TTL := c.getTTL()
			if elap < TTL {
				log.Printf("%s: name=%s id=%s from cache TTL=%v", me, gatewayName, entry.gatewayID, TTL-elap)
				return entry.gatewayID
			}
		}
	}

	return ""
}

/*
// https://stackoverflow.com/questions/52793601/how-to-run-single-instance-of-goroutine

var jobIsRunning uint32

	func maybeStartJob() {
	    if atomic.CompareAndSwapUint32(&jobIsRunning, 0, 1) {
	        go func() {
	            theJob()
	            atomic.StoreUint32(&jobIsRunning, 0)
	        }()
	    }
	}
*/

var refreshing uint32

// Refresh runs only one refreshJob() goroutine at a time.
func (c *Client) Refresh(gatewayName, gatewayID string) {
	if atomic.CompareAndSwapUint32(&refreshing, 0, 1) {
		go func() {
			c.refreshJob(gatewayName, gatewayID)
			atomic.StoreUint32(&refreshing, 0)
		}()
	}
}

func (c *Client) refreshJob(gatewayName, oldGatewayID string) {
	const me = "refreshJob"

	log.Printf("%s: gateway_name=%s old_gateway_id=%s", me, gatewayName, oldGatewayID)

	// 1: query main server

	{
		gatewayID, TTL := c.queryServer(c.options.ServerURL, gatewayName)
		c.updateTTL(TTL)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			if c.options.FallbackURL != "" {
				c.saveFallback(gatewayName, gatewayID)
			}
			log.Printf("%s: gateway_name=%s old_gateway_id=%s new_gateway_id=%s from server",
				me, gatewayName, oldGatewayID, gatewayID)
			return
		}
	}

	// 2: query local fallback repository, if any

	if c.options.FallbackURL != "" {
		gatewayID, _ := c.queryServer(c.options.FallbackURL, gatewayName)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			log.Printf("%s: gateway_name=%s old_gateway_id=%s new_gateway_id=%s from repo",
				me, gatewayName, oldGatewayID, gatewayID)
			return
		}
	}

	log.Printf("%s: gateway_name=%s old_gateway_id=%s: failed to refresh", me, gatewayName, oldGatewayID)
}

func (c *Client) queryServer(URL, gatewayName string) (string, int) {
	const me = "gateboard.Client.queryServer"

	path, errPath := url.JoinPath(URL, gatewayName)
	if errPath != nil {
		log.Printf("%s: URL=%s join error: %v", me, path, errPath)
		return "", 0
	}

	resp, errGet := http.Get(path)
	if errGet != nil {
		log.Printf("%s: URL=%s server error: %v", me, path, errGet)
		return "", 0
	}

	defer resp.Body.Close()

	var reply BodyGetReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		log.Printf("%s: URL=%s yaml error: %v", me, path, errYaml)
		return "", 0
	}

	log.Printf("%s: URL=%s gateway: %v", me, path, toJSON(reply))

	return reply.GatewayID, reply.TTL
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJSON: %v", err)
	}
	return string(b)
}

// BodyPutRequest defines the payload format for a PUT request.
type BodyPutRequest struct {
	GatewayID string `json:"gateway_id" yaml:"gateway_id"`
}

// BodyPutReply defines the payload format for a PUT response.
type BodyPutReply struct {
	GatewayName string `json:"gateway_name"`
	GatewayID   string `json:"gateway_id"`
	Error       string `json:"error,omitempty"`
}

func (c *Client) saveFallback(gatewayName, gatewayID string) {
	const me = "gateboard.Client.saveFallback"

	path, errPath := url.JoinPath(c.options.FallbackURL, gatewayName)
	if errPath != nil {
		log.Printf("%s: URL=%s join error: %v", me, path, errPath)
		return
	}

	//log.Printf("%s: URL=%s gatewayName=%s gatewayID=%s", me, path, gatewayName, gatewayID)

	requestBody := BodyPutRequest{GatewayID: gatewayID}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		log.Printf("%s: URL=%s json error: %v", me, path, errJSON)
		return
	}

	//log.Printf("%s: URL=%s gatewayName=%s gatewayID=%s json:%v", me, path, gatewayName, gatewayID, string(requestBytes))

	req, errReq := http.NewRequest("PUT", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		log.Printf("%s: URL=%s request error: %v", me, path, errReq)
		return
	}

	client := http.DefaultClient
	resp, errDo := client.Do(req)
	if errDo != nil {
		log.Printf("%s: URL=%s server error: %v", me, path, errDo)
		return
	}

	defer resp.Body.Close()

	var reply BodyPutReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		log.Printf("%s: URL=%s yaml error: %v", me, path, errYaml)
		return
	}

	log.Printf("%s: URL=%s gateway: %v", me, path, toJSON(reply))
}

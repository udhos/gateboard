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
	//cache map[string]string
	lock sync.Mutex
}

// DefaultCacheTTL is the default cache TTL.
const DefaultCacheTTL = 60 * time.Second

type gatewayEntry struct {
	gatewayID string
	creation  time.Time
}

// ClientOptions defines options for the client.
type ClientOptions struct {
	ServerURL   string // main centralized server
	FallbackURL string // local fallback server (data cached from main server)
	TTL         time.Duration
}

// NewClient creates a new gateboard client.
func NewClient(options ClientOptions) *Client {
	if options.TTL == 0 {
		options.TTL = DefaultCacheTTL
	}
	return &Client{
		options: options,
		cache:   map[string]gatewayEntry{},
		//cache: map[string]string{},
	}
}

// BodyGetReply defines the payload format for a GET request.
type BodyGetReply struct {
	GatewayName string `json:"gateway_name"    yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"      yaml:"gateway_id"`
	Error       string `json:"error,omitempty" yaml:"error,omitempty"`
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

/*
func (c *Client) GatewayID(gatewayName string) (string, error) {
	const me = "gateboard.Client.GatewayID"

	//log.Printf("%s: gatewayName=%s", me, gatewayName)

	// 1: local cache with TTL=60s

	{
		entry, found := c.cacheGet(gatewayName)
		if found {
			elap := time.Since(entry.creation)
			if elap < cacheTTL {
				log.Printf("%s: name=%s id=%s from cache TTL=%v", me, gatewayName, entry.gatewayID, cacheTTL-elap)
				return entry.gatewayID, nil
			}
		}
	}

	// 2: server

	{
		gatewayID := c.queryServer(c.options.ServerURL, gatewayName)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
                        if c.options.FallbackURL != "" {
			        c.saveFallback(gatewayName, gatewayID)
                        }
			log.Printf("%s: name=%s id=%s from server", me, gatewayName, gatewayID)
			return gatewayID, nil
		}
	}

	// 3: fallback repository, if any

	if c.options.FallbackURL != "" {
		gatewayID := c.queryServer(c.options.FallbackURL, gatewayName)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			log.Printf("%s: name=%s id=%s from repo", me, gatewayName, gatewayID)
			return gatewayID, nil
		}
	}

	return "", fmt.Errorf("%s: gatewayName=%s not found", me, gatewayName)
}

func (c *Client) Refresh(gatewayName, gatewayID string) {
}
*/

/*
func (c *Client) cacheGet(gatewayName string) (string, bool) {
	c.lock.Lock()
	entry, found := c.cache[gatewayName]
	c.lock.Unlock()
	return entry, found
}

func (c *Client) cachePut(gatewayName, gatewayID string) {
	c.lock.Lock()
	c.cache[gatewayName] = gatewayID
	c.lock.Unlock()
}
*/

// GatewayID retrieves the gateway ID for a `gatewayName` from local fast cache.
// If the result is an empty string, the method `Refresh()` should be called to asynchronously update the cache.
func (c *Client) GatewayID(gatewayName string) string {
	const me = "gateboard.Client.GatewayID"

	// 1: local cache with TTL

	{
		entry, found := c.cacheGet(gatewayName)
		if found {
			elap := time.Since(entry.creation)
			if elap < c.options.TTL {
				log.Printf("%s: name=%s id=%s from cache TTL=%v", me, gatewayName, entry.gatewayID, c.options.TTL-elap)
				return entry.gatewayID
			}
		}
	}

	return ""

	/*
		// 2: server

		{
			gatewayID := c.queryServer(c.options.ServerURL, gatewayName)
			if gatewayID != "" {
				c.cachePut(gatewayName, gatewayID)
                                if c.options.FallbackURL != "" {
			                c.saveFallback(gatewayName, gatewayID)
                                }
				log.Printf("%s: name=%s id=%s from server", me, gatewayName, gatewayID)
				return gatewayID, nil
			}
		}

		// 3: fallback repository, if any

		if c.options.FallbackURL != "" {
			gatewayID := c.queryServer(c.options.FallbackURL, gatewayName)
			if gatewayID != "" {
				c.cachePut(gatewayName, gatewayID)
				log.Printf("%s: name=%s id=%s from repo", me, gatewayName, gatewayID)
				return gatewayID, nil
			}
		}

		return "", fmt.Errorf("%s: gatewayName=%s not found", me, gatewayName)
	*/
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
		gatewayID := c.queryServer(c.options.ServerURL, gatewayName)
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
		gatewayID := c.queryServer(c.options.FallbackURL, gatewayName)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			log.Printf("%s: gateway_name=%s old_gateway_id=%s new_gateway_id=%s from repo",
				me, gatewayName, oldGatewayID, gatewayID)
			return
		}
	}

	log.Printf("%s: gateway_name=%s old_gateway_id=%s: failed to refresh", me, gatewayName, oldGatewayID)
}

func (c *Client) queryServer(URL, gatewayName string) string {
	const me = "gateboard.Client.queryServer"

	path, errPath := url.JoinPath(URL, gatewayName)
	if errPath != nil {
		log.Printf("%s: URL=%s join error: %v", me, path, errPath)
		return ""
	}

	resp, errGet := http.Get(path)
	if errGet != nil {
		log.Printf("%s: URL=%s server error: %v", me, path, errGet)
		return ""
	}

	defer resp.Body.Close()

	var reply BodyGetReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		log.Printf("%s: URL=%s yaml error: %v", me, path, errYaml)
		return ""
	}

	log.Printf("%s: URL=%s gateway: %v", me, path, toJSON(reply))

	return reply.GatewayID
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

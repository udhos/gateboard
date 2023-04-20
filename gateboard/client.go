// Package gateboard provides library for clients.
//
// # Recommended Usage
//
// This pseudocode illustrates the recommended usage:
//
//	// invokeBackend calls a backend http endpoint for a gateway named 'gatewayName'.
//	// 'client' is created in an wider scope because it caches IDs.
//	function invokeBackend(client, gatewayName)
//	    1. get ID := client.GatewayID(gatewayName)
//	    2. if ID is "" {
//	           return status code 503
//	       }
//	    3. call the backend http endpoint with header "x-apigw-api-id: <id>"
//	       if backend status code is 403 {
//	           client.Refresh(gatewayName)
//	           return status code 503
//	       }
//	    4. return backend status code
package gateboard

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	yaml "gopkg.in/yaml.v3"
)

// Client holds context for a gateboard client.
type Client struct {
	options     ClientOptions
	cache       map[string]gatewayEntry
	lock        sync.Mutex
	TTL         time.Duration
	flightGroup singleflight.Group
	random      *rand.Rand
}

const (
	CacheTTLMinimum = 1 * time.Minute  // CacheTTLMinimum defines min limit for TTL
	CacheTTLMax     = 20 * time.Minute // CacheTTLMax defines max limit for TTL
	CacheTTLDefault = 5 * time.Minute  // CacheTTLDefault defines default value for TTL
)

type gatewayEntry struct {
	gatewayID string
	creation  time.Time
}

// ClientOptions defines options for the client.
type ClientOptions struct {
	ServerURL  string        // required main centralized server
	TTLMin     time.Duration // optional, if unspecified defaults to CacheTTLMinimum
	TTLMax     time.Duration // optional, if unspecified defaults to CacheTTLMax
	TTLDefault time.Duration // optional, if unspecified defaults to CacheTTLDefault
	Debug      bool          // optional, log debug information
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
		random:  rand.New(rand.NewSource(time.Now().UnixNano())),
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
	GatewayName string    `json:"gateway_name"    yaml:"gateway_name"    bson:"gateway_name"    dynamodbav:"gateway_name"`
	GatewayID   string    `json:"gateway_id"      yaml:"gateway_id"      bson:"gateway_id"      dynamodbav:"gateway_id"`
	Changes     int64     `json:"changes"         yaml:"changes"         bson:"changes"         dynamodbav:"changes"`
	LastUpdate  time.Time `json:"last_update"     yaml:"last_update"     bson:"last_update"     dynamodbav:"last_update"`
	Error       string    `json:"error,omitempty" yaml:"error,omitempty" bson:"error,omitempty" dynamodbav:"error,omitempty"`
	TTL         int       `json:"TTL,omitempty"   yaml:"TTL,omitempty"   bson:"TTL,omitempty"   dynamodbav:"TTL,omitempty"`
	Token       string    `json:"token,omitempty" yaml:"token,omitempty" bson:"token,omitempty" dynamodbav:"token,omitempty"`
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

// GatewayID retrieves the gateway ID for a 'gatewayName' from local fast cache.
// If the ID is not found in the local fast cache, it will use 'singleflight' to fetch up-to-date data.
func (c *Client) GatewayID(gatewayName string) string {
	const me = "gateboard.Client.GatewayID"

	list := c.getID(gatewayName)
	if list == "" {
		return ""
	}

	id, errPick := c.pickOne(gatewayName, list)
	if errPick != nil {
		log.Printf("%s: name=%s id=%s error: %v", me, gatewayName, id, errPick)
	}

	return id
}

func (c *Client) getID(gatewayName string) string {
	const me = "gateboard.Client.getID"

	// 1: local cache with TTL

	{
		entry, found := c.cacheGet(gatewayName)
		if found {
			elap := time.Since(entry.creation)
			TTL := c.getTTL()
			if elap < TTL {
				if c.options.Debug {
					log.Printf("%s: name=%s id=%s from cache TTL=%v", me, gatewayName, entry.gatewayID, TTL-elap)
				}
				return entry.gatewayID
			}
		}
	}

	// 2: fetch from server

	result, err, shared := c.flightGroup.Do(gatewayName, func() (interface{}, error) {
		return c.refresh(gatewayName)
	})

	id := result.(string)

	if err != nil || c.options.Debug {
		log.Printf("%s: gateway='%s' id='%s' shared=%t error:%v", me, gatewayName, id, shared, err)
		return id
	}

	return id
}

// pickOne randomly picks one id from a weighted list of ids.
// list entry: "id;weight".
// list: "id1;weight1,id2;weight2,id3;weight3".
// example: "id1;5,id2;2,id3;3".
// simplest list is "id1".
// omitted weight defaults to 1.
func (c *Client) pickOne(gatewayName, listStr string) (string, error) {
	list, err := newIDList(listStr)
	if err != nil {
		return "", fmt.Errorf("pickOne: gateway='%s' id='%s' list error: %v",
			gatewayName, listStr, err)
	}
	if len(list.list) == 0 {
		return "", fmt.Errorf("pickOne: gateway='%s' id='%s' empty id list",
			gatewayName, listStr)
	}
	if len(list.list) == 1 {
		return list.list[0].id, nil // only a single element
	}

	r := c.random.Intn(list.sum) + 1 // random 1..sum

	id := pickRandom(list, r)

	if c.options.Debug {
		log.Printf("pickOne: list=%s listSum=%d random=%d id=%s",
			listStr, list.sum, r, id)
	}

	return id, nil
}

func pickRandom(list idList, random int) string {
	var runningSum int

	for _, e := range list.list {
		runningSum += e.weight
		if random <= runningSum {
			return e.id
		}
	}

	log.Printf("pickRandom: failed to pick random id: list=%v random=%d", list, random)

	return list.list[0].id
}

// refresh fetches up-to-date data from server.
// Whenever new data is found, the local cache is updated.
func (c *Client) refresh(gatewayName string) (string, error) {
	const me = "refresh"

	if c.options.Debug {
		log.Printf("%s: gateway_name=%s", me, gatewayName)
	}

	{
		gatewayID, TTL := c.queryServer(c.options.ServerURL, gatewayName)
		c.updateTTL(TTL)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			if c.options.Debug {
				log.Printf("%s: gateway_name=%s gateway_id=%s from main server",
					me, gatewayName, gatewayID)
			}
			return gatewayID, nil
		}
	}

	return "", fmt.Errorf("%s: gateway_name=%s: error: failed to refresh", me, gatewayName)
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

// Refresh spawns only one refreshJob() goroutine at a time.
// The async refresh job will attempt to update the local fast cache entry for gatewayName with information retrieved from server.
func (c *Client) Refresh(gatewayName string) {
	if atomic.CompareAndSwapUint32(&refreshing, 0, 1) {
		go func() {
			c.refreshJob(gatewayName)
			atomic.StoreUint32(&refreshing, 0)
		}()
	}
}

func (c *Client) refreshJob(gatewayName string) {
	const me = "refreshJob"
	_, err := c.refresh(gatewayName)
	if err != nil {
		log.Printf("%s: %v", me, err)
	}
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

	if c.options.Debug {
		log.Printf("%s: URL=%s gateway: %v", me, path, toJSON(reply))
	}

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
	Token     string `json:"token"      yaml:"token"`
}

// BodyPutReply defines the payload format for a PUT response.
type BodyPutReply struct {
	GatewayName string `json:"gateway_name"    yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"      yaml:"gateway_id"`
	Error       string `json:"error,omitempty" yaml:"error,omitempty"`
}

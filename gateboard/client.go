package gateboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v3"
)

type Client struct {
	options ClientOptions
	cache   map[string]gatewayEntry
	lock    sync.Mutex
}

const cacheTTL = 60 * time.Second

type gatewayEntry struct {
	gatewayID string
	creation  time.Time
}

type ClientOptions struct {
	ServerURL string
}

func NewClient(options ClientOptions) *Client {
	return &Client{options: options, cache: map[string]gatewayEntry{}}
}

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
		gatewayID := c.queryServer(gatewayName)
		if gatewayID != "" {
			c.cachePut(gatewayName, gatewayID)
			c.saveRepo(gatewayName, gatewayID)
			log.Printf("%s: name=%s id=%s from server", me, gatewayName, gatewayID)
			return gatewayID, nil
		}
	}

	// 3: repository

	gatewayID := c.queryRepo(gatewayName)
	if gatewayID != "" {
		c.cachePut(gatewayName, gatewayID)
		log.Printf("%s: name=%s id=%s from repo", me, gatewayName, gatewayID)
		return gatewayID, nil
	}

	return "", fmt.Errorf("%s: gatewayName=%s not found", me, gatewayName)
}

func (c *Client) queryServer(gatewayName string) string {
	const me = "gateboard.Client.queryServer"

	path, errPath := url.JoinPath(c.options.ServerURL, gatewayName)
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

func (c *Client) queryRepo(gatewayName string) string {
	log.Printf("queryRepo FIXME")
	return "FIXME"
}

func (c *Client) saveRepo(gatewayName, gatewayID string) {
	log.Printf("saveRepo FIXME")
}

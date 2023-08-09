package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/udhos/gateboard/gateboard"
)

//
// Repository: Memory
//

type memEntry struct {
	id         string
	changes    int64
	lastUpdate time.Time
	token      string
}

type repoMem struct {
	options repoMemOptions
	tab     map[string]memEntry // name => id
	lock    sync.Mutex
}

type repoMemOptions struct {
	metricRepoName string // kind:name
}

func newRepoMem(opt repoMemOptions) *repoMem {
	return &repoMem{
		options: opt,
		tab:     map[string]memEntry{},
	}
}

func (r *repoMem) repoName() string {
	return r.options.metricRepoName
}

func (r *repoMem) dump(ctx context.Context) (repoDump, error) {
	list := make(repoDump, 0, len(r.tab))
	r.lock.Lock()

	for name, e := range r.tab {
		item := map[string]interface{}{
			"gateway_name": name,
			"gateway_id":   e.id,
			"changes":      e.changes,
			"last_update":  e.lastUpdate,
			"token":        e.token,
		}
		list = append(list, item)
	}

	r.lock.Unlock()
	return list, nil
}

func (r *repoMem) get(ctx context.Context, gatewayName string) (gateboard.BodyGetReply, error) {
	var result gateboard.BodyGetReply

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return result, errVal
	}

	r.lock.Lock()
	e, found := r.tab[gatewayName]
	r.lock.Unlock()
	result.GatewayName = gatewayName
	if found {
		result.GatewayID = e.id
		result.Changes = e.changes
		result.LastUpdate = e.lastUpdate
		result.Token = e.token
		return result, nil
	}
	return result, errRepositoryGatewayNotFound
}

func (r *repoMem) put(ctx context.Context, gatewayName, gatewayID string) error {

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return errVal
	}

	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("repoMem.put: bad gateway id: '%s'", gatewayID)
	}
	now := time.Now()
	r.lock.Lock()
	e, _ := r.tab[gatewayName]
	e.id = gatewayID
	e.changes++
	e.lastUpdate = now
	r.tab[gatewayName] = e
	r.lock.Unlock()
	return nil
}

func (r *repoMem) putToken(ctx context.Context, gatewayName, token string) error {
	r.lock.Lock()
	e, _ := r.tab[gatewayName]
	e.token = token
	r.tab[gatewayName] = e
	r.lock.Unlock()
	return nil
}

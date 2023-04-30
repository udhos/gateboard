package main

import (
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
	tab  map[string]memEntry // name => id
	lock sync.Mutex
}

func newRepoMem() *repoMem {
	return &repoMem{tab: map[string]memEntry{}}
}

func (r *repoMem) dump() (repoDump, error) {
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

func (r *repoMem) get(gatewayName string) (gateboard.BodyGetReply, error) {
	var result gateboard.BodyGetReply
	if strings.TrimSpace(gatewayName) == "" {
		return result, fmt.Errorf("repoMem.get: bad gateway name: '%s'", gatewayName)
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

func (r *repoMem) put(gatewayName, gatewayID string) error {
	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("repoMem.put: bad gateway name: '%s'", gatewayName)
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

func (r *repoMem) putToken(gatewayName, token string) error {
	r.lock.Lock()
	e, _ := r.tab[gatewayName]
	e.token = token
	r.tab[gatewayName] = e
	r.lock.Unlock()
	return nil
}

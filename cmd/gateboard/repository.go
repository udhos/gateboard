package main

import (
	"errors"
	"sync"
)

type repository interface {
	get(gateway_name string) (string, error)
	put(gateway_name, gateway_id string) error
}

var errRepositoryGatewayNotFound = errors.New("repository gateway not found error")

type repoMem struct {
	tab  map[string]string
	lock sync.Mutex
}

func NewRepoMem() *repoMem {
	return &repoMem{tab: map[string]string{}}
}

func (r *repoMem) get(gateway_name string) (string, error) {
	r.lock.Lock()
	gateway_id, found := r.tab[gateway_name]
	r.lock.Unlock()
	if found {
		return gateway_id, nil
	}
	return "", errRepositoryGatewayNotFound
}

func (r *repoMem) put(gateway_name, gateway_id string) error {
	r.lock.Lock()
	r.tab[gateway_name] = gateway_id
	r.lock.Unlock()
	return nil
}

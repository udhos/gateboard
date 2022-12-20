package main

import (
	"errors"

	"github.com/udhos/gateboard/gateboard"
)

type repository interface {
	get(gatewayName string) (gateboard.BodyGetReply, error)
	put(gatewayName, gatewayID string) error
	dump() (repoDump, error)
	putToken(gatewayName, token string) error
}

type repoDump []map[string]interface{}

var (
	errRepositoryGatewayNotFound    = errors.New("repository: gateway not found error")
	errRepositoryGatewayIDNotString = errors.New("repository: gateway ID not a string")
)

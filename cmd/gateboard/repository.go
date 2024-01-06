package main

import (
	"context"
	"errors"

	"github.com/udhos/gateboard/gateboard"
)

type repository interface {
	get(ctx context.Context, gatewayName string) (gateboard.BodyGetReply, error)
	put(ctx context.Context, gatewayName, gatewayID string) error
	dump(ctx context.Context) (repoDump, error)
	putToken(ctx context.Context, gatewayName, token string) error
	repoName() string
}

type repoDump []map[string]interface{}

var (
	errRepositoryGatewayNotFound = errors.New("repository: gateway not found error")
	errRepositoryTimeout         = errors.New("repository: cross-repository timeout")
)

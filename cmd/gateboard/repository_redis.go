package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/gateboard"
)

type repoRedisOptions struct {
	metricRepoName        string // kind:name
	debug                 bool
	addr                  string
	password              string
	key                   string
	tls                   bool
	tlsInsecureSkipVerify bool
	clientName            string
}

type repoRedis struct {
	options     repoRedisOptions
	redisClient *redis.Client
}

func newRepoRedis(opt repoRedisOptions) (*repoRedis, error) {

	redisOptions := &redis.Options{
		Addr:       opt.addr,
		Password:   opt.password,
		DB:         0,
		ClientName: opt.clientName,
	}

	if opt.tls || opt.tlsInsecureSkipVerify {
		redisOptions.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: opt.tlsInsecureSkipVerify,
		}
	}

	r := &repoRedis{
		options:     opt,
		redisClient: redis.NewClient(redisOptions),
	}

	return r, nil
}

func (r *repoRedis) repoName() string {
	return r.options.metricRepoName
}

func (r *repoRedis) dropDatabase() error {
	return r.redisClient.Del(context.TODO(), r.options.key).Err()
}

const (
	prefix = "gateway:"
	match  = prefix + "*"
)

func (r *repoRedis) dump(ctx context.Context) (repoDump, error) {
	const me = "repoRedis.dump"

	list := repoDump{}

	tab := map[string]map[string]interface{}{}

	isKey := true
	var gatewayField string
	var gatewayName string

	iter := r.redisClient.HScan(ctx, r.options.key, 0, match, 0).Iterator()
	for iter.Next(ctx) {
		if err := iter.Err(); err != nil {
			return list, err
		}
		val := iter.Val()
		zlog.CtxDebugf(ctx, r.options.debug, "%s: isKey=%-5v value:%s", me, isKey, val)
		if isKey {
			// found key: gateway:gateway_id:gw1
			_, after, _ := strings.Cut(val, ":")
			gatewayField, gatewayName, _ = strings.Cut(after, ":")
		} else {
			// found value: id1
			gateway, found := tab[gatewayName]
			if !found {
				gateway = map[string]interface{}{
					"gateway_name": gatewayName,
				}
				tab[gatewayName] = gateway
			}
			gateway[gatewayField] = val
		}
		isKey = !isKey
	}

	for _, g := range tab {
		list = append(list, g)
	}

	return list, nil
}

// prefix:field_name:gateway_name = field_value
//
// gateway:gateway_id:gateway1    = id1
// gateway:changes:gateway1       = 4
func field(gatewayName, field string) string {
	return prefix + field + ":" + gatewayName
}

func (r *repoRedis) get(ctx context.Context, gatewayName string) (gateboard.BodyGetReply, error) {
	const me = "repoRedis.get"

	body := gateboard.BodyGetReply{GatewayName: gatewayName}

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return body, errVal
	}

	fieldID := field(gatewayName, "gateway_id")
	fieldChanges := field(gatewayName, "changes")
	fieldLastUpdate := field(gatewayName, "last_update")
	fieldToken := field(gatewayName, "token")

	// id

	cmdID := r.redisClient.HGet(ctx, r.options.key, fieldID)
	errID := cmdID.Err()
	if errID == redis.Nil {
		return body, errRepositoryGatewayNotFound
	}
	if errID != nil {
		return body, cmdID.Err()
	}
	body.GatewayID = cmdID.Val()

	// changes

	cmdChanges := r.redisClient.HGet(ctx, r.options.key, fieldChanges)
	if err := cmdChanges.Err(); err != nil {
		zlog.CtxErrorf(ctx, "%s: changes retrieve: %v", me, err)
	}
	changes, errInt64 := cmdChanges.Int64()
	if errInt64 != nil {
		zlog.CtxErrorf(ctx, "%s: changes int: %v", me, errInt64)
	}
	body.Changes = changes

	// last update

	cmdLastUpdate := r.redisClient.HGet(ctx, r.options.key, fieldLastUpdate)
	if err := cmdLastUpdate.Err(); err != nil {
		zlog.CtxErrorf(ctx, "%s: last update retrieve: %v", me, err)
	}
	lastUpdate, errParse := time.Parse(time.RFC3339, cmdLastUpdate.Val())
	if errParse != nil {
		zlog.CtxErrorf(ctx, "%s: last update parse: %v", me, errParse)
	}
	body.LastUpdate = lastUpdate

	// token

	cmdToken := r.redisClient.HGet(ctx, r.options.key, fieldToken)
	if err := cmdToken.Err(); err != nil {
		zlog.CtxErrorf(ctx, "%s: token retrieve: %v", me, err)
	}
	body.Token = cmdToken.Val()

	return body, nil
}

func (r *repoRedis) put(ctx context.Context, gatewayName, gatewayID string) error {
	const me = "repoRedis.put"

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return errVal
	}

	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	fieldID := field(gatewayName, "gateway_id")
	fieldChanges := field(gatewayName, "changes")
	fieldLastUpdate := field(gatewayName, "last_update")

	if errHSetID := r.redisClient.HSet(ctx, r.options.key, fieldID, gatewayID).Err(); errHSetID != nil {
		return errHSetID
	}

	if errHIncrChanges := r.redisClient.HIncrBy(ctx, r.options.key, fieldChanges, 1).Err(); errHIncrChanges != nil {
		zlog.CtxErrorf(ctx, "%s: changes: %v", me, errHIncrChanges)
	}

	now := time.Now().Format(time.RFC3339)

	if errHSetLastUpdate := r.redisClient.HSet(ctx, r.options.key, fieldLastUpdate, now).Err(); errHSetLastUpdate != nil {
		zlog.CtxErrorf(ctx, "%s: last update: %v", me, errHSetLastUpdate)
	}

	return nil
}

func (r *repoRedis) putToken(ctx context.Context, gatewayName, token string) error {
	fieldToken := field(gatewayName, "token")
	return r.redisClient.HSet(ctx, r.options.key, fieldToken, token).Err()
}

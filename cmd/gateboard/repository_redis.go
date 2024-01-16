package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
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

	cmdMGet := r.redisClient.HMGet(ctx, r.options.key, fieldID, fieldChanges, fieldLastUpdate, fieldToken)
	errMGet := cmdMGet.Err()
	if errMGet == redis.Nil {
		return body, errRepositoryGatewayNotFound
	}
	if errMGet != nil {
		return body, cmdMGet.Err()
	}
	fieldValues := cmdMGet.Val()

	var ok bool

	//
	// gateway id
	//
	valueGatewayID := fieldValues[0]
	body.GatewayID, ok = valueGatewayID.(string)
	if !ok {
		return body, fmt.Errorf("field gateway_id not string: %[1]T: %[1]v", valueGatewayID)
	}

	//
	// changes
	//
	valueChanges := fieldValues[1]
	var changesStr string
	changesStr, ok = valueChanges.(string)
	if !ok {
		return body, fmt.Errorf("field changes not string: %[1]T: %[1]v", valueChanges)
	}
	changes, errConv := strconv.ParseInt(changesStr, 10, 64)
	if errConv != nil {
		zlog.CtxErrorf(ctx, "%s: parse changes: %v", me, errConv)
	}
	body.Changes = changes

	//
	// last update
	//
	valueLastUpdate := fieldValues[2]
	var lastUpdateStr string
	lastUpdateStr, ok = valueLastUpdate.(string)
	if !ok {
		return body, fmt.Errorf("field last_update not string: %[1]T: %[1]v", valueLastUpdate)
	}
	lastUpdate, errParse := time.Parse(time.RFC3339, lastUpdateStr)
	if errParse != nil {
		zlog.CtxErrorf(ctx, "%s: last update parse: %v", me, errParse)
	}
	body.LastUpdate = lastUpdate

	//
	// token
	//
	valueToken := fieldValues[3]
	if valueToken != nil {
		body.Token, ok = valueToken.(string)
		if !ok {
			return body, fmt.Errorf("field token not string: %[1]T: %[1]v", valueToken)
		}
	}

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

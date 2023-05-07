package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis"

	"github.com/udhos/gateboard/gateboard"
)

type repoRedisOptions struct {
	debug    bool
	addr     string
	password string
	key      string
}

type repoRedis struct {
	options     repoRedisOptions
	redisClient *redis.Client
}

func newRepoRedis(opt repoRedisOptions) (*repoRedis, error) {
	const me = "newRepoRedis"

	r := &repoRedis{
		options: opt,
		redisClient: redis.NewClient(&redis.Options{
			Addr:     opt.addr,
			Password: opt.password,
			DB:       0,
		}),
	}

	return r, nil
}

func (r *repoRedis) dropDatabase() error {
	return r.redisClient.Del(r.options.key).Err()
}

const (
	prefix = "gateway:"
	match  = prefix + "*"
)

func (r *repoRedis) dump() (repoDump, error) {
	const me = "repoRedis.dump"

	list := repoDump{}

	tab := map[string]map[string]interface{}{}

	isKey := true
	var gatewayField string
	var gatewayName string

	iter := r.redisClient.HScan(r.options.key, 0, match, 0).Iterator()
	for iter.Next() {
		if err := iter.Err(); err != nil {
			return list, err
		}
		val := iter.Val()
		if r.options.debug {
			log.Printf("%s: isKey=%-5v value:%s", me, isKey, val)
		}
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

func (r *repoRedis) get(gatewayName string) (gateboard.BodyGetReply, error) {
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

	cmdID := r.redisClient.HGet(r.options.key, fieldID)
	errID := cmdID.Err()
	if errID == redis.Nil {
		return body, errRepositoryGatewayNotFound
	}
	if errID != nil {
		return body, cmdID.Err()
	}
	body.GatewayID = cmdID.Val()

	// changes

	cmdChanges := r.redisClient.HGet(r.options.key, fieldChanges)
	if err := cmdChanges.Err(); err != nil {
		log.Printf("%s: changes retrieve: %v", me, err)
	}
	changes, errInt64 := cmdChanges.Int64()
	if errInt64 != nil {
		log.Printf("%s: changes int: %v", me, errInt64)
	}
	body.Changes = changes

	// last update

	cmdLastUpdate := r.redisClient.HGet(r.options.key, fieldLastUpdate)
	if err := cmdLastUpdate.Err(); err != nil {
		log.Printf("%s: last update retrieve: %v", me, err)
	}
	lastUpdate, errParse := time.Parse(time.RFC3339, cmdLastUpdate.Val())
	if errParse != nil {
		log.Printf("%s: last update parse: %v", me, errParse)
	}
	body.LastUpdate = lastUpdate

	// token

	cmdToken := r.redisClient.HGet(r.options.key, fieldToken)
	if err := cmdToken.Err(); err != nil {
		log.Printf("%s: token retrieve: %v", me, err)
	}
	body.Token = cmdToken.Val()

	return body, nil
}

func (r *repoRedis) put(gatewayName, gatewayID string) error {
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

	if errHSetID := r.redisClient.HSet(r.options.key, fieldID, gatewayID).Err(); errHSetID != nil {
		return errHSetID
	}

	if errHIncrChanges := r.redisClient.HIncrBy(r.options.key, fieldChanges, 1).Err(); errHIncrChanges != nil {
		log.Printf("%s: changes: %v", me, errHIncrChanges)
	}

	now := time.Now().Format(time.RFC3339)

	if errHSetLastUpdate := r.redisClient.HSet(r.options.key, fieldLastUpdate, now).Err(); errHSetLastUpdate != nil {
		log.Printf("%s: last update: %v", me, errHSetLastUpdate)
	}

	return nil
}

func (r *repoRedis) putToken(gatewayName, token string) error {
	const me = "repoRedis.putToken"

	fieldToken := field(gatewayName, "token")

	if errHSetToken := r.redisClient.HSet(r.options.key, fieldToken, token).Err(); errHSetToken != nil {
		return errHSetToken
	}

	return nil
}

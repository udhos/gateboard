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

func (r *repoRedis) dump() (repoDump, error) {
	const me = "repoRedis.dump"

	list := repoDump{}

	return list, nil
}

func (r *repoRedis) get(gatewayName string) (gateboard.BodyGetReply, error) {
	const me = "repoRedis.get"

	body := gateboard.BodyGetReply{GatewayName: gatewayName}

	if strings.TrimSpace(gatewayName) == "" {
		return body, fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}

	fieldID := "gateway_id:" + gatewayName
	fieldChanges := "changes:" + gatewayName
	fieldLastUpdate := "last_update:" + gatewayName
	fieldToken := "token:" + gatewayName

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

	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}
	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	fieldID := "gateway_id:" + gatewayName
	fieldChanges := "changes:" + gatewayName
	fieldLastUpdate := "last_update:" + gatewayName

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

	fieldToken := "token:" + gatewayName

	if errHSetToken := r.redisClient.HSet(r.options.key, fieldToken, token).Err(); errHSetToken != nil {
		return errHSetToken
	}

	return nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type repository interface {
	get(gatewayName string) (string, error)
	put(gatewayName, gatewayID string) error
	dump() (repoDump, error)
}

type repoDump []map[string]interface{}

var (
	errRepositoryGatewayNotFound    = errors.New("repository: gateway not found error")
	errRepositoryGatewayIDNotString = errors.New("repository: gateway ID not a string")
)

//
// Repository: Memory
//

type repoMem struct {
	tab  map[string]string
	lock sync.Mutex
}

func newRepoMem() *repoMem {
	return &repoMem{tab: map[string]string{}}
}

/*
// GatewayDump defines a item type for exporting database.
type GatewayDump struct {
	GatewayName string `json:"gateway_name" yaml:"gateway_name" bson:"gateway_name"`
	GatewayID   string `json:"gateway_id"   yaml:"gateway_id"   bson:"gateway_id"`
}
*/

func (r *repoMem) dump() (repoDump, error) {
	list := make(repoDump, 0, len(r.tab))
	r.lock.Lock()

	for name, id := range r.tab {
		item := map[string]interface{}{
			"GatewayName": name,
			"GatewayID":   id,
		}
		list = append(list, item)
	}

	r.lock.Unlock()
	return list, nil
}

func (r *repoMem) get(gatewayName string) (string, error) {
	if strings.TrimSpace(gatewayName) == "" {
		return "", fmt.Errorf("repoMem.get: bad gateway name: '%s'", gatewayName)
	}
	r.lock.Lock()
	gatewayID, found := r.tab[gatewayName]
	r.lock.Unlock()
	if found {
		return gatewayID, nil
	}
	return "", errRepositoryGatewayNotFound
}

func (r *repoMem) put(gatewayName, gatewayID string) error {
	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("repoMem.put: bad gateway name: '%s'", gatewayName)
	}
	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("repoMem.put: bad gateway id: '%s'", gatewayID)
	}
	r.lock.Lock()
	r.tab[gatewayName] = gatewayID
	r.lock.Unlock()
	return nil
}

//
// Repository: Mongo
//

type repoMongoOptions struct {
	debug      bool
	URI        string
	database   string
	collection string
	timeout    time.Duration
}

type repoMongo struct {
	options repoMongoOptions
	client  *mongo.Client
}

func newRepoMongo(opt repoMongoOptions) (*repoMongo, error) {
	const me = "newRepoMongo"

	r := &repoMongo{options: opt}

	//
	// connect
	//

	{
		ctx, cancel := context.WithTimeout(context.Background(), r.options.timeout)
		defer cancel()
		var errConnect error
		r.client, errConnect = mongo.Connect(ctx, options.Client().ApplyURI(r.options.URI).SetRetryWrites(false))
		if errConnect != nil {
			log.Printf("%s: mongo connect: %v", me, errConnect)
			return nil, errConnect
		}
	}

	//
	// create index
	//

	{
		const field = "gateway_name"
		collection := r.client.Database(r.options.database).Collection(r.options.collection)

		model := mongo.IndexModel{
			Keys: bson.M{
				field: 1, // index in ascending order
			}, Options: options.Index().SetUnique(true), // create UniqueIndex option
		}

		// withstand temporary errors (istio sidecar not ready)
		const cooldown = 5 * time.Second
		const retry = 10
		for i := 1; i <= retry; i++ {
			ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
			defer cancel()
			indexName, errCreate := collection.Indexes().CreateOne(ctxTimeout, model)
			if errCreate != nil {
				log.Printf("%s: attempt=%d/%d create index for field=%s: index=%s: error: %v, sleeping %v",
					me, i, retry, field, indexName, errCreate, cooldown)
				time.Sleep(cooldown)
				continue
			}
			log.Printf("%s: attempt=%d/%d create index for field=%s: index=%s: success",
				me, i, retry, field, indexName)
			break
		}
	}

	return r, nil
}

func (r *repoMongo) dropDatabase() error {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	return r.client.Database(r.options.database).Drop(ctxTimeout)
}

func (r *repoMongo) dump() (repoDump, error) {
	list := repoDump{}

	const me = "repoMongo.dump"

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	filter := bson.D{{}}
	findOptions := options.Find()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	cursor, errFind := collection.Find(ctxTimeout, filter, findOptions)

	if errFind != nil {
		log.Printf("%s: dump find error: %v", me, errFind)
		return list, errFind
	}

	ctxTimeout2, cancel2 := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel2()
	errAll := cursor.All(ctxTimeout2, &list)

	switch errAll {
	case mongo.ErrNoDocuments:
		return list, errRepositoryGatewayNotFound
	case nil:
		return list, nil
	}

	log.Printf("%s: dump cursor error: %v", me, errAll)
	return list, errAll
}

func (r *repoMongo) get(gatewayName string) (string, error) {

	const me = "repoMongo.get"

	if strings.TrimSpace(gatewayName) == "" {
		return "", fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	var result map[string]interface{}
	filter := bson.D{{Key: "gateway_name", Value: gatewayName}}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	errFind := collection.FindOne(ctxTimeout, filter).Decode(&result)

	switch errFind {
	case mongo.ErrNoDocuments:
		return "", errRepositoryGatewayNotFound
	case nil:
		value := result["gateway_id"]
		gatewayID, isStr := value.(string)
		if isStr {
			return gatewayID, nil
		}
		log.Printf("%s: gateway ID not a string: gatewayName=%s gatewayID=%v %T",
			me, gatewayName, value, value)
		return "", errRepositoryGatewayIDNotString
	}

	log.Printf("%s: gatewayName=%s find error: %v",
		me, gatewayName, errFind)

	return "", errFind
}

func (r *repoMongo) put(gatewayName, gatewayID string) error {

	const me = "repoMongo.put"

	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}
	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	filter := bson.D{{Key: "gateway_name", Value: gatewayName}}
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "gateway_id", Value: gatewayID}}},                                  // update ID
		{Key: "$inc", Value: bson.D{{Key: "changes", Value: 1}}},                                             // increment changes counter
		{Key: "$set", Value: bson.D{{Key: "last_update", Value: primitive.NewDateTimeFromTime(time.Now())}}}, // last update
	}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	opts := options.Update().SetUpsert(true)
	defer cancel()
	response, errUpdate := collection.UpdateOne(ctxTimeout, filter, update, opts)

	if errUpdate != nil {
		log.Printf("%s: gatewayName=%s gatewayID=%s update error:%v response:%v",
			me, gatewayName, gatewayID, errUpdate, *response)
		return errUpdate
	}

	return nil
}

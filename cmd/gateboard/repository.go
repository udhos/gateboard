package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type repository interface {
	get(gateway_name string) (string, error)
	put(gateway_name, gateway_id string) error
}

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

		ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
		defer cancel()
		indexName, errCreate := collection.Indexes().CreateOne(ctxTimeout, model)
		log.Printf("%s: create index for field=%s: index=%s: error: %v",
			me, field, indexName, errCreate)
	}

	return r, nil
}

func (r *repoMongo) get(gatewayName string) (string, error) {

	const me = "repoMongo.get"

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

	const me = "repoMongo.get"

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	filter := bson.D{{Key: "gateway_name", Value: gatewayName}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "gateway_id", Value: gatewayID}}}}
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

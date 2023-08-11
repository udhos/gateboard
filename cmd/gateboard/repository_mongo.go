package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/udhos/mongodbclient/mongodbclient"

	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/gateboard"
)

//
// Repository: Mongo
//

type repoMongoOptions struct {
	metricRepoName       string // kind:name
	debug                bool
	URI                  string
	database             string
	collection           string
	username             string
	password             string
	tlsCAFile            string
	minPool              uint64
	disableIndexCreation bool
	timeout              time.Duration
}

type repoMongo struct {
	options repoMongoOptions
	client  *mongo.Client
}

func newRepoMongo(opt repoMongoOptions) (*repoMongo, error) {
	const me = "newRepoMongo"

	r := &repoMongo{options: opt}

	{
		clientOptions := mongodbclient.Options{
			URI:         opt.URI,
			Username:    opt.username,
			Password:    opt.password,
			TLSCAFile:   opt.tlsCAFile,
			Timeout:     opt.timeout,
			MinPoolSize: opt.minPool,
			Debug:       opt.debug,
		}
		client, errConnect := mongodbclient.New(clientOptions)
		if errConnect != nil {
			return nil, errConnect
		}
		r.client = client
	}

	//
	// create index
	//

	if !opt.disableIndexCreation {
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
				zlog.Errorf("%s: attempt=%d/%d create index for field=%s: index=%s: error: %v, sleeping %v",
					me, i, retry, field, indexName, errCreate, cooldown)
				time.Sleep(cooldown)
				continue
			}
			zlog.Infof("%s: attempt=%d/%d create index for field=%s: index=%s: success",
				me, i, retry, field, indexName)
			break
		}
	}

	return r, nil
}

func (r *repoMongo) repoName() string {
	return r.options.metricRepoName
}

func (r *repoMongo) dropDatabase() error {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	return r.client.Database(r.options.database).Drop(ctxTimeout)
}

func (r *repoMongo) dump(ctx context.Context) (repoDump, error) {
	list := repoDump{}

	const me = "repoMongo.dump"

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	filter := bson.D{{}}
	findOptions := options.Find()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	cursor, errFind := collection.Find(ctxTimeout, filter, findOptions)

	if errFind != nil {
		zlog.CtxErrorf(ctx, "%s: dump find error: %v", me, errFind)
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

	zlog.CtxErrorf(ctx, "%s: dump cursor error: %v", me, errAll)

	return list, errAll
}

func (r *repoMongo) get(ctx context.Context, gatewayName string) (gateboard.BodyGetReply, error) {

	const me = "repoMongo.get"

	var body gateboard.BodyGetReply

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return body, errVal
	}

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	// Literal value queries return the same value as the $eq comparison operator.
	// For example, the following query filters produce the same result:
	// filter := bson.D{{"type", "Oolong"}}
	// filter := bson.D{{"type", bson.D{{"$eq", "Oolong"}}}}
	//
	// https://www.mongodb.com/docs/drivers/go/current/fundamentals/crud/read-operations/query-document/
	//
	filter := bson.D{{Key: "gateway_name", Value: gatewayName}}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	defer cancel()
	errFind := collection.FindOne(ctxTimeout, filter).Decode(&body)

	switch errFind {
	case mongo.ErrNoDocuments:
		return body, errRepositoryGatewayNotFound
	case nil:
		return body, nil
	}

	zlog.CtxErrorf(ctx, "%s: gatewayName=%s find error: %v",
		me, gatewayName, errFind)

	return body, errFind
}

func (r *repoMongo) put(ctx context.Context, gatewayName, gatewayID string) error {

	const me = "repoMongo.put"

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return errVal
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
		zlog.CtxErrorf(ctx, "%s: gatewayName=%s gatewayID=%s update error:%v response:%v",
			me, gatewayName, gatewayID, errUpdate, mongoResultString(response))
		return errUpdate
	}

	return nil
}

func (r *repoMongo) putToken(ctx context.Context, gatewayName, token string) error {

	const me = "repoMongo.putToken"

	collection := r.client.Database(r.options.database).Collection(r.options.collection)

	filter := bson.D{{Key: "gateway_name", Value: gatewayName}}
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "token", Value: token}}},
	}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), r.options.timeout)
	opts := options.Update().SetUpsert(true)
	defer cancel()
	response, errUpdate := collection.UpdateOne(ctxTimeout, filter, update, opts)

	if errUpdate != nil {
		zlog.CtxErrorf(ctx, "%s: gatewayName=%s token update error:%v response:%v",
			me, gatewayName, errUpdate, mongoResultString(response))
		return errUpdate
	}

	return nil
}

func mongoResultString(response *mongo.UpdateResult) string {
	if response == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *response)
}

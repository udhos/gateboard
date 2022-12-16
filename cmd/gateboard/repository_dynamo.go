package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/udhos/gateboard/gateboard"
)

type repoDynamoOptions struct {
	table       string
	region      string
	roleArn     string
	sessionName string
	debug       bool
}

type repoDynamo struct {
	options repoDynamoOptions
	dynamo  *dynamodb.Client
}

func newRepoDynamo(opt repoDynamoOptions) (*repoDynamo, error) {
	const me = "newRepoDynamo"

	cfg := awsConfig(opt.region, opt.roleArn, opt.sessionName)

	r := &repoDynamo{
		options: opt,
		dynamo:  dynamodb.NewFromConfig(cfg),
	}

	return r, nil
}

func (r *repoDynamo) dropDatabase() error {
	return nil
}

func (r *repoDynamo) dump() (repoDump, error) {
	const me = "repoDynamo.dump"

	list := repoDump{}

	return list, nil
}

func (r *repoDynamo) get(gatewayName string) (gateboard.BodyGetReply, error) {
	const me = "repoDynamo.get"

	var body gateboard.BodyGetReply

	if strings.TrimSpace(gatewayName) == "" {
		return body, fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}

	av, errMarshal := attributevalue.Marshal(gatewayName)
	if errMarshal != nil {
		return body, errMarshal
	}

	key := map[string]types.AttributeValue{"gateway_name": av}

	response, errGet := r.dynamo.GetItem(context.TODO(), &dynamodb.GetItemInput{
		Key: key, TableName: aws.String(r.options.table),
	})

	if errGet != nil {
		return body, errGet
	}

	if len(response.Item) == 0 {
		return body, errRepositoryGatewayNotFound
	}

	errUnmarshal := attributevalue.UnmarshalMap(response.Item, &body)

	return body, errUnmarshal
}

func (r *repoDynamo) put(gatewayName, gatewayID string) error {
	const me = "repoDynamo.put"

	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("%s: bad gateway name: '%s'", me, gatewayName)
	}
	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	//
	// get previous items since we need to increase the changes counter
	//

	body, errGet := r.get(gatewayName)
	switch errGet {
	case nil:
	case errRepositoryGatewayNotFound:
		body.GatewayName = gatewayName
	default:
		return errGet
	}

	//
	// update and save item
	//

	body.GatewayID = gatewayID
	body.LastUpdate = time.Now()
	body.Changes++

	item, errMarshal := attributevalue.MarshalMap(&body)
	if errMarshal != nil {
		return errMarshal
	}

	_, errPut := r.dynamo.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(r.options.table), Item: item,
	})

	return errPut
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/udhos/gateboard/awsconfig"
	"github.com/udhos/gateboard/gateboard"
)

type repoDynamoOptions struct {
	table        string
	region       string
	roleArn      string
	sessionName  string
	debug        bool
	manualCreate bool
}

type repoDynamo struct {
	options repoDynamoOptions
	dynamo  *dynamodb.Client
}

func newRepoDynamo(opt repoDynamoOptions) (*repoDynamo, error) {
	const me = "newRepoDynamo"

	cfg := awsconfig.AwsConfig(opt.region, opt.roleArn, opt.sessionName)

	r := &repoDynamo{
		options: opt,
		dynamo:  dynamodb.NewFromConfig(cfg),
	}

	if !r.options.manualCreate {
		r.createTable()
	}

	return r, nil
}

func (r *repoDynamo) createTable() {
	const me = "repoDynamo.createTable"

	//
	// Create table
	//

	input := &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("gateway_name"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("gateway_name"),
				KeyType:       types.KeyTypeHash,
			},
		},
		TableName: aws.String(r.options.table),
	}

	const onDemand = true

	if onDemand {
		input.BillingMode = types.BillingModePayPerRequest
	} else {
		input.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(10),
			WriteCapacityUnits: aws.Int64(10),
		}
	}

	output, errCreate := r.dynamo.CreateTable(context.TODO(), input)
	if errCreate != nil {
		log.Printf("%s: creating table '%s': error: %v", me, r.options.table, errCreate)
		return
	}

	log.Printf("%s: creating table: '%s': arn=%s status=%s", me, r.options.table, *output.TableDescription.TableArn, output.TableDescription.TableStatus)

	//
	// Waiting for table
	//

	log.Printf("%s: waiting for table '%s'", me, r.options.table)

	waiter := dynamodb.NewTableExistsWaiter(r.dynamo)
	errWait := waiter.Wait(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: aws.String(r.options.table)}, 5*time.Minute)
	if errWait != nil {
		log.Printf("%s: waiting for table '%s': error: %v", me, r.options.table, errWait)
		return
	}

	log.Printf("%s: waiting for table '%s': done", me, r.options.table)

	//
	// Refuse to run without table
	//

	const cooldown = 5 * time.Second
	const max = 10
	for i := 1; i <= max; i++ {
		log.Printf("%s: %d/%d table active? '%s'", me, i, max, r.options.table)
		active := r.tableActive()
		log.Printf("%s: %d/%d table active? '%s': %t", me, i, max, r.options.table, active)
		if active {
			log.Printf("%s: %d/%d table active? '%s': %t: done", me, i, max, r.options.table, active)
			return
		}
		log.Printf("%s: %d/%d table active? '%s': %t, sleeping for %v", me, i, max, r.options.table, active, cooldown)
		time.Sleep(cooldown)
	}
	log.Fatalf("%s: table '%s' is not active, ABORTING", me, r.options.table)
}

func (r *repoDynamo) tableActive() bool {
	const me = "tableActive"

	t, err := r.dynamo.DescribeTable(
		context.TODO(), &dynamodb.DescribeTableInput{TableName: aws.String(r.options.table)},
	)

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("%s: table '%s' does not exist", me, r.options.table)
		} else {
			log.Printf("%s: table '%s': error: %v", me, r.options.table, err)
		}
		return false
	}

	if r.options.debug {
		log.Printf("%s: table '%s' status=%s", me, r.options.table, t.Table.TableStatus)
	}

	return t.Table.TableStatus == types.TableStatusActive
}

func (r *repoDynamo) tableExists() bool {
	const me = "tableExists"

	t, err := r.dynamo.DescribeTable(
		context.TODO(), &dynamodb.DescribeTableInput{TableName: aws.String(r.options.table)},
	)

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("%s: table '%s' does not exist", me, r.options.table)
		} else {
			log.Printf("%s: table '%s': error: %v", me, r.options.table, err)
		}
		return false
	}

	if r.options.debug {
		log.Printf("%s: table '%s' status=%s", me, r.options.table, t.Table.TableStatus)
	}

	return true
}

func (r *repoDynamo) dropDatabase() error {
	const me = "repoDynamo.dropDatabase"

	_, err := r.dynamo.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
		TableName: aws.String(r.options.table)})
	if err != nil {
		return err
	}

	//
	// Refuse to run with table
	//

	const cooldown = 5 * time.Second
	const max = 10
	for i := 1; i <= max; i++ {
		log.Printf("%s: %d/%d table exists? '%s'", me, i, max, r.options.table)
		exists := r.tableExists()
		log.Printf("%s: %d/%d table exists? '%s': %t", me, i, max, r.options.table, exists)
		if !exists {
			log.Printf("%s: %d/%d table exists? '%s': %t: done", me, i, max, r.options.table, exists)
			return nil
		}
		log.Printf("%s: %d/%d table exists? '%s': %t, sleeping for %v", me, i, max, r.options.table, exists, cooldown)
		time.Sleep(cooldown)
	}
	log.Fatalf("%s: table '%s' exists, ABORTING", me, r.options.table)

	return fmt.Errorf("%s: table '%s' exists, ABORTING", me, r.options.table)
}

func (r *repoDynamo) dump() (repoDump, error) {
	const me = "repoDynamo.dump"

	list := repoDump{}

	//filtEx := expression.Name("year").Between(expression.Value(startYear), expression.Value(endYear))
	projEx := expression.NamesList(
		expression.Name("gateway_name"),
		expression.Name("gateway_id"),
		expression.Name("changes"),
		expression.Name("last_update"),
	)
	//expr, err := expression.NewBuilder().WithFilter(filtEx).WithProjection(projEx).Build()
	expr, errEx := expression.NewBuilder().WithProjection(projEx).Build()
	if errEx != nil {
		return list, errEx
	}

	response, errScan := r.dynamo.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName:                 aws.String(r.options.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		//FilterExpression:          expr.Filter(),
		ProjectionExpression: expr.Projection(),
	})
	if errScan != nil {
		return list, errScan
	}

	errUnmarshal := attributevalue.UnmarshalListOfMaps(response.Items, &list)
	if errUnmarshal != nil {
		return list, errUnmarshal
	}

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

func (r *repoDynamo) putToken(gatewayName, token string) error {
	update := expression.Set(expression.Name("token"), expression.Value(token))

	expr, errBuild := expression.NewBuilder().WithUpdate(update).Build()
	if errBuild != nil {
		return errBuild
	}

	av, errMarshal := attributevalue.Marshal(gatewayName)
	if errMarshal != nil {
		return errMarshal
	}

	key := map[string]types.AttributeValue{"gateway_name": av}

	_, errUpdate := r.dynamo.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName:                 aws.String(r.options.table),
		Key:                       key,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		ReturnValues:              types.ReturnValueUpdatedNew,
	})

	return errUpdate
}

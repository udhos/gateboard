package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/udhos/boilerplate/awsconfig"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/gateboard"
)

type repoDynamoOptions struct {
	metricRepoName string // kind:name
	table          string
	region         string
	roleArn        string
	sessionName    string
	debug          bool
	manualCreate   bool
}

type repoDynamo struct {
	options repoDynamoOptions
	dynamo  *dynamodb.Client
}

func newRepoDynamo(opt repoDynamoOptions) (*repoDynamo, error) {

	awsConfOptions := awsconfig.Options{
		Region:          opt.region,
		RoleArn:         opt.roleArn,
		RoleSessionName: opt.sessionName,
	}

	cfg, errAwsConfig := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConfig != nil {
		return nil, errAwsConfig
	}

	r := &repoDynamo{
		options: opt,
		dynamo:  dynamodb.NewFromConfig(cfg.AwsConfig),
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
		zlog.Errorf("%s: creating table '%s': error: %v", me, r.options.table, errCreate)
		return
	}

	zlog.Infof("%s: creating table: '%s': arn=%s status=%s", me, r.options.table, *output.TableDescription.TableArn, output.TableDescription.TableStatus)

	//
	// Waiting for table
	//

	zlog.Infof("%s: waiting for table '%s'", me, r.options.table)

	waiter := dynamodb.NewTableExistsWaiter(r.dynamo)
	errWait := waiter.Wait(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: aws.String(r.options.table)}, 5*time.Minute)
	if errWait != nil {
		zlog.Errorf("%s: waiting for table '%s': error: %v", me, r.options.table, errWait)
		return
	}

	zlog.Infof("%s: waiting for table '%s': done", me, r.options.table)

	//
	// Refuse to run without table
	//

	const cooldown = 5 * time.Second
	const max = 10
	for i := 1; i <= max; i++ {
		zlog.Infof("%s: %d/%d table active? '%s'", me, i, max, r.options.table)
		active := r.tableActive()
		zlog.Infof("%s: %d/%d table active? '%s': %t", me, i, max, r.options.table, active)
		if active {
			zlog.Infof("%s: %d/%d table active? '%s': %t: done", me, i, max, r.options.table, active)
			return
		}
		zlog.Infof("%s: %d/%d table active? '%s': %t, sleeping for %v", me, i, max, r.options.table, active, cooldown)
		time.Sleep(cooldown)
	}
	zlog.Fatalf("%s: table '%s' is not active, ABORTING", me, r.options.table)
}

func (r *repoDynamo) tableActive() bool {
	const me = "tableActive"

	t, err := r.dynamo.DescribeTable(
		context.TODO(), &dynamodb.DescribeTableInput{TableName: aws.String(r.options.table)},
	)

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			zlog.Infof("%s: table '%s' does not exist", me, r.options.table)
		} else {
			zlog.Errorf("%s: table '%s': error: %v", me, r.options.table, err)
		}
		return false
	}

	zlog.Debugf(r.options.debug, "%s: table '%s' status=%s", me, r.options.table, t.Table.TableStatus)

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
			zlog.Infof("%s: table '%s' does not exist", me, r.options.table)
		} else {
			zlog.Errorf("%s: table '%s': error: %v", me, r.options.table, err)
		}
		return false
	}

	zlog.Debugf(r.options.debug, "%s: table '%s' status=%s", me, r.options.table, t.Table.TableStatus)

	return true
}

func (r *repoDynamo) repoName() string {
	return r.options.metricRepoName
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
		zlog.Infof("%s: %d/%d table exists? '%s'", me, i, max, r.options.table)
		exists := r.tableExists()
		zlog.Infof("%s: %d/%d table exists? '%s': %t", me, i, max, r.options.table, exists)
		if !exists {
			zlog.Infof("%s: %d/%d table exists? '%s': %t: done", me, i, max, r.options.table, exists)
			return nil
		}
		zlog.Infof("%s: %d/%d table exists? '%s': %t, sleeping for %v", me, i, max, r.options.table, exists, cooldown)
		time.Sleep(cooldown)
	}
	zlog.Fatalf("%s: table '%s' exists, ABORTING", me, r.options.table)

	return fmt.Errorf("%s: table '%s' exists, ABORTING", me, r.options.table)
}

func (r *repoDynamo) dump(_ /*ctx*/ context.Context) (repoDump, error) {

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

func (r *repoDynamo) get(_ /*ctx*/ context.Context, gatewayName string) (gateboard.BodyGetReply, error) {

	var body gateboard.BodyGetReply

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return body, errVal
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

func (r *repoDynamo) put(_ /*ctx*/ context.Context, gatewayName, gatewayID string) error {
	const me = "repoDynamo.put"

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		return errVal
	}

	if strings.TrimSpace(gatewayID) == "" {
		return fmt.Errorf("%s: bad gateway id: '%s'", me, gatewayID)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(r.options.table),

		Key: map[string]types.AttributeValue{
			"gateway_name": &types.AttributeValueMemberS{Value: gatewayName},
		},

		UpdateExpression: aws.String("set gateway_id = :id, last_update = :now add changes :inc"),

		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id":  &types.AttributeValueMemberS{Value: gatewayID},
			":inc": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339Nano)},
		},

		ReturnValues: types.ReturnValueNone,
	}

	_, errUpdate := r.dynamo.UpdateItem(context.TODO(), input)

	return errUpdate
}

func (r *repoDynamo) putToken(_ /*ctx*/ context.Context, gatewayName, token string) error {
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

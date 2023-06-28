package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/udhos/boilerplate/awsconfig"
	"go.opentelemetry.io/otel/trace"
)

var awsConfigCache = map[string]cacheEntry{}

type cacheEntry struct {
	config    aws.Config
	accountID string
}

func awsConfig(region, roleArn, roleExternalID, roleSessionName string) (aws.Config, string, error) {
	const me = "awsConfig"

	key := fmt.Sprintf("%s,%s,%s,%s", region, roleArn, roleExternalID, roleSessionName)
	if cfg, found := awsConfigCache[key]; found {
		log.Printf("%s: key='%s' retrieved from cache", me, key)
		return cfg.config, cfg.accountID, nil
	}

	awsConfOptions := awsconfig.Options{
		Region:          region,
		RoleArn:         roleArn,
		RoleSessionName: roleSessionName,
		RoleExternalID:  roleExternalID,
	}
	awsConf, errAwsConf := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConf != nil {
		return awsConf.AwsConfig, awsConf.StsAccountID, errAwsConf
	}

	awsConfigCache[key] = cacheEntry{
		config:    awsConf.AwsConfig,
		accountID: awsConf.StsAccountID,
	}

	return awsConf.AwsConfig, awsConf.StsAccountID, errAwsConf
}

type scannerAWS struct {
	apiGatewayClient *apigateway.Client
	accountID        string
	region           string
	roleARN          string
}

func newScannerAWS(ctx context.Context, tracer trace.Tracer, region, roleArn, roleExternalID, roleSessionName string) (*scannerAWS, string) {

	const me = "newScannerAWS"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	cfg, accountID, errConfig := awsConfig(region, roleArn, roleExternalID, roleSessionName)
	if errConfig != nil {
		msg := fmt.Sprintf("%s: region=%s role=%s: %v", me, region, roleArn, errConfig)
		traceError(span, msg)
		return nil, ""
	}

	s := scannerAWS{
		apiGatewayClient: apigateway.NewFromConfig(cfg),
		accountID:        accountID,
		region:           region,
		roleARN:          roleArn,
	}

	log.Printf("%s: region=%s role=%s accountId=%s", me, region, roleArn, accountID)

	return &s, accountID
}

func (s *scannerAWS) list(ctx context.Context, tracer trace.Tracer) []item {

	const me = "scannerAWS.list"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	var limit int32 = 500 // max number of results per page. default=25, max=500

	input := apigateway.GetRestApisInput{Limit: &limit}
	paginator := apigateway.NewGetRestApisPaginator(s.apiGatewayClient, &input,
		func(o *apigateway.GetRestApisPaginatorOptions) {
			o.Limit = limit
			o.StopOnDuplicateToken = true
		})

	var page int
	var found int

	var array []item

	for paginator.HasMorePages() {

		page++

		ctx := context.TODO()
		output, errOut := paginator.NextPage(ctx, func(o *apigateway.Options) {
			o.Region = s.region
		})
		if errOut != nil {
			msg := fmt.Sprintf("%s: region=%s role=%s accountId=%s page=%d: error: %v",
				me, s.region, s.roleARN, s.accountID, page, errOut)
			log.Print(msg)
			traceError(span, msg)

			// abort this account/credential,
			// otherwise we might keep paginating a lot on a broken credential
			break
		}

		found += len(output.Items)

		log.Printf("%s: region=%s role=%s accountId=%s page=%d gateways_in_page: %d gateways_total: %d",
			me, s.region, s.roleARN, s.accountID, page, len(output.Items), found)

		for _, i := range output.Items {

			gatewayName := *i.Name
			gatewayID := *i.Id

			log.Printf("%s: region=%s role=%s accountId=%s page=%d name=%s ID=%s",
				me, s.region, s.roleARN, s.accountID, page, gatewayName, gatewayID)

			array = append(array, item{name: gatewayName, id: gatewayID})
		}
	}

	return array
}

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

	cfg, errConfig := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region))
	if errConfig != nil {
		log.Printf("%s: load config: %v", me, errConfig)
		return cfg, "", errConfig
	}

	if roleArn != "" {
		//
		// AssumeRole
		//
		log.Printf("%s: AssumeRole: arn: %s", me, roleArn)
		clientSts := sts.NewFromConfig(cfg)
		cfg2, errConfig2 := config.LoadDefaultConfig(
			context.TODO(), config.WithRegion(region),
			config.WithCredentialsProvider(aws.NewCredentialsCache(
				stscreds.NewAssumeRoleProvider(
					clientSts,
					roleArn,
					func(o *stscreds.AssumeRoleOptions) {
						o.RoleSessionName = roleSessionName
						if roleExternalID != "" {
							o.ExternalID = &roleExternalID
						}
					},
				)),
			),
		)
		if errConfig2 != nil {
			log.Printf("%s: AssumeRole %s: error: %v", me, roleArn, errConfig2)
			return cfg, "", errConfig
		}
		cfg = cfg2
	}

	var accountID string

	{
		//
		// show caller identity
		//
		clientSts := sts.NewFromConfig(cfg)
		input := sts.GetCallerIdentityInput{}
		respSts, errSts := clientSts.GetCallerIdentity(context.TODO(), &input)
		if errSts != nil {
			log.Printf("%s: GetCallerIdentity: error: %v", me, errSts)
		} else {
			accountID = *respSts.Account
			log.Printf("%s: GetCallerIdentity: Account=%s ARN=%s UserId=%s", me, *respSts.Account, *respSts.Arn, *respSts.UserId)
		}
	}

	awsConfigCache[key] = cacheEntry{
		config:    cfg,
		accountID: accountID,
	}

	return cfg, accountID, nil
}

type scannerAWS struct {
	apiGatewayClient *apigateway.Client
	accountID        string
	region           string
	roleARN          string
}

func newScannerAWS(region, roleArn, roleExternalID, roleSessionName string) (*scannerAWS, string) {

	const me = "newScannerAWS"

	cfg, accountID, errConfig := awsConfig(region, roleArn, roleExternalID, roleSessionName)
	if errConfig != nil {
		log.Fatalf("%s: region=%s role=%s: %v", me, region, roleArn, errConfig)
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

func (s *scannerAWS) list() []item {

	const me = "scannerAWS.list"

	var limit int32 = 500 // max number of results per page. default=25, max=500
	//table := map[string]gateway{}

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
			log.Printf("%s: region=%s role=%s accountId=%s page=%d: error: %v",
				me, s.region, s.roleARN, s.accountID, page, errOut)
			continue
		}

		found += len(output.Items)

		log.Printf("%s: region=%s role=%s accountId=%s page=%d gateways_in_page: %d gateways_total: %d",
			me, s.region, s.roleARN, s.accountID, page, len(output.Items), found)

		for _, i := range output.Items {

			gatewayName := *i.Name
			gatewayID := *i.Id

			log.Printf("%s: region=%s role=%s accountId=%s page=%d name=%s ID=%s",
				me, s.region, s.roleARN, s.accountID, page, gatewayName, gatewayID)

			//
			// add gateway to table
			//

			/*
				gw, found := table[gatewayName]
				if !found {
					gw = gateway{id: gatewayID}
				}
				gw.count++
				table[gatewayName] = gw
			*/

			array = append(array, item{name: gatewayName, id: gatewayID})
		}
	}

	/*
		var array []item

		for k, g := range table {
			if g.count != 1 {
				log.Printf("%s: region=%s role=%s accountId=%s IGNORING dup gateway=%s count=%d",
					me, s.region, s.roleARN, s.accountID, k, g.count)
				continue
			}
			array = append(array, item{name: k, id: g.id})
		}

		log.Printf("%s: region=%s role=%s accountId=%s gateways_unique: %d",
			me, s.region, s.roleARN, s.accountID, len(array))
	*/

	return array
}

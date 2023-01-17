package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/udhos/gateboard/gateboard"
	"gopkg.in/yaml.v2"
)

func awsConfig(region, roleArn, roleExternalID, roleSessionName string) (aws.Config, string) {
	const me = "awsConfig"

	cfg, errConfig := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region))
	if errConfig != nil {
		log.Fatalf("%s: load config: %v", me, errConfig)
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
			log.Fatalf("%s: AssumeRole %s: error: %v", me, roleArn, errConfig2)
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

	return cfg, accountID
}

type gateway struct {
	count int
	id    string
}

func findGateways(cred credential, roleSessionName, serverURL string, debug bool) {

	const me = "findGateways"

	log.Printf("%s: region=%s role=%s", me, cred.Region, cred.RoleArn)

	cfg, accountID := awsConfig(cred.Region, cred.RoleArn, cred.RoleExternalID, roleSessionName)

	log.Printf("%s: region=%s role=%s accountId=%s", me, cred.Region, cred.RoleArn, accountID)

	apiGatewayClient := apigateway.NewFromConfig(cfg)

	var limit int32 = 500 // max number of results per page. default=25, max=500
	input := apigateway.GetRestApisInput{Limit: &limit}

	paginator := apigateway.NewGetRestApisPaginator(apiGatewayClient, &input, func(o *apigateway.GetRestApisPaginatorOptions) {
		o.Limit = limit
		o.StopOnDuplicateToken = true
	})

	var count int

	table := map[string]gateway{}

	for paginator.HasMorePages() {
		ctx := context.TODO()
		output, errOut := paginator.NextPage(ctx, func(o *apigateway.Options) {
			o.Region = cred.Region
		})
		if errOut != nil {
			log.Printf("%s: region=%s role=%s accountId=%s: error: %v",
				me, cred.Region, cred.RoleArn, accountID, errOut)
		}

		for _, item := range output.Items {

			gatewayName := *item.Name
			gatewayID := *item.Id

			log.Printf("%s: region=%s role=%s accountId=%s name=%s ID=%s",
				me, cred.Region, cred.RoleArn, accountID, gatewayName, gatewayID)

			key := accountID + ":" + gatewayName
			gw, found := table[key]
			if !found {
				gw = gateway{id: gatewayID}
			}
			gw.count++
			table[key] = gw

			count++
		}
	}

	log.Printf("%s: region=%s role=%s accountId=%s found: %d",
		me, cred.Region, cred.RoleArn, accountID, count)

	for k, g := range table {
		if g.count != 1 {
			log.Printf("%s: region=%s role=%s accountId=%s IGNORING dup gateway=%s count=%d",
				me, cred.Region, cred.RoleArn, accountID, k, g.count)
			continue
		}
		saveGatewayID(serverURL, k, g.id, debug)
	}
}

func saveGatewayID(serverURL, gatewayName, gatewayID string, debug bool) {
	const me = "saveGatewayID"

	if debug {
		log.Printf("%s: URL=%s name=%s ID=%s", me, serverURL, gatewayName, gatewayID)
	}

	path, errPath := url.JoinPath(serverURL, gatewayName)
	if errPath != nil {
		log.Printf("%s: URL=%s join error: %v", me, path, errPath)
		return
	}

	requestBody := gateboard.BodyPutRequest{GatewayID: gatewayID}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		log.Printf("%s: URL=%s json error: %v", me, path, errJSON)
		return
	}

	req, errReq := http.NewRequest("PUT", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		log.Printf("%s: URL=%s request error: %v", me, path, errReq)
		return
	}

	client := http.DefaultClient
	resp, errDo := client.Do(req)
	if errDo != nil {
		log.Printf("%s: URL=%s server error: %v", me, path, errDo)
		return
	}

	defer resp.Body.Close()

	var reply gateboard.BodyPutReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		log.Printf("%s: URL=%s yaml error: %v", me, path, errYaml)
		return
	}

	if debug {
		log.Printf("%s: URL=%s gateway reply: %v", me, path, toJSON(reply))
	}
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJSON: %v", err)
	}
	return string(b)
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	yaml "gopkg.in/yaml.v3"
)

type clientConfig struct {
	sqs      *sqs.Client
	queueURL string
}

func initClient(caller, queueURL, roleArn, roleSessionName string) clientConfig {

	var c clientConfig

	region, errRegion := getRegion(queueURL)
	if errRegion != nil {
		log.Fatalf("%s initClient: error: %v", caller, errRegion)
		os.Exit(1)
		return c
	}

	cfg, errConfig := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region))
	if errConfig != nil {
		log.Fatalf("%s initClient: error: %v", caller, errConfig)
		os.Exit(1)
		return c
	}

	if roleArn != "" {
		//
		// AssumeRole
		//
		log.Printf("%s initClient: AssumeRole: arn: %s", caller, roleArn)
		clientSts := sts.NewFromConfig(cfg)
		cfg2, errConfig2 := config.LoadDefaultConfig(
			context.TODO(), config.WithRegion(region),
			config.WithCredentialsProvider(aws.NewCredentialsCache(
				stscreds.NewAssumeRoleProvider(
					clientSts,
					roleArn,
					func(o *stscreds.AssumeRoleOptions) {
						o.RoleSessionName = roleSessionName
					},
				)),
			),
		)
		if errConfig2 != nil {
			log.Fatalf("%s initClient: AssumeRole %s: error: %v", caller, roleArn, errConfig2)
			os.Exit(1)
			return c

		}
		cfg = cfg2
	}

	{
		// show caller identity
		clientSts := sts.NewFromConfig(cfg)
		input := sts.GetCallerIdentityInput{}
		respSts, errSts := clientSts.GetCallerIdentity(context.TODO(), &input)
		if errSts != nil {
			log.Printf("%s initClient: GetCallerIdentity: error: %v", caller, errSts)
		} else {
			log.Printf("%s initClient: GetCallerIdentity: Account=%s ARN=%s UserId=%s", caller, *respSts.Account, *respSts.Arn, *respSts.UserId)
		}
	}

	c = clientConfig{
		sqs:      sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}

	return c
}

// https://sqs.us-east-1.amazonaws.com/123456789012/myqueue
func getRegion(queueURL string) (string, error) {
	fields := strings.SplitN(queueURL, ".", 3)
	if len(fields) < 3 {
		return "", fmt.Errorf("queueRegion: bad queue url=[%s]", queueURL)
	}
	region := fields[1]
	log.Printf("queueRegion=[%s]", region)
	return region, nil
}

func sqsListener(app *application) {
	const me = "sqsListener"
	const waitTimeSeconds = 20 // 0..20
	const errorCooldown = time.Second * 10

	for {
		input := &sqs.ReceiveMessageInput{
			QueueUrl: &app.sqsClient.queueURL,
			AttributeNames: []types.QueueAttributeName{
				"SentTimestamp",
			},
			MaxNumberOfMessages: 10, // 1..10
			MessageAttributeNames: []string{
				"All",
			},
			WaitTimeSeconds: waitTimeSeconds,
		}

		resp, errRecv := app.sqsClient.sqs.ReceiveMessage(context.TODO(), input)
		if errRecv != nil {
			log.Printf("%s: ReceiveMessage: error: %v", me, errRecv)
			time.Sleep(errorCooldown)
			continue
		}

		count := len(resp.Messages)

		for i, msg := range resp.Messages {
			log.Printf("%s: %d/%d MessageId=%s body:%s", me, i+1, count, *msg.MessageId, *msg.Body)

			var put sqsPut

			errYaml := yaml.Unmarshal([]byte(*msg.Body), &put)
			if errYaml != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s yaml error: %v",
					me, put.GatewayName, put.GatewayID, *msg.MessageId, errYaml)
				continue
			}

			put.GatewayName = strings.TrimSpace(put.GatewayName)
			if put.GatewayName == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_name",
					me, put.GatewayName, *msg.MessageId, put.GatewayID)
				continue
			}

			put.GatewayID = strings.TrimSpace(put.GatewayID)
			if put.GatewayID == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_id",
					me, put.GatewayName, *msg.MessageId, put.GatewayID)
				continue
			}

			errPut := app.repo.put(put.GatewayName, put.GatewayID)
			if errPut != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s repo error: %v",
					me, put.GatewayName, put.GatewayID, *msg.MessageId, errPut)
				continue
			}

			sqsDeleteMessage(app, msg)
		}
	}
}

type sqsPut struct {
	GatewayName string `json:"gateway_name" yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"   yaml:"gateway_id"`
}

func sqsDeleteMessage(app *application, m types.Message) {
	const me = "sqsDeleteMessage"

	inputDelete := &sqs.DeleteMessageInput{
		QueueUrl:      &app.sqsClient.queueURL,
		ReceiptHandle: m.ReceiptHandle,
	}

	_, errDelete := app.sqsClient.sqs.DeleteMessage(context.TODO(), inputDelete)
	if errDelete != nil {
		log.Printf("%s: MessageId: %s - DeleteMessage: error: %v", me, *m.MessageId, errDelete)
	}
}

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

func (q *clientConfig) receive() ([]queueMessage, error) {

	const me = "clientConfig.receive"

	const waitTimeSeconds = 20 // 0..20

	input := &sqs.ReceiveMessageInput{
		QueueUrl: &q.queueURL,
		AttributeNames: []types.QueueAttributeName{
			"SentTimestamp",
		},
		MaxNumberOfMessages: 10, // 1..10
		MessageAttributeNames: []string{
			"All",
		},
		WaitTimeSeconds: waitTimeSeconds,
	}

	resp, errRecv := q.sqs.ReceiveMessage(context.TODO(), input)
	if errRecv != nil {
		log.Printf("%s: ReceiveMessage: error: %v", me, errRecv)
		return nil, errRecv
	}

	messages := make([]queueMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		messages = append(messages, &sqsMessage{message: m})
	}

	return messages, nil
}

func awsConfig(region, roleArn, roleSessionName string) aws.Config {
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
					},
				)),
			),
		)
		if errConfig2 != nil {
			log.Fatalf("%s: AssumeRole %s: error: %v", me, roleArn, errConfig2)
		}
		cfg = cfg2
	}

	{
		// show caller identity
		clientSts := sts.NewFromConfig(cfg)
		input := sts.GetCallerIdentityInput{}
		respSts, errSts := clientSts.GetCallerIdentity(context.TODO(), &input)
		if errSts != nil {
			log.Printf("%s: GetCallerIdentity: error: %v", me, errSts)
		} else {
			log.Printf("%s: GetCallerIdentity: Account=%s ARN=%s UserId=%s", me, *respSts.Account, *respSts.Arn, *respSts.UserId)
		}
	}

	return cfg
}

func initClient(caller, queueURL, roleArn, roleSessionName string) *clientConfig {

	region, errRegion := getRegion(queueURL)
	if errRegion != nil {
		log.Fatalf("%s initClient: error: %v", caller, errRegion)
		os.Exit(1)
		return nil
	}

	cfg := awsConfig(region, roleArn, roleSessionName)

	c := clientConfig{
		sqs:      sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}

	return &c
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
	const errorCooldown = time.Second * 10

	for {
		messages, errRecv := app.sqsClient.receive()
		if errRecv != nil {
			log.Printf("%s: receive: error: %v", me, errRecv)
			time.Sleep(errorCooldown)
			continue
		}
		count := len(messages)

		for i, msg := range messages {
			log.Printf("%s: %d/%d MessageId=%s body:%s", me, i+1, count, msg.id(), msg.body())

			var put sqsPut

			errYaml := yaml.Unmarshal([]byte(msg.body()), &put)
			if errYaml != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s yaml error: %v",
					me, put.GatewayName, put.GatewayID, msg.id(), errYaml)
				continue
			}

			put.GatewayName = strings.TrimSpace(put.GatewayName)
			if put.GatewayName == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_name",
					me, put.GatewayName, put.GatewayID, msg.id())
				continue
			}

			put.GatewayID = strings.TrimSpace(put.GatewayID)
			if put.GatewayID == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_id",
					me, put.GatewayName, put.GatewayID, msg.id())
				continue
			}

			//
			// check write token
			//

			if app.config.writeToken {
				if invalidToken(app, put.GatewayName, put.Token) {
					log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid token='%s'",
						me, put.GatewayName, put.GatewayID, msg.id(), put.Token)
					continue
				}
			}

			errPut := app.repo.put(put.GatewayName, put.GatewayID)
			if errPut != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s repo error: %v",
					me, put.GatewayName, put.GatewayID, msg.id(), errPut)
				continue
			}

			app.sqsClient.deleteMessage(msg)
		}
	}
}

type sqsPut struct {
	GatewayName string `json:"gateway_name" yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"   yaml:"gateway_id"`
	Token       string `json:"token"        yaml:"token"`
}

func (q *clientConfig) deleteMessage(m queueMessage) error {
	const me = "clientConfig.deleteMessage"

	msg := m.(*sqsMessage)

	inputDelete := &sqs.DeleteMessageInput{
		QueueUrl:      &q.queueURL,
		ReceiptHandle: msg.message.ReceiptHandle,
	}

	_, errDelete := q.sqs.DeleteMessage(context.TODO(), inputDelete)
	if errDelete != nil {
		log.Printf("%s: MessageId: %s - DeleteMessage: error: %v", me, m.id(), errDelete)
	}

	return errDelete
}

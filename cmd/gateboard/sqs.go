package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/udhos/boilerplate/awsconfig"
	yaml "gopkg.in/yaml.v3"
)

type clientConfig struct {
	sqs      *sqs.Client
	queueURL string
	cooldown time.Duration
}

func (q *clientConfig) errorCooldown() time.Duration {
	return q.cooldown
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

func initClient(caller, queueURL, roleArn, roleSessionName string) *clientConfig {

	region, errRegion := getRegion(queueURL)
	if errRegion != nil {
		log.Fatalf("%s initClient: error: %v", caller, errRegion)
		os.Exit(1)
		return nil
	}

	awsConfOptions := awsconfig.Options{
		Region:          region,
		RoleArn:         roleArn,
		RoleSessionName: roleSessionName,
	}

	cfg, errAwsConfig := awsconfig.AwsConfig(awsConfOptions)
	if errAwsConfig != nil {
		log.Fatalf("%s initClient: aws config error: %v", caller, errRegion)
		return nil
	}

	c := clientConfig{
		sqs:      sqs.NewFromConfig(cfg.AwsConfig),
		queueURL: queueURL,
		cooldown: 10 * time.Second,
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

	errorCooldown := app.sqsClient.errorCooldown()

	for {
		messages, errRecv := app.sqsClient.receive()
		if errRecv != nil {
			log.Printf("%s: receive: error: %v, sleeping %v", me, errRecv, errorCooldown)
			time.Sleep(errorCooldown)
			continue
		}
		count := len(messages)

		if count == 0 {
			// guard against hammering api on empty receives.
			// it should not happen on live aws api, but it might happen on simulated apis.
			log.Printf("%s: empty receive, sleeping %v", me, errorCooldown)
			time.Sleep(errorCooldown)
			continue
		}

		for i, msg := range messages {
			log.Printf("%s: %d/%d MessageId=%s body:%s", me, i+1, count, msg.id(), msg.body())

			var put sqsPut

			errYaml := yaml.Unmarshal([]byte(msg.body()), &put)
			if errYaml != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s yaml error: %v",
					me, put.GatewayName, put.GatewayID, msg.id(), errYaml)

				if app.config.sqsConsumeBadMessage {
					deleteMessage(app.sqsClient, msg, put.GatewayName, put.GatewayID)
				}

				continue
			}

			put.GatewayName = strings.TrimSpace(put.GatewayName)
			if put.GatewayName == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_name",
					me, put.GatewayName, put.GatewayID, msg.id())

				if app.config.sqsConsumeBadMessage {
					deleteMessage(app.sqsClient, msg, put.GatewayName, put.GatewayID)
				}

				continue
			}

			put.GatewayID = strings.TrimSpace(put.GatewayID)
			if put.GatewayID == "" {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid gateway_id",
					me, put.GatewayName, put.GatewayID, msg.id())

				if app.config.sqsConsumeBadMessage {
					deleteMessage(app.sqsClient, msg, put.GatewayName, put.GatewayID)
				}

				continue
			}

			//
			// check write token
			//

			if app.config.writeToken {
				if invalidToken(context.TODO(), app, put.GatewayName, put.Token) {
					log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s invalid token='%s'",
						me, put.GatewayName, put.GatewayID, msg.id(), put.Token)

					if app.config.sqsConsumeInvalidToken {
						deleteMessage(app.sqsClient, msg, put.GatewayName, put.GatewayID)
					}

					continue
				}
			}

			errPut := app.repo.put(put.GatewayName, put.GatewayID)
			if errPut != nil {
				log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s repo error: %v",
					me, put.GatewayName, put.GatewayID, msg.id(), errPut)
				continue
			}

			deleteMessage(app.sqsClient, msg, put.GatewayName, put.GatewayID)
		}
	}
}

// deleteMessage consumes a message from SQS queue.
func deleteMessage(sqsClient queue, msg queueMessage, gatewayName, gatewayID string) {
	const me = "deleteMessage"
	err := sqsClient.deleteMessage(msg)
	if err != nil {
		log.Printf("%s: gateway_name=[%s] gateway_id=[%s] MessageId=%s repo error: %v",
			me, gatewayName, gatewayID, msg.id(), err)
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

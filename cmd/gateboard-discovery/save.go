package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"gopkg.in/yaml.v2"

	"github.com/udhos/gateboard/gateboard"
)

type saver interface {
	save(name, id string, debug bool) error
}

//
// save on server
//

type saverServer struct {
	serverURL string
}

func newSaverServer(serverURL string) *saverServer {
	s := saverServer{serverURL: serverURL}
	return &s
}

func (s *saverServer) save(name, id string, debug bool) error {
	const me = "saverServer.save"

	path, errPath := url.JoinPath(s.serverURL, name)
	if errPath != nil {
		return errPath
	}

	requestBody := gateboard.BodyPutRequest{GatewayID: id}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		return errJSON
	}

	req, errReq := http.NewRequest("PUT", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		return errReq
	}

	client := http.DefaultClient
	resp, errDo := client.Do(req)
	if errDo != nil {
		return errDo
	}

	defer resp.Body.Close()

	var reply gateboard.BodyPutReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		return errYaml
	}

	if debug {
		log.Printf("%s: gateboard URL=%s reply: status=%d: %v",
			me, path, resp.StatusCode, toJSON(reply))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: gateboard URL=%s bad status=%d: %v",
			me, path, resp.StatusCode, toJSON(reply))
	}

	return nil
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJSON: %v", err)
	}
	return string(b)
}

type saveBody struct {
	GatewayName string `json:"gateway_name"`
	GatewayID   string `json:"gateway_id"`
}

func bodyJSON(name, id string) ([]byte, error) {
	requestBody := saveBody{
		GatewayName: name,
		GatewayID:   id,
	}
	return json.Marshal(&requestBody)
}

//
// save on webhook
//

type saverWebhook struct {
	serverURL string
	token     string
}

func newSaverWebhook(serverURL, token string) *saverWebhook {
	s := saverWebhook{serverURL: serverURL, token: token}
	return &s
}

func (s *saverWebhook) save(name, id string, debug bool) error {

	const me = "saverWebhook.save"

	path := s.serverURL

	requestBytes, errJSON := bodyJSON(name, id)
	if errJSON != nil {
		return errJSON
	}

	req, errReq := http.NewRequest("POST", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		return errReq
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	resp, errDo := client.Do(req)
	if errDo != nil {
		return errDo
	}

	defer resp.Body.Close()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return errRead
	}

	if debug {
		log.Printf("%s: webhook URL=%s reply: status=%d: %v",
			me, path, resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: webhook URL=%s bad status=%d: %v",
			me, path, resp.StatusCode, string(body))
	}

	return nil
}

//
// save on sqs
//

type saverSQS struct {
	queueURL        string
	roleARN         string
	roleExternalID  string
	roleSessionName string
}

func newSaverSQS(queueURL, roleARN, roleExternalID, roleSessionName string) *saverSQS {
	s := saverSQS{
		queueURL:        queueURL,
		roleARN:         roleARN,
		roleExternalID:  roleExternalID,
		roleSessionName: roleSessionName,
	}
	return &s
}

// https://sqs.us-east-1.amazonaws.com/123456789012/myqueue
func getRegion(queueURL string) (string, error) {
	const me = "getRegion"
	fields := strings.SplitN(queueURL, ".", 3)
	if len(fields) < 3 {
		return "", fmt.Errorf("%s: bad queue url=[%s]", me, queueURL)
	}
	region := fields[1]
	log.Printf("%s=[%s]", me, region)
	return region, nil
}

func (s *saverSQS) save(name, id string, debug bool) error {

	const me = "saverSQS.save"

	region, errRegion := getRegion(s.queueURL)
	if errRegion != nil {
		return fmt.Errorf("%s: region error: %v", me, errRegion)
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		return fmt.Errorf("%s: aws config error: %v", me, errConfig)
	}

	requestBytes, errJSON := bodyJSON(name, id)
	if errJSON != nil {
		return errJSON
	}

	message := string(requestBytes)

	input := &sqs.SendMessageInput{
		QueueUrl:     &s.queueURL,
		DelaySeconds: 0, // 0..900
		MessageBody:  &message,
	}

	clientSQS := sqs.NewFromConfig(cfg)

	resp, errSend := clientSQS.SendMessage(context.TODO(), input)
	if errSend != nil {
		return fmt.Errorf("%s: SendMessage error: %v", me, errSend)
	}

	if debug {
		log.Printf("%s: SendMessage MessageId: %s", me, *resp.MessageId)
	}

	return nil
}

//
// save on sns
//

type saverSNS struct {
	topicARN        string
	roleARN         string
	roleExternalID  string
	roleSessionName string
}

func newSaverSNS(topicARN, roleARN, roleExternalID, roleSessionName string) *saverSNS {
	s := saverSNS{
		topicARN:        topicARN,
		roleARN:         roleARN,
		roleExternalID:  roleExternalID,
		roleSessionName: roleSessionName,
	}
	return &s
}

// arn:aws:sns:us-east-1:123456789012:gateboard
// arn:aws:lambda:us-east-1:123456789012:function:forward_to_sqs
func getARNRegion(arn string) (string, error) {
	const me = "getARNRegion"
	fields := strings.SplitN(arn, ":", 5)
	if len(fields) < 5 {
		return "", fmt.Errorf("%s: bad ARN=[%s]", me, arn)
	}
	region := fields[3]
	log.Printf("%s=[%s]", me, region)
	return region, nil
}

// arn:aws:lambda:us-east-1:123456789012:function:forward_to_sqs
func getARNFunctionName(arn string) (string, error) {
	const me = "getARNFunctionName"
	fields := strings.SplitN(arn, ":", 7)
	if len(fields) < 7 {
		return "", fmt.Errorf("%s: bad ARN=[%s]", me, arn)
	}
	funcName := fields[6]
	log.Printf("%s=[%s]", me, funcName)
	return funcName, nil
}

func (s *saverSNS) save(name, id string, debug bool) error {

	const me = "saverSNS.save"

	region, errRegion := getARNRegion(s.topicARN)
	if errRegion != nil {
		return errRegion
	}

	if debug {
		log.Printf("%s: region=%s topicARN=%s roleARN=%s",
			me, region, s.topicARN, s.roleARN)
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		return fmt.Errorf("%s: aws config error: %v", me, errConfig)
	}

	requestBytes, errJSON := bodyJSON(name, id)
	if errJSON != nil {
		return errJSON
	}

	message := string(requestBytes)

	input := &sns.PublishInput{
		Message:  &message,
		TopicArn: &s.topicARN,
	}

	clientSNS := sns.NewFromConfig(cfg)

	resp, errPublish := clientSNS.Publish(context.TODO(), input)
	if errPublish != nil {
		return fmt.Errorf("%s: Publish error: %v", me, errPublish)
	}

	if debug {
		log.Printf("%s: Publish MessageId: %s", me, *resp.MessageId)
	}

	return nil
}

//
// save on lambda
//

type saverLambda struct {
	lambdaARN       string
	roleARN         string
	roleExternalID  string
	roleSessionName string
}

func newSaverLambda(lambdaARN, roleARN, roleExternalID, roleSessionName string) *saverLambda {
	s := saverLambda{
		lambdaARN:       lambdaARN,
		roleARN:         roleARN,
		roleExternalID:  roleExternalID,
		roleSessionName: roleSessionName,
	}
	return &s
}

func (s *saverLambda) save(name, id string, debug bool) error {

	const me = "saverLambda.save"

	region, errRegion := getARNRegion(s.lambdaARN)
	if errRegion != nil {
		return errRegion
	}

	functionName, errFuncName := getARNFunctionName(s.lambdaARN)
	if errFuncName != nil {
		return errFuncName
	}

	if debug {
		log.Printf("%s: region=%s lambdaARN=%s roleARN=%s",
			me, region, s.lambdaARN, s.roleARN)
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		return fmt.Errorf("%s: aws config error: %v", me, errConfig)
	}

	requestBytes, errJSON := bodyJSON(name, id)
	if errJSON != nil {
		return errJSON
	}

	input := &lambda.InvokeInput{
		FunctionName: &functionName,
		Payload:      requestBytes,
	}

	clientLambda := lambda.NewFromConfig(cfg)

	resp, errInvoke := clientLambda.Invoke(context.TODO(), input)
	if errInvoke != nil {
		return fmt.Errorf("%s: Invoke error: %v", me, errInvoke)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: Invoke ARN=%s bad status=%d payload: %s",
			me, s.lambdaARN, resp.StatusCode, resp.Payload)
	}

	var funcError string
	if resp.FunctionError != nil {
		funcError = *resp.FunctionError
	}
	if funcError != "" {
		return fmt.Errorf("%s: Invoke ARN=%s function_error='%s' payload: %s",
			me, s.lambdaARN, funcError, resp.Payload)
	}

	if debug {
		log.Printf("%s: Invoke ARN=%s function_error='%s' payload: %s",
			me, s.lambdaARN, funcError, resp.Payload)
	}

	return nil
}

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
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"

	"github.com/udhos/gateboard/gateboard"
)

type saver interface {
	save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error
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

func (s *saverServer) save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error {
	const me = "saverServer.save"

	ctxNew, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	path, errPath := url.JoinPath(s.serverURL, name)
	if errPath != nil {
		traceError(span, errPath.Error())
		return errPath
	}

	requestBody := gateboard.BodyPutRequest{GatewayID: id, Token: writeToken}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		traceError(span, errJSON.Error())
		return errJSON
	}

	req, errReq := http.NewRequestWithContext(ctxNew, "PUT", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		traceError(span, errReq.Error())
		return errReq
	}

	client := httpClient()

	resp, errDo := client.Do(req)
	if errDo != nil {
		traceError(span, errDo.Error())
		return errDo
	}

	defer resp.Body.Close()

	var reply gateboard.BodyPutReply

	dec := yaml.NewDecoder(resp.Body)
	errYaml := dec.Decode(&reply)
	if errYaml != nil {
		traceError(span, errYaml.Error())
		return errYaml
	}

	if debug {
		log.Printf("%s: gateboard URL=%s reply: status=%d: %v",
			me, path, resp.StatusCode, toJSON(reply))
	}

	if resp.StatusCode != http.StatusOK {
		errStatus := fmt.Errorf("%s: gateboard URL=%s bad status=%d: %v",
			me, path, resp.StatusCode, toJSON(reply))
		traceError(span, errStatus.Error())
		return errStatus
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
	WriteToken  string `json:"token,omitempty"`
}

func bodyJSON(name, id, writeToken string) ([]byte, error) {
	requestBody := saveBody{
		GatewayName: name,
		GatewayID:   id,
		WriteToken:  writeToken,
	}
	return json.Marshal(&requestBody)
}

//
// save on webhook
//

type saverWebhook struct {
	serverURL   string
	bearerToken string
	method      string
}

func newSaverWebhook(serverURL, token, method string) *saverWebhook {
	s := saverWebhook{serverURL: serverURL, bearerToken: token, method: method}
	return &s
}

func (s *saverWebhook) save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error {

	const me = "saverWebhook.save"

	ctxNew, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	path := s.serverURL

	requestBytes, errJSON := bodyJSON(name, id, writeToken)
	if errJSON != nil {
		traceError(span, errJSON.Error())
		return errJSON
	}

	req, errReq := http.NewRequestWithContext(ctxNew, s.method, path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		traceError(span, errReq.Error())
		return errReq
	}

	req.Header.Set("Authorization", "Bearer "+s.bearerToken)
	req.Header.Set("Content-Type", "application/json")

	client := httpClient()

	resp, errDo := client.Do(req)
	if errDo != nil {
		traceError(span, errDo.Error())
		return errDo
	}

	defer resp.Body.Close()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		traceError(span, errRead.Error())
		return errRead
	}

	if debug {
		log.Printf("%s: webhook URL=%s reply: status=%d: %v",
			me, path, resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		errStatus := fmt.Errorf("%s: webhook URL=%s bad status=%d: %v",
			me, path, resp.StatusCode, string(body))
		traceError(span, errStatus.Error())
		return errStatus
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

func (s *saverSQS) save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error {

	const me = "saverSQS.save"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	region, errRegion := getRegion(s.queueURL)
	if errRegion != nil {
		traceError(span, errRegion.Error())
		return errRegion
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		traceError(span, errConfig.Error())
		return errConfig
	}

	requestBytes, errJSON := bodyJSON(name, id, writeToken)
	if errJSON != nil {
		traceError(span, errJSON.Error())
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
		traceError(span, errSend.Error())
		return errSend
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

func (s *saverSNS) save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error {

	const me = "saverSNS.save"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	region, errRegion := getARNRegion(s.topicARN)
	if errRegion != nil {
		traceError(span, errRegion.Error())
		return errRegion
	}

	if debug {
		log.Printf("%s: region=%s topicARN=%s roleARN=%s",
			me, region, s.topicARN, s.roleARN)
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		traceError(span, errConfig.Error())
		return errConfig
	}

	requestBytes, errJSON := bodyJSON(name, id, writeToken)
	if errJSON != nil {
		traceError(span, errJSON.Error())
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
		traceError(span, errPublish.Error())
		return errPublish
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

func (s *saverLambda) save(ctx context.Context, tracer trace.Tracer, name, id, writeToken string, debug bool) error {

	const me = "saverLambda.save"

	_, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	region, errRegion := getARNRegion(s.lambdaARN)
	if errRegion != nil {
		traceError(span, errRegion.Error())
		return errRegion
	}

	functionName, errFuncName := getARNFunctionName(s.lambdaARN)
	if errFuncName != nil {
		traceError(span, errFuncName.Error())
		return errFuncName
	}

	if debug {
		log.Printf("%s: region=%s lambdaARN=%s roleARN=%s",
			me, region, s.lambdaARN, s.roleARN)
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		traceError(span, errConfig.Error())
		return errConfig
	}

	requestBytes, errJSON := bodyJSON(name, id, writeToken)
	if errJSON != nil {
		traceError(span, errJSON.Error())
		return errJSON
	}

	input := &lambda.InvokeInput{
		FunctionName: &functionName,
		Payload:      requestBytes,
	}

	clientLambda := lambda.NewFromConfig(cfg)

	resp, errInvoke := clientLambda.Invoke(context.TODO(), input)
	if errInvoke != nil {
		traceError(span, errInvoke.Error())
		return errInvoke
	}

	if resp.StatusCode != http.StatusOK {
		errStatus := fmt.Errorf("%s: Invoke ARN=%s bad status=%d payload: %s",
			me, s.lambdaARN, resp.StatusCode, resp.Payload)
		traceError(span, errStatus.Error())
		return errStatus
	}

	var funcError string
	if resp.FunctionError != nil {
		funcError = *resp.FunctionError
	}
	if funcError != "" {
		errFunc := fmt.Errorf("%s: Invoke ARN=%s function_error='%s' payload: %s",
			me, s.lambdaARN, funcError, resp.Payload)
		traceError(span, errFunc.Error())
		return errFunc
	}

	if debug {
		log.Printf("%s: Invoke ARN=%s function_error='%s' payload: %s",
			me, s.lambdaARN, funcError, resp.Payload)
	}

	return nil
}

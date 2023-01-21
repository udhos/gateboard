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

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/udhos/gateboard/gateboard"
	"gopkg.in/yaml.v2"
)

type saver interface {
	save(name, id string, debug bool)
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

func (s *saverServer) save(name, id string, debug bool) {
	saveGatewayID(name, id, s.serverURL, debug)
}

func saveGatewayID(gatewayName, gatewayID, serverURL string, debug bool) {
	const me = "saveGatewayID"

	/*
		if dryRun {
			log.Printf("%s: URL=%s name=%s ID=%s dry=%t",
				me, serverURL, gatewayName, gatewayID, dryRun)
		}

		if dryRun {
			log.Printf("%s: running in DRY mode, refusing to update server", me)
			return
		}
	*/

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

	log.Printf("%s: URL=%s gateway reply: status=%d: %v",
		me, path, resp.StatusCode, toJSON(reply))
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJSON: %v", err)
	}
	return string(b)
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

func (s *saverWebhook) save(name, id string, debug bool) {

	const me = "saverWebhook.save"

	path := s.serverURL

	/*
		if dryRun {
			log.Printf("%s: URL=%s name=%s ID=%s dry=%t",
				me, path, name, id, dryRun)
		}

		if dryRun {
			log.Printf("%s: running in DRY mode, refusing to call webhook", me)
			return
		}
	*/

	type Body struct {
		GatewayName string `json:"gateway_name"`
		GatewayID   string `json:"gateway_id"`
	}

	requestBody := Body{GatewayID: id, GatewayName: name}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		log.Printf("%s: URL=%s json error: %v", me, path, errJSON)
		return
	}

	req, errReq := http.NewRequest("POST", path, bytes.NewBuffer(requestBytes))
	if errReq != nil {
		log.Printf("%s: URL=%s request error: %v", me, path, errReq)
		return
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	resp, errDo := client.Do(req)
	if errDo != nil {
		log.Printf("%s: URL=%s server error: %v", me, path, errDo)
		return
	}

	defer resp.Body.Close()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		log.Printf("%s: URL=%s gateway reply error: status=%d: %v",
			me, path, resp.StatusCode, errRead)
		return
	}

	log.Printf("%s: URL=%s gateway reply: status=%d: %v",
		me, path, resp.StatusCode, string(body))
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

func (s *saverSQS) save(name, id string, debug bool) {

	const me = "saverSQS.save"

	region, errRegion := getRegion(s.queueURL)
	if errRegion != nil {
		log.Printf("%s: region error: %v", me, errRegion)
		return
	}

	cfg, _, errConfig := awsConfig(region, s.roleARN, s.roleExternalID, s.roleSessionName)
	if errConfig != nil {
		log.Printf("%s: aws config error: %v", me, errConfig)
		return
	}

	type Body struct {
		GatewayName string `json:"gateway_name"`
		GatewayID   string `json:"gateway_id"`
	}

	requestBody := Body{GatewayID: id, GatewayName: name}
	requestBytes, errJSON := json.Marshal(&requestBody)
	if errJSON != nil {
		log.Printf("%s: URL=%s json error: %v", me, s.queueURL, errJSON)
		return
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
		log.Printf("%s: SendMessage error: %v", me, errSend)
		return
	}

	log.Printf("%s: SendMessage MessageId: %s", me, *resp.MessageId)
}

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/udhos/gateboard/gateboard"
	"gopkg.in/yaml.v2"
)

type saver interface {
	save(name, id string, debug, dryRun bool)
}

type saverServer struct {
	serverURL string
}

func newSaverServer(serverURL string) *saverServer {
	s := saverServer{serverURL: serverURL}
	return &s
}

func (s *saverServer) save(name, id string, debug, dryRun bool) {
	saveGatewayID(name, id, s.serverURL, debug, dryRun)
}

func saveGatewayID(gatewayName, gatewayID, serverURL string, debug, dryRun bool) {
	const me = "saveGatewayID"

	if dryRun {
		log.Printf("%s: URL=%s name=%s ID=%s dry=%t",
			me, serverURL, gatewayName, gatewayID, dryRun)
	}

	if dryRun {
		log.Printf("%s: running in DRY mode, refusing to update server", me)
		return
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

type saverWebhook struct {
	serverURL string
	token     string
}

func newSaverWebhook(serverURL, token string) *saverWebhook {
	s := saverWebhook{serverURL: serverURL, token: token}
	return &s
}

func (s *saverWebhook) save(name, id string, debug, dryRun bool) {

	const me = "saverWebhook.save"

	path := s.serverURL

	if dryRun {
		log.Printf("%s: URL=%s name=%s ID=%s dry=%t",
			me, path, name, id, dryRun)
	}

	if dryRun {
		log.Printf("%s: running in DRY mode, refusing to call webhook", me)
		return
	}

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

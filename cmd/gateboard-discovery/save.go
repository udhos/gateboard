package main

import (
	"bytes"
	"encoding/json"
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

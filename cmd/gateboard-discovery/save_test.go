package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/udhos/gateboard/gateboard"
	"gopkg.in/yaml.v2"
)

const (
	expectOk    = true
	expectError = false
)

type testCaseSaverServer struct {
	name               string
	gatewayName        string
	gatewayID          string
	writeToken         string
	requiredWriteToken string
	expectResult       bool
}

var testTableSaverServer = []testCaseSaverServer{
	{"empty gateway name and id", "", "", "", "", expectError},
	{"empty gateway name", "", "id1", "", "", expectError},
	{"empty gateway id", "gate1", "", "", "", expectError},
	{"simple save success", "gate1", "id1", "", "", expectOk},
	{"good token", "gate1", "id1", "good_token", "good_token", expectOk},
	{"bad token", "gate1", "id1", "bad_token", "good_token", expectError},
}

// go test -v -run TestSaverServer ./cmd/gateboard-discovery
func TestSaverServer(t *testing.T) {
	for _, data := range testTableSaverServer {
		testSaverServer(t, data)
	}
}

func sendError(t *testing.T, caller string, w http.ResponseWriter, status int, out gateboard.BodyPutReply) {
	buf, errJSON := json.Marshal(&out)
	if errJSON != nil {
		t.Logf("%s: json error:%v", caller, errJSON)
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, errJSON), http.StatusInternalServerError)
		return
	}
	http.Error(w, string(buf), status)
}

func testSaverServer(t *testing.T, data testCaseSaverServer) {

	const me = "testSaverServer"

	//
	// create mock server
	//
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var out gateboard.BodyPutReply

		if r.Method != "PUT" {
			t.Logf("%s: %s: non-PUT method: %s", me, data.name, r.Method)
			out.Error = "non-PUT method"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		gatewayName := strings.TrimPrefix(r.URL.Path, "/gateboard/")
		if gatewayName != data.gatewayName {
			t.Logf("%s: %s: bad request path: %s", me, data.name, r.URL.Path)
			out.Error = "bad request path"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if gatewayName == "" {
			t.Logf("%s: %s: empty gateway name", me, data.name)
			out.Error = "empty gateway name"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		buf, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Logf("%s: %s: body error: %s", me, data.name, errRead)
			out.Error = "body error"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		var body gateboard.BodyPutRequest

		errYaml := yaml.Unmarshal(buf, &body)
		if errYaml != nil {
			t.Logf("%s: %s: body yaml error: %s", me, data.name, errYaml)
			out.Error = "body yaml error"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayID == "" {
			t.Logf("%s: %s: empty gateway id", me, data.name)
			out.Error = "empty gateway id"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayID != data.gatewayID {
			t.Logf("%s: %s: wrong gateway id: expected=%s got=%s",
				me, data.name, data.gatewayID, body.GatewayID)
			out.Error = "wrong gateway id"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.Token != data.requiredWriteToken {
			t.Logf("%s: %s: wrong write token: expected=%s got=%s",
				me, data.name, data.requiredWriteToken, body.Token)
			out.Error = "wrong write token"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		sendError(t, me, w, 200, out)
	}))
	defer ts.Close()

	//
	// save into mock server
	//
	u, errJoin := url.JoinPath(ts.URL, "/gateboard")
	if errJoin != nil {
		t.Errorf("%s: url join error: %v", me, errJoin)
		return
	}

	saver := newSaverServer(u)

	const debug = true

	errSave := saver.save(context.TODO(), nil, data.gatewayName, data.gatewayID, data.writeToken, debug)

	ok := errSave == nil

	if ok != data.expectResult {
		t.Errorf("%s: %s: expecting sucess=%t got=%t error=%v",
			me, data.name, data.expectResult, ok, errSave)
	}
}

type testCaseSaverWebhook struct {
	name                string
	gatewayName         string
	gatewayID           string
	writeToken          string
	requiredWriteToken  string
	method              string
	bearerToken         string
	requiredBearerToken string
	expectResult        bool
}

var testTableSaverWebhook = []testCaseSaverWebhook{
	{"empty gateway name and id", "", "", "", "", "POST", "", "", expectError},
	{"empty gateway name", "", "id1", "", "", "POST", "", "", expectError},
	{"empty gateway id", "gate1", "", "", "", "POST", "", "", expectError},
	{"simple save success PUT", "gate1", "id1", "", "", "PUT", "", "", expectOk},
	{"simple save success POST", "gate1", "id1", "", "", "POST", "", "", expectOk},
	{"simple save success KKKK", "gate1", "id1", "", "", "KKKK", "", "", expectOk},
	{"good write token, good bearer token", "gate1", "id1", "good_write_token", "good_write_token", "POST", "good_bearer_token", "good_bearer_token", expectOk},
	{"bad write token, good bearer token", "gate1", "id1", "bad_write_token", "good_write_token", "POST", "good_bearer_token", "good_bearer_token", expectError},
	{"good write token, bad bearer token", "gate1", "id1", "good_write_token", "good_write_token", "POST", "bad_bearer_token", "good_bearer_token", expectError},
	{"bad write token, bad bearer token", "gate1", "id1", "bad_write_token", "good_write_token", "POST", "bad_bearer_token", "good_bearer_token", expectError},
}

// go test -v -run TestSaverWebhook ./cmd/gateboard-discovery
func TestSaverWebhook(t *testing.T) {
	for _, data := range testTableSaverWebhook {
		testSaverWebhook(t, data)
	}
}

func testSaverWebhook(t *testing.T, data testCaseSaverWebhook) {

	const me = "testSaverWebhook"

	//
	// create mock server
	//
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var out gateboard.BodyPutReply

		if r.Method != data.method {
			t.Logf("%s: %s: wrong method: expected=%s got=%s", me, data.name, data.method, r.Method)
			out.Error = "wrong method"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if r.URL.Path != "/gateboard" {
			t.Logf("%s: %s: bad request path: %s", me, data.name, r.URL.Path)
			out.Error = "bad request path"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		auth := r.Header.Get("authorization")
		bearerToken := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer"))
		if bearerToken != data.requiredBearerToken {
			t.Logf("%s: %s: wrong bearer token: expected='%s' got='%s' header='%s'",
				me, data.name, data.requiredBearerToken, bearerToken, auth)
			out.Error = "wrong bearer token"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		buf, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Logf("%s: %s: body error: %s", me, data.name, errRead)
			out.Error = "body error"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		var body bodyReq

		errYaml := yaml.Unmarshal(buf, &body)
		if errYaml != nil {
			t.Logf("%s: %s: body yaml error: %s", me, data.name, errYaml)
			out.Error = "body yaml error"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayName == "" {
			t.Logf("%s: %s: empty gateway name", me, data.name)
			out.Error = "empty gateway name"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayID == "" {
			t.Logf("%s: %s: empty gateway id", me, data.name)
			out.Error = "empty gateway id"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayName != data.gatewayName {
			t.Logf("%s: %s: wrong gateway name: expected=%s got=%s",
				me, data.name, data.gatewayName, body.GatewayName)
			out.Error = "wrong gateway name"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.GatewayID != data.gatewayID {
			t.Logf("%s: %s: wrong gateway id: expected=%s got=%s",
				me, data.name, data.gatewayID, body.GatewayID)
			out.Error = "wrong gateway id"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		if body.Token != data.requiredWriteToken {
			t.Logf("%s: %s: wrong write token: expected=%s got=%s",
				me, data.name, data.requiredWriteToken, body.Token)
			out.Error = "wrong write token"
			sendError(t, me, w, http.StatusBadRequest, out)
			return
		}

		sendError(t, me, w, 200, out)
	}))
	defer ts.Close()

	//
	// save into mock server
	//
	u, errJoin := url.JoinPath(ts.URL, "/gateboard")
	if errJoin != nil {
		t.Errorf("%s: url join error: %v", me, errJoin)
		return
	}

	saver := newSaverWebhook(u, data.bearerToken, data.method)

	const debug = true

	errSave := saver.save(context.TODO(), nil, data.gatewayName, data.gatewayID, data.writeToken, debug)

	ok := errSave == nil

	if ok != data.expectResult {
		t.Errorf("%s: %s: expecting sucess=%t got=%t error=%v",
			me, data.name, data.expectResult, ok, errSave)
	}
}

type bodyReq struct {
	GatewayName string `json:"gateway_name" yaml:"gateway_name"`
	GatewayID   string `json:"gateway_id"   yaml:"gateway_id"`
	Token       string `json:"token"        yaml:"token"`
}

// go test -v -run TestSaverSQS ./cmd/gateboard-discovery
func TestSaverSQS(t *testing.T) {
	const me = "TestSaverSQS"

	const queueURL = "bogus_queueURL"
	const queueRoleArn = "bogus_queueRoleArn"
	const queueRoleExternalID = "bogus_queueRoleExternalID"

	mock := &mockSqs{}

	newSqsClientMock := func(queueURL, roleArn, roleSessionName, roleExternalID string) (sqsClient, error) {
		return mock, nil
	}

	saver := newSaverSQS(queueURL, queueRoleArn, queueRoleExternalID, me, newSqsClientMock)

	const debug = true

	errSave := saver.save(context.TODO(), nil, "gate1", "id1", "", debug)
	if errSave != nil {
		t.Errorf("%s: save error: %v", me, errSave)
	}

	var body bodyReq

	errYaml := yaml.Unmarshal([]byte(mock.body), &body)
	if errYaml != nil {
		t.Errorf("%s: body yaml error: %s", me, errYaml)
	}

	if body.GatewayName != "gate1" {
		t.Errorf("%s: wrong gateway name: expect=gate1 got=%s", me, body.GatewayName)
	}

	if body.GatewayID != "id1" {
		t.Errorf("%s: wrong gateway id: expect=id1 got=%s", me, body.GatewayID)
	}
}

type mockSqs struct {
	body string
}

func (s *mockSqs) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	s.body = *params.MessageBody
	messageID := "mockSqs.fake-message-id"
	out := &sqs.SendMessageOutput{MessageId: aws.String(messageID)}
	return out, nil
}

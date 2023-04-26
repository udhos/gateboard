package gateboard

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type operationType int

const (
	operationQuery = iota
	operationPut
	operationRefresh
	operationSleep100ms
	operationDeleteFromMain
	operationExpireFromCache
)

type testCase struct {
	name        string
	operation   operationType
	gatewayName string
	expectedID  string
}

var testTable = []testCase{
	{"1: non-existing gateway 1", operationQuery, "gateway1", ""},

	{"2: put gateway", operationPut, "gateway1", "id1"},
	//{"2: existing gateway shold not be found before refresh+sleep", operationQuery, "gateway1", ""},
	//{"2: refresh", operationRefresh, "gateway1", ""},
	//{"2: existing gateway should not be found before sleep", operationQuery, "gateway1", ""},
	//{"2: sleep", operationSleep100ms, "gateway1", ""},
	{"2: existing gateway should be found", operationQuery, "gateway1", "id1"},

	{"3: delete gateway from main", operationDeleteFromMain, "gateway1", ""},
	{"3: find gateway from cache", operationQuery, "gateway1", "id1"},
	{"3: expire gateway from cache", operationExpireFromCache, "gateway1", ""},
	//{"3: refresh", operationRefresh, "gateway1", ""},
	//{"3: sleep", operationSleep100ms, "gateway1", ""},
}

func jsonWrite(w http.ResponseWriter, code int, v interface{}) {
	buf, errJSON := json.Marshal(v)
	if errJSON != nil {
		log.Printf("jsonWrite: json error: %v", errJSON)
	}
	w.WriteHeader(code)
	_, errWrite := w.Write(buf)
	if errWrite != nil {
		log.Printf("jsonWrite: write error: %v", errWrite)
	}
}

func errorPut(w http.ResponseWriter, gatewayName, gatewayID, errorMessage string, code int) {
	var bodyReply BodyPutReply
	bodyReply.GatewayName = gatewayName
	bodyReply.GatewayID = gatewayID
	bodyReply.Error = errorMessage
	jsonWrite(w, code, &bodyReply)
}

func resultGet(w http.ResponseWriter, gatewayName, gatewayID string, found bool) {
	var out BodyGetReply
	out.GatewayName = gatewayName
	out.GatewayID = gatewayID
	out.TTL = 10
	var code int
	if found {
		code = 200
	} else {
		code = 404
	}
	jsonWrite(w, code, &out)
}

func resultPut(w http.ResponseWriter, gatewayName, gatewayID string) {
	var out BodyPutReply
	out.GatewayName = gatewayName
	out.GatewayID = gatewayID
	jsonWrite(w, 200, &out)
}

// go test -v -run TestClient ./gateboard
func TestClient(t *testing.T) {

	dbMain := map[string]string{}
	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("main server: %s %s", r.Method, r.URL)

		gatewayName := strings.TrimPrefix(r.URL.Path, "/gateway/")
		id, found := dbMain[gatewayName]

		resultGet(w, gatewayName, id, found)
	}))
	defer main.Close()
	mainURL, _ := url.JoinPath(main.URL, "/gateway")

	t.Logf("main url:%s", mainURL)

	client := NewClient(ClientOptions{
		ServerURL: mainURL,
	})

	for _, data := range testTable {
		switch data.operation {
		case operationQuery:
			id := client.GatewayID(context.TODO(), data.gatewayName)
			if id != data.expectedID {
				t.Errorf("%s: query error: gateway=%s expectedID=[%s] foundID=[%s]",
					data.name, data.gatewayName, data.expectedID, id)
			}
		case operationPut:
			dbMain[data.gatewayName] = data.expectedID
		case operationRefresh:
			client.Refresh(context.TODO(), data.gatewayName)
		case operationSleep100ms:
			time.Sleep(100 * time.Millisecond)
		case operationDeleteFromMain:
			dbMain[data.gatewayName] = ""
		case operationExpireFromCache:
			entry, found := client.cache[data.gatewayName]
			if found {
				entry.creation = entry.creation.Add(-client.TTL)
				client.cache[data.gatewayName] = entry
			}
		default:
			t.Errorf("%s: unexpected operation: %d", data.name, data.operation)
		}
	}

	const sleep = time.Second
	t.Logf("sleeping for %v", sleep)
	time.Sleep(sleep)
}

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type testCase struct {
	name           string
	method         string
	path           string
	body           string
	expectedStatus int
	expectedID     string
}

const (
	expectAnyStatus = -1
	expectAnyID     = "*"
)

var testTable = []testCase{
	{"GET empty gateway", "GET", "/gateway/", "", 400, expectAnyID},
	{"GET non-existing gateway", "GET", "/gateway/gw1", "", 404, expectAnyID},
	{"PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1"}`, 200, "id1"},
	{"GET existing gateway", "GET", "/gateway/gw1", "", 200, "id1"},
	{"PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 200, "id2"},
	{"GET updated gateway", "GET", "/gateway/gw1", "", 200, "id2"},
	{"PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 200, "id2"},
	{"GET updated gateway 2", "GET", "/gateway/gw1", "", 200, "id2"},
	{"GET non-existing gateway 2", "GET", "/gateway/gw2", "", 404, expectAnyID},
	{"GET non-existing gateway url-like", "GET", "/gateway/http://a:5555/b/c", "", 404, expectAnyID},
	{"PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1"}`, 200, "id1"},
	{"GET existing gateway url-like", "GET", "/gateway/http://a:5555/b/c", "", 200, "id1"},
}

// go test -v -run TestController ./cmd/gateboard
func TestController(t *testing.T) {

	app := newTestApp()

	for _, data := range testTable {

		//req, _ := http.NewRequest("GET", "/gateway/gw1", strings.NewReader(`{"id": "1","name": "joe"}`))
		//req, _ := http.NewRequest("GET", "/gateway/gw1", nil)
		req, _ := http.NewRequest(data.method, data.path, strings.NewReader(data.body))
		w := httptest.NewRecorder()
		app.serverMain.router.ServeHTTP(w, req)

		t.Logf("path: %s", data.path)
		t.Logf("status: %d", w.Code)
		t.Logf("response: %s", w.Body.String())

		if data.expectedStatus != expectAnyStatus {
			if data.expectedStatus != w.Code {
				t.Errorf("%s: %s %s body='%s' status=%d expectedStatus=%d",
					data.name, data.method, data.path, data.body, w.Code, data.expectedStatus)
			}
		}

		if data.expectedID != expectAnyID {
			response := map[string]string{}
			errYaml := yaml.Unmarshal(w.Body.Bytes(), &response)
			if errYaml != nil {
				t.Errorf("%s: %s %s body='%s' status=%d responseBody='%v' yaml error: %v",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String(), errYaml)
			}
			id, found := response["gateway_id"]
			if !found {
				t.Errorf("%s: %s %s body='%s' status=%d responseBody='%v' missing gateway_id in response",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String())
			}
			if id != data.expectedID {
				t.Errorf("%s: %s %s body='%s' status=%d responseBody='%v' gateway_id=%s expected_gateway_id=%s",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String(), id, data.expectedID)
			}
		}
	}
}

func newTestApp() *application {
	app := &application{
		me:   "gateboard_app_test",
		repo: newRepoMem(),
	}

	initApplication(app, ":8080")

	return app
}

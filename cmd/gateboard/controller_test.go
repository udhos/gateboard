package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

var testWriteTokenNoToken = []testCase{
	{"missing token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1"}`, 401, "id1"},
	{"missing token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 401, "id2"},
	{"missing token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 401, "id2"},
	{"missing token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1"}`, 401, "id1"},

	{"empty token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1","token":""}`, 401, "id1"},
	{"empty token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":""}`, 401, "id2"},
	{"empty token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":""}`, 401, "id2"},
	{"empty token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1","token":""}`, 401, "id1"},

	{"bad token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1","token":"BAD_TOKEN"}`, 401, "id1"},
	{"bad token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":"BAD_TOKEN"}`, 401, "id2"},
	{"bad token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":"BAD_TOKEN"}`, 401, "id2"},
	{"bad token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1","token":"BAD_TOKEN"}`, 401, "id1"},
}

var testWriteTokenWithToken = []testCase{
	{"missing token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1"}`, 401, "id1"},
	{"missing token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 401, "id2"},
	{"missing token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 401, "id2"},
	{"missing token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1"}`, 401, "id1"},

	{"empty token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1","token":""}`, 401, "id1"},
	{"empty token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":""}`, 401, "id2"},
	{"empty token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":""}`, 401, "id2"},
	{"empty token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1","token":""}`, 401, "id1"},

	{"good token 1: PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1","token":"good_token"}`, 200, "id1"},
	{"good token 2: PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":"good_token"}`, 200, "id2"},
	{"good token 3: PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2","token":"good_token"}`, 200, "id2"},
	{"good token 4: PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1","token":"good_token"}`, 200, "id1"},
}

// go test -v -run TestController ./cmd/gateboard
func TestController(t *testing.T) {
	testController(t, newTestApp(false), testTable)
	testController(t, newTestApp(true), testWriteTokenNoToken)
	app := newTestApp(true)
	repoPutTokenMultiple(context.TODO(), app, "gw1", "good_token")
	repoPutTokenMultiple(context.TODO(), app, "http://a:5555/b/c", "good_token")
	testController(t, app, testWriteTokenWithToken)
}

func testController(t *testing.T, app *application, table []testCase) {
	for _, data := range table {

		req, errReq := http.NewRequest(data.method, data.path, strings.NewReader(data.body))
		if errReq != nil {
			t.Errorf("%s: NewRequest: %v", data.name, errReq)
			return
		}
		w := httptest.NewRecorder()
		app.serverMain.router.ServeHTTP(w, req)

		t.Logf("%s: path: %s", data.name, data.path)
		t.Logf("%s: status: %d", data.name, w.Code)
		t.Logf("%s: response: %s", data.name, w.Body.String())

		if data.expectedStatus != expectAnyStatus {
			if data.expectedStatus != w.Code {
				t.Errorf("%s: ERROR %s %s token=%t body='%s' status=%d expectedStatus=%d",
					data.name, data.method, data.path, app.config.writeToken, data.body, w.Code, data.expectedStatus)
			}
		}

		if data.expectedID != expectAnyID {
			response := map[string]string{}
			errYaml := yaml.Unmarshal(w.Body.Bytes(), &response)
			if errYaml != nil {
				t.Errorf("%s: ERROR %s %s body='%s' status=%d responseBody='%v' yaml error: %v",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String(), errYaml)
			}
			id, found := response["gateway_id"]
			if !found {
				t.Errorf("%s: ERROR %s %s body='%s' status=%d responseBody='%v' missing gateway_id in response",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String())
			}
			if id != data.expectedID {
				t.Errorf("%s: ERROR %s %s body='%s' status=%d responseBody='%v' gateway_id=%s expected_gateway_id=%s",
					data.name, data.method, data.path, data.body, w.Code, w.Body.String(), id, data.expectedID)
			}
		}
	}
}

func newTestApp(writeToken bool) *application {

	const me = "gateboard_app_test"

	os.Setenv("REPO_LIST", "testdata/repo_mem.yaml")

	app := &application{
		me:     me,
		config: newConfig(me),
	}

	app.config.writeToken = writeToken

	initApplication(app, ":8080")

	return app
}

// go test -bench=BenchmarkController ./cmd/gateboard
func BenchmarkController(b *testing.B) {

	//
	// setup
	//

	os.Setenv("GIN_MODE", "release")

	const writeToken = false

	app := newTestApp(writeToken)

	// PUT

	reqPut, errReqPut := http.NewRequest("PUT", "/gateway/gw1", strings.NewReader(`{"gateway_id":"id1"}`))
	if errReqPut != nil {
		b.Errorf("NewRequest PUT: %v", errReqPut)
		return
	}
	wPut := httptest.NewRecorder()
	app.serverMain.router.ServeHTTP(wPut, reqPut)

	//
	// Actual test
	//

	b.ResetTimer()

	n := 20 // b.N

	for i := 0; i < n; i++ {
		benchController(b, app)
	}
}

func benchController(b *testing.B, app *application) {
	// GET

	reqGet, errReqGet := http.NewRequest("GET", "/gateway/gw1", nil)
	if errReqGet != nil {
		b.Errorf("NewRequest GET: %v", errReqGet)
		return
	}
	wGet := httptest.NewRecorder()
	app.serverMain.router.ServeHTTP(wGet, reqGet)
	//b.Logf("body: %s", wGet.Body.String())
}

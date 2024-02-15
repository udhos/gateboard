package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

// go test -count=1 -run TestMultirepoFastestGoodOnly ./cmd/gateboard
func TestMultirepoFastestGoodOnly(t *testing.T) {
	app := newTestAppMultirepo("testdata/repo_mem_good3.yaml")

	errPut := repoPutMultiple(context.TODO(), app, "gw1", "id1")
	if errPut != nil {
		t.Errorf(errPut.Error())
	}

	body, repoName, errGet := repoGetMultiple(context.TODO(), app, "gw1")
	if errGet != nil {
		t.Errorf(errGet.Error())
	}

	if body.GatewayID != "id1" {
		t.Errorf("wrong id: %s", body.GatewayID)
	}

	if repoName != "mem:mem200" {
		t.Errorf("wrong repo: %s", repoName)
	}
}

// go test -count=1 -run TestMultirepoFastestTwoBad ./cmd/gateboard
func TestMultirepoFastestTwoBad(t *testing.T) {
	app := newTestAppMultirepo("testdata/repo_mem_bad2.yaml")

	errPut := repoPutMultiple(context.TODO(), app, "gw1", "id1")
	if errPut != nil {
		t.Errorf(errPut.Error())
	}

	body, repoName, errGet := repoGetMultiple(context.TODO(), app, "gw1")
	if errGet != nil {
		t.Errorf(errGet.Error())
	}

	if body.GatewayID != "id1" {
		t.Errorf("wrong id: %s", body.GatewayID)
	}

	if repoName != "mem:mem400" {
		t.Errorf("wrong repo: %s", repoName)
	}
}

// go test -count=1 -run TestMultirepoFastestTimeout ./cmd/gateboard
func TestMultirepoFastestTimeout(t *testing.T) {
	app := newTestAppMultirepo("testdata/repo_mem_good3.yaml")

	app.config.repoTimeout = 100 * time.Millisecond

	errPut := repoPutMultiple(context.TODO(), app, "gw1", "id1")
	if errPut != nil {
		t.Errorf(errPut.Error())
	}

	_, _, errGet := repoGetMultiple(context.TODO(), app, "gw1")

	if errGet != errRepositoryTimeout {
		t.Errorf("expected timeout but got: %v", errGet)
	}
}

type multirepoTestCase struct {
	name           string
	method         string
	path           string
	body           string
	expectedStatus int
	expectedID     string
}

var multirepoTestTableGood = []multirepoTestCase{
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

var multirepoTestTableBad = []multirepoTestCase{
	{"GET empty gateway", "GET", "/gateway/", "", 400, expectAnyID},
	{"GET non-existing gateway", "GET", "/gateway/gw1", "", 500, expectAnyID},
	{"PUT gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id1"}`, 500, "id1"},
	{"GET existing gateway", "GET", "/gateway/gw1", "", 500, expectAnyID},
	{"PUT update gateway", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 500, "id2"},
	{"GET updated gateway", "GET", "/gateway/gw1", "", 500, expectAnyID},
	{"PUT update gateway 2", "PUT", "/gateway/gw1", `{"gateway_id":"id2"}`, 500, "id2"},
	{"GET updated gateway 2", "GET", "/gateway/gw1", "", 500, expectAnyID},
	{"GET non-existing gateway 2", "GET", "/gateway/gw2", "", 500, expectAnyID},
	{"GET non-existing gateway url-like", "GET", "/gateway/http://a:5555/b/c", "", 500, expectAnyID},
	{"PUT gateway url-like", "PUT", "/gateway/http://a:5555/b/c", `{"gateway_id":"id1"}`, 500, "id1"},
	{"GET existing gateway url-like", "GET", "/gateway/http://a:5555/b/c", "", 500, expectAnyID},
}

// go test -count=1 -run TestControllerMultirepoGoodOnly ./cmd/gateboard
func TestControllerMultirepoGoodOnly(t *testing.T) {
	testControllerMultirepo(t, newTestAppMultirepo("testdata/repo_mem_two_good.yaml"), multirepoTestTableGood)
}

// go test -count=1 -run TestControllerMultirepoGoodAndBad ./cmd/gateboard
func TestControllerMultirepoGoodAndBad(t *testing.T) {
	testControllerMultirepo(t, newTestAppMultirepo("testdata/repo_mem_two_goodnbad.yaml"), multirepoTestTableGood)
}

// go test -count=1 -run TestControllerMultirepoBad ./cmd/gateboard
func TestControllerMultirepoBad(t *testing.T) {
	testControllerMultirepo(t, newTestAppMultirepo("testdata/repo_mem_two_bad.yaml"), multirepoTestTableBad)
}

func testControllerMultirepo(t *testing.T, app *application, table []multirepoTestCase) {
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

func newTestAppMultirepo(repo string) *application {

	const me = "gateboard_multirepo_test"

	os.Setenv("REPO_LIST", repo)

	app := &application{
		me:       me,
		config:   newConfig(me),
		registry: prometheus.NewRegistry(),
	}

	initApplication(app, ":8080")

	return app
}

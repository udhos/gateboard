package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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

// go test -v -run TestControllerMultirepoGood ./cmd/gateboard
func TestControllerMultirepoGood(t *testing.T) {
	testControllerMultirepo(t, newTestAppMultirepo("testdata/repo_mem_two_good.yaml"), multirepoTestTableGood)
}

func TestControllerMultirepoGoodAndBad(t *testing.T) {
	testControllerMultirepo(t, newTestAppMultirepo("testdata/repo_mem_two_goodnbad.yaml"), multirepoTestTableGood)
}

// go test -v -run TestControllerMultirepoBad ./cmd/gateboard
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
		me:     me,
		config: newConfig(me),
	}

	initApplication(app, ":8080")

	//log.Fatalf("################ %v", app.repoConf)

	return app
}

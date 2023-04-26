package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"gopkg.in/yaml.v3"
)

type queueTestCase struct {
	name           string
	body           string
	path           string
	expectedStatus int
	expectedID     string
}

var queueTestTable = []queueTestCase{
	{"empty gateway 1", `{}`, "/gateway/", 400, expectAnyID},
	{"empty gateway 2", `{}`, "/gateway/gw1", 404, expectAnyID},
	{"empty gateway id 1", `{"gateway_name":"gw1"}`, "/gateway/gw1", 404, expectAnyID},
	{"empty gateway id 2", `{"gateway_name":"gw1","gateway_id":""}`, "/gateway/gw1", 404, expectAnyID},
	{"valid gateway id", `{"gateway_name":"gw1","gateway_id":"id1"}`, "/gateway/gw1", 200, "id1"},
	{"non-existing id", `{"gateway_name":"gw1","gateway_id":"id1"}`, "/gateway/gw2", 404, expectAnyID},
	{"valid gateway id 2", `{"gateway_name":"gw1","gateway_id":"id2"}`, "/gateway/gw1", 200, "id2"},
	{"non-existing id 2", `{"gateway_name":"gw1","gateway_id":"id1"}`, "/gateway/gw2", 404, expectAnyID},
	{"valid gateway url-like", `{"gateway_name":"http://a:5555/b/c","gateway_id":"id1"}`, "/gateway/http://a:5555/b/c", 200, "id1"},
}

// go test -run TestQueueSimple ./cmd/gateboard
func TestQueueSimple(t *testing.T) {
	q := &mockQueue{}

	{
		m := q.send(`{"gateway_name":"gw1","gateway_id":""}`)
		if l := len(q.messages); l != 1 {
			t.Errorf("expecting one message in queue, got: %d", l)
		}
		q.deleteMessage(m)
		if l := len(q.messages); l != 0 {
			t.Errorf("expecting empty queue, got: %d", l)
		}
	}

	q.send(`{"gateway_name":"gw1","gateway_id":""}`)
	if l := len(q.messages); l != 1 {
		t.Errorf("expecting one message in queue, got: %d", l)
	}
	if visible := q.countVisible(); visible != 1 {
		t.Errorf("expecting one visible messages, got: %d", visible)
	}
	list, errRecv := q.receive()
	if errRecv != nil {
		t.Errorf("receive error: %v", errRecv)
	}
	if l := len(list); l != 1 {
		t.Errorf("expecting one message in queue, got: %d", l)
	}
	if visible := q.countVisible(); visible != 0 {
		t.Errorf("expecting zero visible messages, got: %d", visible)
	}
}

// go test -run TestQueue ./cmd/gateboard
func TestQueue(t *testing.T) {
	app := newTestApp(false)

	q := &mockQueue{cooldown: 100 * time.Millisecond}
	app.sqsClient = q
	go sqsListener(app)

	for _, data := range queueTestTable {

		q.send(data.body)          // add message to queue
		time.Sleep(2 * q.cooldown) // give time to receive from queue

		req, _ := http.NewRequest("GET", data.path, strings.NewReader(data.body))
		w := httptest.NewRecorder()
		app.serverMain.router.ServeHTTP(w, req)

		t.Logf("path: %s", data.path)
		t.Logf("status: %d", w.Code)
		t.Logf("response: %s", w.Body.String())

		if data.expectedStatus != expectAnyStatus {
			if data.expectedStatus != w.Code {
				t.Errorf("%s: %s body='%s' status=%d expectedStatus=%d",
					data.name, data.path, data.body, w.Code, data.expectedStatus)
			}
		}

		if data.expectedID != expectAnyID {
			response := map[string]string{}
			errYaml := yaml.Unmarshal(w.Body.Bytes(), &response)
			if errYaml != nil {
				t.Errorf("%s: %s body='%s' status=%d responseBody='%v' yaml error: %v",
					data.name, data.path, data.body, w.Code, w.Body.String(), errYaml)
			}
			id, found := response["gateway_id"]
			if !found {
				t.Errorf("%s: %s body='%s' status=%d responseBody='%v' missing gateway_id in response",
					data.name, data.path, data.body, w.Code, w.Body.String())
			}
			if id != data.expectedID {
				t.Errorf("%s: %s body='%s' status=%d responseBody='%v' gateway_id=%s expected_gateway_id=%s",
					data.name, data.path, data.body, w.Code, w.Body.String(), id, data.expectedID)
			}
		}
	}

}

type mockQueue struct {
	messages []queueMessage
	lock     sync.Mutex
	cooldown time.Duration
}

func (q *mockQueue) errorCooldown() time.Duration {
	return q.cooldown
}

func (q *mockQueue) countVisible() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	var count int

	now := time.Now()

	for _, m := range q.messages {
		mm := m.(*mockMessage)
		visible := mm.visible(now)
		log.Printf("message:%v visible:%v", *mm, visible)
		if visible {
			count++
		}
	}

	return count
}

func (q *mockQueue) receive() ([]queueMessage, error) {

	const visibilityTimeout = 10 * time.Second
	now := time.Now()
	var result []queueMessage

	q.lock.Lock()
	defer q.lock.Unlock()

	for _, m := range q.messages {
		mm := m.(*mockMessage)
		if mm.visible(now) {
			mm.invisibleUntil = now.Add(visibilityTimeout) // set invisibility for message
			result = append(result, m)
			continue
		}
	}
	return result, nil
}

func (q *mockQueue) deleteMessage(old queueMessage) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	var keep []queueMessage
	for _, m := range q.messages {
		if m.id() != old.id() {
			keep = append(keep, m)
		}
	}
	q.messages = keep
	return nil
}

func (q *mockQueue) send(body string) *mockMessage {
	q.lock.Lock()
	defer q.lock.Unlock()
	m := mockMessage{
		mBody: body,
		mID:   ksuid.New().String(),
	}
	q.messages = append(q.messages, &m)
	return &m
}

type mockMessage struct {
	mID            string
	mBody          string
	invisibleUntil time.Time
}

func (m *mockMessage) visible(now time.Time) bool {
	return m.invisibleUntil.IsZero() || m.invisibleUntil.Before(now)
}

func (m *mockMessage) id() string {
	return m.mID
}

func (m *mockMessage) body() string {
	return m.mBody
}

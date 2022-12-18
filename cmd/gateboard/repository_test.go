package main

import (
	"testing"
	"time"

	"github.com/udhos/gateboard/env"
)

// go test -run TestRepository ./cmd/gateboard
func TestRepository(t *testing.T) {

	//
	// test repo mem
	//
	t.Logf("testing repo mem")
	testRepo(t, newRepoMem())

	//
	// optionally test repo redis
	//
	testRedis := env.Bool("TEST_REPO_REDIS", false)
	t.Logf("testing repo redis: %t", testRedis)
	if testRedis {
		r, err := newRepoRedis(repoRedisOptions{
			debug:    false,
			addr:     "localhost:6379",
			password: "",
			key:      "gateboard_test",
		})
		if err != nil {
			t.Errorf("error initialize redis: %v", err)
		}
		if errDrop := r.dropDatabase(); errDrop != nil {
			t.Errorf("dropping database: %v", errDrop)
		}
		testRepo(t, r)
	}

	//
	// optionally test repo dynamodb
	//
	testDynamo := env.Bool("TEST_REPO_DYNAMO", false)
	t.Logf("testing repo dynamo: %t", testDynamo)
	if testDynamo {
		r, err := newRepoDynamo(repoDynamoOptions{
			table:  "gateboard_test",
			region: "us-east-1",
			debug:  false,
		})
		if err != nil {
			t.Errorf("error initialize dynamodb: %v", err)
		}
		if errDrop := r.dropDatabase(); errDrop != nil {
			t.Errorf("dropping database: %v", errDrop)
		}
		testRepo(t, r)
	}

	//
	// optionally test repo mongo
	//
	testMongo := env.Bool("TEST_REPO_MONGO", false)
	t.Logf("testing repo mongo: %t", testMongo)
	if testMongo {
		r, err := newRepoMongo(repoMongoOptions{
			debug:      false,
			URI:        env.String("MONGO_URL", "mongodb://localhost:27017"),
			database:   "gateboard_test",
			collection: "gateboard_test",
			timeout:    time.Second * 10,
		})
		if err != nil {
			t.Errorf("error initialize mongodb: %v", err)
		}
		if errDrop := r.dropDatabase(); errDrop != nil {
			t.Errorf("dropping database: %v", errDrop)
		}
		testRepo(t, r)
	}
}

func testRepo(t *testing.T, r repository) {
	const expectError = true
	const expectOk = false

	queryExpectError(t, r, "")         // should not find empty key
	queryExpectError(t, r, "XXX")      // should not find non-existing key
	save(t, r, "", "XXX", expectError) // should not insert empty key
	save(t, r, "gw1", "", expectError) // should not insert empty value
	save(t, r, "", "", expectError)    // should not insert all empty

	queryExpectError(t, r, "gw1")      // gw1 does not exist yet
	save(t, r, "gw1", "id1", expectOk) // insert key
	queryExpectID(t, r, "gw1", "id1")  // should find inserted key

	save(t, r, "gw1", "id2", expectOk) // update key
	queryExpectID(t, r, "gw1", "id2")  // should find updated key

	save(t, r, "gw2", "id2", expectOk) // update key
	queryExpectID(t, r, "gw2", "id2")  // should find updated key

	tokenSaveAndQuery(t, r, "gw1", "token1", "token1")
	tokenSaveAndQuery(t, r, "gw1", "token1", "token1")
	tokenSaveAndQuery(t, r, "gw2", "token2", "token2")
}

func tokenSaveAndQuery(t *testing.T, r repository, gatewayName, token, expectedToken string) {

	errPut := r.putToken(gatewayName, token)
	if errPut != nil {
		t.Errorf("tokenSaveAndQuery: putToken: gatewayName=%s token=%s unexpected error: %v",
			gatewayName, token, errPut)
	}

	body, err := r.get(gatewayName)
	if err != nil {
		t.Errorf("tokenSaveAndQuery: get: gatewayName=%s token=%s unexpected error: %v",
			gatewayName, token, err)
	}

	if body.Token != expectedToken {
		t.Errorf("tokenSaveAndQuery: gatewayName=%s expectedToken=%s got token=%s",
			gatewayName, expectedToken, body.Token)
	}
}

func queryExpectError(t *testing.T, r repository, gatewayName string) {
	_, err := r.get(gatewayName)
	if err == nil {
		t.Errorf("queryExpectError: gatewayName=%s expecting error",
			gatewayName)
	}
}

func queryExpectID(t *testing.T, r repository, gatewayName, expectedGatewayID string) {
	body, err := r.get(gatewayName)
	if err != nil {
		t.Errorf("queryExpectID: gatewayName=%s expectedGatewayID=%s unexpected error:%v",
			gatewayName, expectedGatewayID, err)
		return
	}
	if body.GatewayID != expectedGatewayID {
		t.Errorf("queryExpectID: gatewayName=%s expectedGatewayID=%s got ID=%s",
			gatewayName, expectedGatewayID, body.GatewayID)
	}
}

func save(t *testing.T, r repository, gatewayName, gatewayID string, expectError bool) {
	err := r.put(gatewayName, gatewayID)
	gotError := err != nil
	if gotError != expectError {
		if expectError {
			t.Errorf("save: gatewayName=%s gatewayID=%s expecting error",
				gatewayName, gatewayID)
		} else {
			t.Errorf("save: gatewayName=%s gatewayID=%s unexpected error: %v",
				gatewayName, gatewayID, err)
		}
	}
}

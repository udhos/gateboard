package main

import (
	"testing"
	"time"

	"github.com/udhos/gateboard/env"
)

// go test -run TestRepository ./cmd/gateboard
func TestRepository(t *testing.T) {

	const table = "gateboard_test"
	const debug = true

	//
	// test repo mem
	//
	t.Logf("testing repo mem")
	testRepo(t, newRepoMem(), table)

	//
	// optionally test repo redis
	//
	testRedis := env.Bool("TEST_REPO_REDIS", false)
	t.Logf("testing repo redis: %t", testRedis)
	if testRedis {
		r, err := newRepoRedis(repoRedisOptions{
			debug:    debug,
			addr:     "localhost:6379",
			password: "",
			key:      table,
		})
		if err != nil {
			t.Errorf("error initialize redis: %v", err)
		}
		if errDrop := r.dropDatabase(); errDrop != nil {
			t.Errorf("dropping database: %v", errDrop)
		}
		testRepo(t, r, table)
	}

	//
	// optionally test repo dynamodb
	//
	testDynamo := env.Bool("TEST_REPO_DYNAMO", false)
	t.Logf("testing repo dynamo: %t", testDynamo)
	if testDynamo {
		{
			//
			// temporary client just to reset the table
			//
			r, err := newRepoDynamo(repoDynamoOptions{
				table:        table,
				region:       "us-east-1",
				debug:        debug,
				manualCreate: true, // do not create table
			})
			if err != nil {
				t.Errorf("error initialize dynamodb: %v", err)
			}
			if errDrop := r.dropDatabase(); errDrop != nil {
				// just log since it is not an error,
				// the table might not exist
				t.Logf("dropping database: %v", errDrop)
			}
		}
		//
		// actual client for testing
		//
		r, err := newRepoDynamo(repoDynamoOptions{
			table:  table,
			region: "us-east-1",
			debug:  debug,
		})
		if err != nil {
			t.Errorf("error initialize dynamodb: %v", err)
		}
		testRepo(t, r, table)
	}

	//
	// optionally test repo mongo
	//
	testMongo := env.Bool("TEST_REPO_MONGO", false)
	t.Logf("testing repo mongo: %t", testMongo)
	if testMongo {
		r, err := newRepoMongo(repoMongoOptions{
			debug:      debug,
			URI:        env.String("MONGO_URL", "mongodb://localhost:27017"),
			database:   table,
			collection: table,
			timeout:    time.Second * 10,
		})
		if err != nil {
			t.Errorf("error initialize mongodb: %v", err)
		}
		if errDrop := r.dropDatabase(); errDrop != nil {
			t.Errorf("dropping database: %v", errDrop)
		}
		testRepo(t, r, table)
	}
}

func testRepo(t *testing.T, r repository, table string) {
	const expectError = true
	const expectOk = false

	queryExpectError(t, r, "")                // should not find empty key
	queryExpectError(t, r, "XXX")             // should not find non-existing key
	save(t, r, table, "", "XXX", expectError) // should not insert empty key
	save(t, r, table, "gw1", "", expectError) // should not insert empty value
	save(t, r, table, "", "", expectError)    // should not insert all empty

	queryExpectError(t, r, "gw1")               // gw1 does not exist yet
	save(t, r, table, "gw1", "id1", expectOk)   // insert key
	queryExpectID(t, r, "query1", "gw1", "id1") // should find inserted key

	save(t, r, table, "gw1", "id2", expectOk)   // update key
	queryExpectID(t, r, "query2", "gw1", "id2") // should find updated key

	save(t, r, table, "gw2", "id2", expectOk)   // update key
	queryExpectID(t, r, "query3", "gw2", "id2") // should find updated key

	tokenSaveAndQuery(t, r, table, "gw1", "token1", "token1")
	tokenSaveAndQuery(t, r, table, "gw1", "token1", "token1")
	tokenSaveAndQuery(t, r, table, "gw2", "token2", "token2")
}

func tokenSaveAndQuery(t *testing.T, r repository, table, gatewayName, token, expectedToken string) {

	errPut := r.putToken(gatewayName, token)
	if errPut != nil {
		t.Errorf("tokenSaveAndQuery: putToken: table=%s gatewayName=%s token=%s unexpected error: %v",
			table, gatewayName, token, errPut)
	}

	body, err := r.get(gatewayName)
	if err != nil {
		t.Errorf("tokenSaveAndQuery: get: table=%s gatewayName=%s token=%s unexpected error: %v",
			table, gatewayName, token, err)
	}

	if body.Token != expectedToken {
		t.Errorf("tokenSaveAndQuery: table=%s gatewayName=%s expectedToken=%s got token=%s",
			table, gatewayName, expectedToken, body.Token)
	}
}

func queryExpectError(t *testing.T, r repository, gatewayName string) {
	_, err := r.get(gatewayName)
	if err == nil {
		t.Errorf("queryExpectError: gatewayName=%s expecting error",
			gatewayName)
	}
}

func queryExpectID(t *testing.T, r repository, name, gatewayName, expectedGatewayID string) {
	body, err := r.get(gatewayName)
	if err != nil {
		t.Errorf("queryExpectID: %s: gatewayName=%s expectedGatewayID=%s unexpected error:%v",
			name, gatewayName, expectedGatewayID, err)
		return
	}
	if body.GatewayID != expectedGatewayID {
		t.Errorf("queryExpectID: %s: gatewayName=%s expectedGatewayID=%s got ID=%s",
			name, gatewayName, expectedGatewayID, body.GatewayID)
	}
}

func save(t *testing.T, r repository, table, gatewayName, gatewayID string, expectError bool) {
	err := r.put(gatewayName, gatewayID)
	gotError := err != nil
	if gotError != expectError {
		if expectError {
			t.Errorf("save: table=%s gatewayName=%s gatewayID=%s expecting error",
				table, gatewayName, gatewayID)
		} else {
			t.Errorf("save: table=%s, gatewayName=%s gatewayID=%s unexpected error: %v",
				table, gatewayName, gatewayID, err)
		}
	}
}

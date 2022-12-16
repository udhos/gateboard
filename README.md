[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/gateboard/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/gateboard)](https://goreportcard.com/report/github.com/udhos/gateboard)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/gateboard.svg)](https://pkg.go.dev/github.com/udhos/gateboard)

# gateboard

[gateboard](https://github.com/udhos/gateboard) resolves AWS Private API Gateway ID.

## TODO

- [X] SQS listener
- [X] Client with async update
- [X] Create mongodb index on startup
- [X] Define TTL on server record (60s), restrict acceptable TTL range on client (60s..600s)
- [X] Repository tests
- [X] HTTP server tests
- [X] SQS tests
- [X] Docker image
- [X] Client tests
- [X] Refactor config
- [X] Repository DynamoDB
- [ ] Optional authentication
- [ ] Zap logging
- [ ] Metrics
- [ ] Tracing
- [ ] Benchmark
- [ ] User guide

## Build

```bash
git clone https://github.com/udhos/gateboard
cd gateboard
CGO_ENABLED=0 go install ./...
```

## Testing repository mongo

Start mongodb:

```bash
docker run --rm --name mongo-main -p 27017:27017 -d mongo
gateboard
```

Run repository tests:

```bash
export TEST_REPO_MONGO=true ;# enable mongodb tests
go test -count=1 -run TestRepository ./cmd/gateboard
```

## Testing repository dynamodb

Create a dynamodb table named `gateboard_test` with partition key `gateway_name`.

Make sure the table is empty before running the tests.

Run repository tests:

```bash
export TEST_REPO_DYNAMO=true ;# enable dynamodb tests
go test -count=1 -run TestRepository ./cmd/gateboard
```

## Running both servers on same host

Main:

```bash
docker run --rm --name mongo-main -p 27017:27017 -d mongo
export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/140330866198/gateboard
gateboard
```

Fallback:

```bash
docker run --rm --name mongo-fallback -p 27018:27017 -d mongo
export LISTEN_ADDR=:8181                   ;# main 8080
export HEALTH_ADDR=:9999                   ;# main 8888
export METRICS_ADDR=:3001                  ;# main 3000
export MONGO_URL=mongodb://localhost:27018 ;# main mongodb://localhost:27017
gateboard
```

Run interactive client:

```bash
gateboard-client-example
```

## Example

```bash
curl localhost:8080/gateway/gate1
{"gateway_name":"gate1","gateway_id":"","error":"gatewayGet: not found: repository gateway not found error"}

curl -X PUT -d '{"gateway_id":"id1"}' localhost:8080/gateway/gate1
{"gateway_name":"gate1","gateway_id":"id1"}

curl localhost:8080/gateway/gate1
{"gateway_name":"gate1","gateway_id":"id1"}

curl -X PUT -d '{"gateway_id":"id2"}' localhost:8080/gateway/gate1
{"gateway_name":"gate1","gateway_id":"id2"}

curl localhost:8080/gateway/gate1
{"gateway_name":"gate1","gateway_id":"id2"}
```

## Docker

Docker hub:

https://hub.docker.com/r/udhos/gateboard

Pull from docker hub:

```
docker pull udhos/gateboard:0.0.0
```

Build recipe:

```
./docker/build.sh
```

Multiarch build recipe:

```
./docker/build-multiarch.sh
```

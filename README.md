[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/gateboard/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/gateboard)](https://goreportcard.com/report/github.com/udhos/gateboard)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/gateboard.svg)](https://pkg.go.dev/github.com/udhos/gateboard)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/gateboard)](https://artifacthub.io/packages/search?repo=gateboard)
[![Docker Pulls gateboard](https://img.shields.io/docker/pulls/udhos/gateboard)](https://hub.docker.com/r/udhos/gateboard)
[![Docker Pulls gateboard-discovery](https://img.shields.io/docker/pulls/udhos/gateboard-discovery)](https://hub.docker.com/r/udhos/gateboard-discovery)

# gateboard

[gateboard](https://github.com/udhos/gateboard) resolves AWS Private API Gateway ID.

In order to [invoke an AWS API Gateway private API](https://docs.aws.amazon.com/apigateway/latest/developerguide/apigateway-private-api-test-invoke-url.html), an applications needs its API ID.

However, whenever API is destroyed and recreated, the API ID will change.

This projects helps in building an autodiscovery mechanism to withstand changes in the API ID.

There are three main pieces:

1\. `gateboard` service ([gateboard](./cmd/gateboard)) provides a simple REST API for saving/retrieving name-to-id mappings:

```bash
# add api1=>id1 mapping
curl -X PUT -d '{"gateway_id":"id1"}' localhost:8080/gateway/api1

# retrieve mapping for api1
curl localhost:8080/gateway/api1
{"gateway_name":"api1","gateway_id":"id1"}
```

2\. `gateboard-discovery` service ([gateboard-discovery](./cmd/gateboard-discovery)) provides autodiscovery by periodically scanning AWS API Gateway API and updating `gateboard` with discovered name-to-id mappings.

3\. `gateboard` client ([gateboard](./gateboard)) is a helper library to assist client application in retrieving current API ID for a given API name. The main benefit from this client is automatic caching for the name-to-id mappings.

`gateboard` client usage looks like this:

```golang
client := gateboard.NewClient(gateboard.ClientOptions{
    ServerURL: "http://gateboard:8080/gateway",
})

apiName := "my-api"

apiID := client.GatewayID(ctx, apiName)
if gatewayID == "" {
    log.Printf("missing gateway_id for gateway_name=%s", apiName)
    return
}

// code to invoke AWS API with apiID follows
```

Find client documentation here: https://pkg.go.dev/github.com/udhos/gateboard@main/gateboard

# Features

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
- [X] Optional authentication
- [X] AWS Secrets Manager
- [X] Preload token for optional authentication
- [X] Gateway load sharing
- [X] Repository redis
- [X] Discovery service
- [X] Metrics
- [X] Repository S3
- [X] Tracing
- [X] Benchmark
- [ ] User guide
- [X] Zap logging
- [ ] Cache service
- [X] Multiple repositories - basic tests
- [X] Multiple repositories - dump
- [X] Multiple repositories - multirepo tests
- [X] Multiple repositories - README
- [X] Multiple repositories - secrets
- [X] Multiple repositories - metrics
- [X] Multiple repositories - helm chart
- [X] Multiple repositories - fastest concurrent response
- [X] Multiple repositories - fastest concurrent response tests
- [X] Group cache

# Services

| Service  | Description |
| --- | --- |
| gateboard | Holds database of key value mappings: gateway_name => gateway_id. You can populate the database as you wish. |
| gateboard-discovery | Can be used to scan AWS API Gateway APIs and to save the name x ID mappings into gateboard. |
| gateboard-cache | Can be used as local fast cache to save resources on a centralized main gateboard service. |

# Build

```bash
git clone https://github.com/udhos/gateboard
cd gateboard
CGO_ENABLED=0 go install ./...
```

# Supported Repositories (Persistent Storage)

```
# Available repo kinds:
#
# mem:      testing-only pseudo-storage
# mongo:    MongoDB
# redis:    redis
# dynamodb: DynamoDB
# s3:       S3

export REPO_LIST=repo.yaml

$ cat repo.yaml
- kind: mem
  name: mem1 # name is used for metrics
- kind: mem
  name: mem2 # pick distinct names for multiple instances of the same kind

- kind: mongo
  name: mongo1 # name is used for metrics
  mongo:
    uri: mongodb://localhost:27017/
    database: gateboard
    collection: gateboard
    username: ""
    #password: "aws-parameterstore:us-east-1:/mongo/cluster1/password" # see https://github.com/udhos/boilerplate
    #tls_ca_file: /etc/gateboard/mongo-tls-ca-bundle.pem
    min_pool: 1
    index_creation_disable: false
    index_creation_retry: 5
    index_creation_cooldown: 5s

- kind: dynamodb
  name: dynamo1 # name is used for metrics
  dynamodb:
    table: gateboard
    region: us-east-1
    role_arn: ""
    manual_create: false # if false, gateboard will create the table automatically

- kind: redis
  name: redis1 # name is used for metrics
  redis:
    addr: localhost:6379
    #password: "aws-parameterstore:us-east-1:/redis/cluster3/password" # see https://github.com/udhos/boilerplate
    key: gateboard
    #tls: true
    #tls_insecure_skip_verify: true
    client_name: auto # 'auto' means use hostname

- kind: s3
  name: s3one # name is used for metrics
  s3:
    bucket_name: ""
    bucket_region: us-east-1
    prefix: gateboard
    role_arn: ""
    manual_create: false # if false, gateboard will create the bucket automatically
    #server_side_encryption: AES256
```

# Testing repositories

## Testing repository mongo

Start mongodb:

```bash
docker run --rm --name mongo-main -p 27017:27017 mongo
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

## Testing repository redis

Start redis:

```bash
docker run --rm --name redis-main -p 6379:6379 -d redis
```

Run repository tests:

```bash
export TEST_REPO_REDIS=true ;# enable redis tests
go test -count=1 -run TestRepository ./cmd/gateboard
```

## Testing repository S3

Create a bucket.

Make sure the bucket is empty before running the tests.

Run repository tests:

```bash
export TEST_REPO_S3=put_bucket_name_here ;# enable S3 tests
go test -count=1 -run TestRepository ./cmd/gateboard
```

## Optional Authentication

Enable `WRITE_TOKEN=true` in order to require token authentication for write requests.

```bash
export WRITE_TOKEN=true
```

Make sure the repository has the token `token1` assigned to gateway `gw2`.

```bash
Example for mongodb:

db.gateboard.insertOne({"gateway_name":"gw2","token":"token1"})
```

Now requests to update gateway `gw2` must include the token `token1`.


```bash
curl -X PUT -s -d '{"gateway_id":"id1","token":"token1"}' localhost:8080/gateway/gw2

{"gateway_name":"gw2","gateway_id":"id1"}
```

Otherwise the request will be denied.

```bash
curl -X PUT -v -d '{"gateway_id":"id2"}' localhost:8080/gateway/gw2
*   Trying ::1:8080...
* TCP_NODELAY set
* Connected to localhost (::1) port 8080 (#0)
> PUT /gateway/gw2 HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.68.0
> Accept: */*
> Content-Length: 20
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 20 out of 20 bytes
* Mark bundle as not supporting multiuse
< HTTP/1.1 401 Unauthorized
< Content-Type: application/json; charset=utf-8
< Date: Sat, 17 Dec 2022 00:59:22 GMT
< Content-Length: 65
< 
* Connection #0 to host localhost left intact
{"gateway_name":"gw2","gateway_id":"id2","error":"invalid token"}
```

# Examples

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

# gateway-discovery

## Save to server

Discovery writes directly to server.

Start server.

    export REPO_LIST=repo.yaml
    cat repo.yaml
    - kind: mem
      name: mem1
    gateboard

Run discovery.

    export SAVE=server
    export DRY_RUN=false
    gateboard-discovery

Dump database.

    curl localhost:8080/dump | jq

## Save to webhook

Discovery writes to webhook that forwards to SQS queue.

Start server.

    export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
    export REPO_LIST=repo.yaml
    cat repo.yaml
    - kind: mem
      name: mem1
    gateboard

Run discovery.

    export SAVE=webhook
    # use lambda function url as webhook
    export WEBHOOK_URL=https://xxxxxxxxxxxxx.lambda-url.us-east-1.on.aws
    export DRY_RUN=false
    gateboard-discovery

Dump database.

    curl localhost:8080/dump | jq

## Save to SQS

Discovery writes to SQS queue.

Start server.

    export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
    export REPO_LIST=repo.yaml
    cat repo.yaml
    - kind: mem
      name: mem1
    gateboard

Run discovery.

    export SAVE=sqs
    export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
    export DRY_RUN=false
    gateboard-discovery

Dump database.

    curl localhost:8080/dump | jq

## Save to SNS

Discovery writes to SNS topic that forwards to SQS queue.

Start server.

    export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
    export REPO_LIST=repo.yaml
    cat repo.yaml
    - kind: mem
      name: mem1
    gateboard

Run discovery.

    export SAVE=sns
    export TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:gateboard
    export DRY_RUN=false
    gateboard-discovery

Dump database.

    curl localhost:8080/dump | jq

## Save to lambda

Discovery writes to lambda function that forwards to SQS queue.

Start server.

    export QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/gateboard
    export REPO_LIST=repo.yaml
    cat repo.yaml
    - kind: mem
      name: mem1
    gateboard

Run discovery.

    export SAVE=lambda
    export LAMBDA_ARN=arn:aws:lambda:us-east-1:123456789012:function:forward_to_sqs
    export DRY_RUN=false
    gateboard-discovery

Dump database.

    curl localhost:8080/dump | jq

# AWS Secrets Manager

Retrieve config vars from AWS Secrets Manager.

    export CONFIG_VAR=aws-secretsmanager:region:name:json_field

See detailed documentation at https://github.com/udhos/boilerplate.

Example:

    export MONGO_URL=aws-secretsmanager::mongo_uri

    # The secret `mongo_uri` must store a scalar value like: `mongodb://127.0.0.1:27017`

Example with JSON field `uri`:

    export MONGO_URL=aws-secretsmanager::mongo:uri

    # The secret `mongo` must store a JSON value like: `{"uri":"mongodb://127.0.0.2:27017"}`

# Metrics

```
# HELP http_server_requests_seconds Spring-like server request duration in seconds.
# TYPE http_server_requests_seconds histogram

Example: http_server_requests_seconds_bucket{method="GET",status="200",uri="/gateway/*gateway_name",le="0.001"} 4

# HELP repository_requests_seconds Repository request duration in seconds.
# TYPE repository_requests_seconds histogram

Example: repository_requests_seconds_bucket{method="get",status="success",le="0.00025"} 4
```

# Test Jaeger Tracing

```
# start jaeger
./run-jaeger-local.sh

# start gateboard
export REPO_LIST=repo.yaml
cat repo.yaml
- kind: mem
    name: mem1

export OTEL_TRACES_SAMPLER=parentbased_always_on
export OTEL_SERVICE_NAME=mynamespace.gateboard
# Jaeger
export OTELCONFIG_EXPORTER=jaeger
export OTEL_TRACES_EXPORTER=jaeger
export OTEL_PROPAGATORS=b3multi
export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:14268
# OTLP gRPC
#export OTELCONFIG_EXPORTER=grpc
#export OTEL_TRACES_EXPORTER=otlp
#export OTEL_PROPAGATORS=b3multi
#export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4317
# OTLP HTTP
#export OTELCONFIG_EXPORTER=http
#export OTEL_TRACES_EXPORTER=otlp
#export OTEL_PROPAGATORS=b3multi
#export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4318

gateboard

# open jaeger UI http://localhost:16686/
```

Jaeger: https://www.jaegertracing.io/docs/1.44/getting-started/

Open Telemetry Go: https://opentelemetry.io/docs/instrumentation/go/getting-started/


# Docker

## gateboard docker image

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

## gateboard-discovery docker image

Docker hub:

https://hub.docker.com/r/udhos/gateboard-discovery

Pull from docker hub:

```
docker pull udhos/gateboard-discovery:0.0.0
```

Build recipe:

```
./docker/build-discovery.sh
```

Push:

```
docker push -a udhos/gateboard-discovery
```

# Helm charts

You can use the provided helm charts to install gateboard in your Kubernetes cluster.

See: https://udhos.github.io/gateboard/

# Datadog

Image tags suffixed with `-datadog` are instrumented with Datadog [Orchestrion](https://github.com/DataDog/orchestrion).

See https://hub.docker.com/r/udhos/gateboard/tags

## Example

Install latest image instrumented with support for Datadog:

    helm upgrade --install gateboard-dd ./charts/gateboard --set image.tag=latest-datadog

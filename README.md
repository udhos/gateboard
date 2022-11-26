# gateboard

## Running both servers on same host

Main:

```bash
docker run --rm --name mongo-main -p 27017:27017 -d mongo
#export QUEUE_URL=...
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

# gateboard

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

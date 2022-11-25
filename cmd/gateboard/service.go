package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/udhos/gateboard/gateboard"
	yaml "gopkg.in/yaml.v3"
)

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	ctx := c.Request.Context()
	_, span := app.tracer.Start(ctx, me)
	defer span.End()

	log.Printf("traceID=%s", span.SpanContext().TraceID())

	gatewayName := c.Param("gateway_name")

	log.Printf("%s: traceID=%s gateway_name=%s", me, span.SpanContext().TraceID(), gatewayName)

	var out gateboard.BodyGetReply
	out.GatewayName = gatewayName

	//
	// retrieve gateway_id
	//

	gateway_id, errID := app.repo.get(gatewayName)
	switch errID {
	case nil:
	case errRepositoryGatewayNotFound:
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	out.GatewayID = gateway_id

	c.JSON(http.StatusOK, out)
}

type bodyPutRequest struct {
	GatewayID string `json:"gateway_id" yaml:"gateway_id"`
}

type bodyPutReply struct {
	GatewayName string `json:"gateway_name"`
	GatewayID   string `json:"gateway_id"`
	Error       string `json:"error,omitempty"`
}

func gatewayPut(c *gin.Context, app *application) {
	const me = "gatewayPut"

	ctx := c.Request.Context()
	_, span := app.tracer.Start(ctx, me)
	defer span.End()

	log.Printf("%s: traceID=%s", me, span.SpanContext().TraceID())

	gatewayName := c.Param("gateway_name")

	log.Printf("%s: traceID=%s gateway_name=%s", me, span.SpanContext().TraceID(), gatewayName)

	var out bodyPutReply
	out.GatewayName = gatewayName

	//
	// parse body to get gateway_id
	//

	dec := yaml.NewDecoder(c.Request.Body)
	var in bodyPutRequest
	errYaml := dec.Decode(&in)
	if errYaml != nil {
		out.Error = fmt.Sprintf("%s: body yaml: %v", me, errYaml)
		log.Print(out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	log.Printf("%s: traceID=%s gateway_name=%s body:%v", me, span.SpanContext().TraceID(), gatewayName, toJSON(in))

	out.GatewayID = in.GatewayID

	//
	// refuse blank gateway_id
	//

	gatewayID := strings.TrimSpace(in.GatewayID)
	if gatewayID == "" {
		out.Error = "invalid gateway_id"
		log.Print(out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	out.GatewayID = gatewayID

	//
	// save gateway_id
	//

	errPut := app.repo.put(gatewayName, gatewayID)
	if errPut != nil {
		out.Error = fmt.Sprintf("%s: error: %v", me, errPut)
		log.Print(out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, out)
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJSON: %v", err)
	}
	return string(b)
}

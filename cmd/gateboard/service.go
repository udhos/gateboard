package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/udhos/gateboard/gateboard"
	"go.opentelemetry.io/otel/trace"
	yaml "gopkg.in/yaml.v3"
)

func getTrace(caller string, c *gin.Context, app *application) (context.Context, trace.Span) {
	ctx := c.Request.Context()
	if app.tracer == nil {
		return ctx, nil
	}
	newCtx, span := app.tracer.Start(ctx, caller)
	return newCtx, span
}

func getTraceID(span trace.Span) string {
	if span == nil {
		return "tracing-disabled"
	}
	return span.SpanContext().TraceID().String()
}

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	/*
		ctx := c.Request.Context()
		_, span := app.tracer.Start(ctx, me)
		defer span.End()
	*/
	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}

	log.Printf("traceID=%s", getTraceID(span))

	gatewayName := c.Param("gateway_name")

	log.Printf("%s: traceID=%s gateway_name=%s", me, getTraceID(span), gatewayName)

	var out gateboard.BodyGetReply
	out.GatewayName = gatewayName
	out.TTL = app.TTL

	//
	// retrieve gateway_id
	//

	gatewayID, errID := app.repo.get(gatewayName)
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

	out.GatewayID = gatewayID

	c.JSON(http.StatusOK, out)
}

func gatewayPut(c *gin.Context, app *application) {
	const me = "gatewayPut"

	/*
		ctx := c.Request.Context()
		_, span := app.tracer.Start(ctx, me)
		defer span.End()
	*/
	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}

	log.Printf("%s: traceID=%s", me, getTraceID(span))

	gatewayName := c.Param("gateway_name")

	log.Printf("%s: traceID=%s gateway_name=%s", me, getTraceID(span), gatewayName)

	var out gateboard.BodyPutReply
	out.GatewayName = gatewayName

	//
	// parse body to get gateway_id
	//

	dec := yaml.NewDecoder(c.Request.Body)
	var in gateboard.BodyPutRequest
	errYaml := dec.Decode(&in)
	if errYaml != nil {
		out.Error = fmt.Sprintf("%s: body yaml: %v", me, errYaml)
		log.Print(out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	log.Printf("%s: traceID=%s gateway_name=%s body:%v", me, getTraceID(span), gatewayName, toJSON(in))

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

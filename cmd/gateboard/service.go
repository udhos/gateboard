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

func gatewayDump(c *gin.Context, app *application) {
	const me = "gatewayDump"

	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}

	log.Printf("traceID=%s", getTraceID(span))

	//
	// dump gateways
	//

	type output struct {
		Error string
	}

	var out output

	dump, errID := app.repo.dump()
	switch errID {
	case nil:
	case errRepositoryGatewayNotFound:
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, dump)
}

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}

	log.Printf("traceID=%s", getTraceID(span))

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	log.Printf("%s: traceID=%s gateway_name=%s", me, getTraceID(span), gatewayName)

	var out gateboard.BodyGetReply

	if strings.TrimSpace(gatewayName) == "" {
		out.GatewayName = gatewayName
		out.TTL = app.config.TTL
		out.Error = fmt.Sprintf("%s: empty gateway name is invalid", me)
		log.Print(out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	//
	// retrieve gateway_id
	//

	out, errID := app.repo.get(gatewayName)
	out.Token = "" // prevent token leaking
	out.TTL = app.config.TTL
	switch errID {
	case nil:
	case errRepositoryGatewayNotFound:
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, out)
}

func gatewayPut(c *gin.Context, app *application) {
	const me = "gatewayPut"

	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}

	log.Printf("%s: traceID=%s", me, getTraceID(span))

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	log.Printf("%s: traceID=%s gateway_name=%s", me, getTraceID(span), gatewayName)

	var out gateboard.BodyPutReply
	out.GatewayName = gatewayName

	if strings.TrimSpace(gatewayName) == "" {
		out.Error = fmt.Sprintf("%s: empty gateway name is invalid", me)
		log.Print(out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

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
	// check write token
	//

	if app.config.writeToken {
		if invalidToken(app, gatewayName, in.Token) {
			out.Error = "invalid token"
			log.Print(out.Error)
			c.JSON(http.StatusUnauthorized, out)
			return
		}
	}

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

func invalidToken(app *application, gatewayName, token string) bool {
	const me = "invalidToken"

	if token == "" {
		return true // empty token is always invalid
	}

	result, errID := app.repo.get(gatewayName)
	if errID != nil {
		log.Printf("%s: error: %v", me, errID)
		return true
	}

	return result.Token != token
}

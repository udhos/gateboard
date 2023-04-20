package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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

const (
	repoStatusOK       = "success"
	repoStatusError    = "error"
	repoStatusNotFound = "not-found"
)

func gatewayDump(c *gin.Context, app *application) {
	const me = "gatewayDump"

	_, span := getTrace(me, c, app)
	if span != nil {
		defer span.End()
	}
	traceID := getTraceID(span)

	log.Printf("%s: traceID=%s", me, traceID)

	//
	// dump gateways
	//

	begin := time.Now()

	type output struct {
		Error string
	}

	var out output

	dump, errDump := app.repo.dump()

	elap := time.Since(begin)
	if app.config.debug {
		log.Printf("%s: traceID=%s repo_dump_latency: elapsed=%v (error:%v)",
			me, traceID, elap, errDump)
	}

	const repoMethod = "dump"

	switch errDump {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		log.Print(out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
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
	traceID := getTraceID(span)

	log.Printf("traceID=%s", traceID)

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	log.Printf("%s: traceID=%s gateway_name=%s", me, traceID, gatewayName)

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

	begin := time.Now()

	out, errID := app.repo.get(gatewayName)
	out.Token = "" // prevent token leaking
	out.TTL = app.config.TTL

	elap := time.Since(begin)
	if app.config.debug {
		log.Printf("%s: traceID=%s gateway_name=%s repo_get_latency: elapsed=%v (error:%v)",
			me, traceID, gatewayName, elap, errID)
	}

	const repoMethod = "get"

	switch errID {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		log.Print(out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
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
	traceID := getTraceID(span)

	log.Printf("%s: traceID=%s", me, traceID)

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	log.Printf("%s: traceID=%s gateway_name=%s", me, traceID, gatewayName)

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

	log.Printf("%s: traceID=%s gateway_name=%s body:%v", me, traceID, gatewayName, toJSON(in))

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

	max := app.config.writeRetry

	for attempt := 1; attempt <= max; attempt++ {

		begin := time.Now()

		errPut := app.repo.put(gatewayName, gatewayID)

		elap := time.Since(begin)
		if app.config.debug {
			log.Printf("%s: traceID=%s gateway_name=%s repo_put_latency: elapsed=%v (error:%v)",
				me, traceID, gatewayName, elap, errPut)
		}

		const repoMethod = "put"

		if errPut == nil {
			recordRepositoryLatency(repoMethod, repoStatusOK, elap)
			out.Error = ""
			c.JSON(http.StatusOK, out)
			return
		}

		recordRepositoryLatency(repoMethod, repoStatusError, elap)

		out.Error = fmt.Sprintf("%s: attempt=%d/%d error: %v",
			me, attempt, max, errPut)
		log.Print(out.Error)

		if attempt < max {
			log.Printf("%s: attempt=%d/%d sleeping %v",
				me, attempt, app.config.writeRetry, app.config.writeRetryInterval)
			time.Sleep(app.config.writeRetryInterval)
		}
	}

	c.JSON(http.StatusInternalServerError, out)
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

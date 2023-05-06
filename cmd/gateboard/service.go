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
	yaml "gopkg.in/yaml.v3"
)

const (
	repoStatusOK       = "success"
	repoStatusError    = "error"
	repoStatusNotFound = "not-found"
)

func gatewayDump(c *gin.Context, app *application) {
	const me = "gatewayDump"

	ctx, span := newSpanGin(c, me, app)
	if span != nil {
		defer span.End()
	}

	logf(ctx, "%s", me)

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
		logf(ctx, "%s: repo_dump_latency: elapsed=%v (error:%v)",
			me, elap, errDump)
	}

	const repoMethod = "dump"

	switch errDump {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, dump)
}

func repoGet(ctx context.Context, app *application, gatewayName string) (gateboard.BodyGetReply, error) {
	// create trace span
	_, span := newSpan(ctx, "repoGet", app)
	if span != nil {
		defer span.End()
	}

	body, err := app.repo.get(gatewayName)

	// record error in trace span
	if err != nil {
		traceError(span, err.Error())
	}

	return body, err
}

func repoPut(ctx context.Context, app *application, gatewayName, gatewayID string) error {
	// create trace span
	_, span := newSpan(ctx, "repoPut", app)
	if span != nil {
		defer span.End()
	}

	err := app.repo.put(gatewayName, gatewayID)

	// record error in trace span
	if err != nil {
		traceError(span, err.Error())
	}

	return err
}

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	ctx, span := newSpanGin(c, me, app)
	if span != nil {
		defer span.End()
	}

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	logf(ctx, "%s: gateway_name=%s", me, gatewayName)

	var out gateboard.BodyGetReply
	out.GatewayName = gatewayName

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		out.TTL = app.config.TTL
		out.Error = errVal.Error()
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	//
	// retrieve gateway_id
	//

	begin := time.Now()

	out, errID := repoGet(ctx, app, gatewayName)
	out.Token = "" // prevent token leaking
	out.TTL = app.config.TTL

	elap := time.Since(begin)
	if app.config.debug {
		logf(ctx, "%s: gateway_name=%s repo_get_latency: elapsed=%v (error:%v)",
			me, gatewayName, elap, errID)
	}

	const repoMethod = "get"

	switch errID {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, out)
}

func gatewayPut(c *gin.Context, app *application) {
	const me = "gatewayPut"

	ctx, span := newSpanGin(c, me, app)
	if span != nil {
		defer span.End()
	}

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	logf(ctx, "%s: gateway_name=%s", me, gatewayName)

	var out gateboard.BodyPutReply
	out.GatewayName = gatewayName

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		out.Error = errVal.Error()
		traceError(span, out.Error)
		logf(ctx, out.Error)
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
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	logf(ctx, "%s: gateway_name=%s body:%v", me, gatewayName, toJSON(in))

	out.GatewayID = in.GatewayID

	//
	// refuse blank gateway_id
	//

	gatewayID := strings.TrimSpace(in.GatewayID)
	if gatewayID == "" {
		out.Error = "invalid blank gateway_id"
		traceError(span, out.Error)
		logf(ctx, out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	out.GatewayID = gatewayID

	//
	// check write token
	//

	if app.config.writeToken {
		if invalidToken(ctx, app, gatewayName, in.Token) {
			out.Error = "invalid token"
			traceError(span, out.Error)
			logf(ctx, out.Error)
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

		errPut := repoPut(ctx, app, gatewayName, gatewayID)

		elap := time.Since(begin)
		if app.config.debug {
			logf(ctx, "%s: gateway_name=%s repo_put_latency: elapsed=%v (error:%v)",
				me, gatewayName, elap, errPut)
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
		traceError(span, out.Error)
		logf(ctx, out.Error)

		if attempt < max {
			logf(ctx, "%s: attempt=%d/%d sleeping %v",
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

func invalidToken(ctx context.Context, app *application, gatewayName, token string) bool {
	const me = "invalidToken"

	if token == "" {
		return true // empty token is always invalid
	}

	result, errID := repoGet(ctx, app, gatewayName)
	if errID != nil {
		logf(ctx, "%s: error: %v", me, errID)
		return true
	}

	return result.Token != token
}

// validateInputGatewayName checks that gatewayName is valid.
func validateInputGatewayName(gatewayName string) error {
	const me = "validateGatewayName"
	if strings.TrimSpace(gatewayName) == "" {
		return fmt.Errorf("%s: invalid blank gateway name: '%s'", me, gatewayName)
	}
	if index := strings.IndexAny(gatewayName, " ${}"); index >= 0 {
		return fmt.Errorf("%s: invalid character '%c' in gateway name: '%s'",
			me, gatewayName[index], gatewayName)
	}
	return nil
}

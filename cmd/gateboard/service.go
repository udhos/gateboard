package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/gateboard"
	"go.opentelemetry.io/otel/trace"
	yaml "gopkg.in/yaml.v3"
)

func gatewayDump(c *gin.Context, app *application) {
	const me = "gatewayDump"

	ctx, span := newSpanGin(c, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	zlog.CtxInfof(ctx, "%s", me)

	//
	// dump gateways
	//

	begin := time.Now()

	type output struct {
		Error string
	}

	var out output

	dump, errDump := app.repo.dump(ctx)

	elap := time.Since(begin)

	zlog.CtxDebugf(ctx, app.config.debug, "%s: repo_dump_latency: elapsed=%v (error:%v)",
		me, elap, errDump)

	const repoMethod = "dump"

	switch errDump {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, dump)
}

func repoGet(ctx context.Context, tracer trace.Tracer, repo repository, gatewayName string) (gateboard.BodyGetReply, error) {
	// create trace span
	ctxNew, span := newSpan(ctx, "repoGet", tracer)
	if span != nil {
		defer span.End()
	}

	body, err := repo.get(ctxNew, gatewayName)

	// record error in trace span
	if err != nil {
		traceError(span, err.Error())
	}

	return body, err
}

func repoPut(ctx context.Context, tracer trace.Tracer, repo repository, gatewayName, gatewayID string) error {
	// create trace span
	ctxNew, span := newSpan(ctx, "repoPut", tracer)
	if span != nil {
		defer span.End()
	}

	err := repo.put(ctxNew, gatewayName, gatewayID)

	// record error in trace span
	if err != nil {
		traceError(span, err.Error())
	}

	return err
}

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	ctx, span := newSpanGin(c, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	zlog.CtxInfof(ctx, "%s: gateway_name=%s", me, gatewayName)

	var out gateboard.BodyGetReply
	out.GatewayName = gatewayName

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		out.TTL = app.config.TTL
		out.Error = errVal.Error()
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	//
	// retrieve gateway_id
	//

	begin := time.Now()

	out, errID := repoGet(ctx, app.tracer, app.repo, gatewayName)
	out.Token = "" // prevent token leaking
	out.TTL = app.config.TTL

	elap := time.Since(begin)

	zlog.CtxDebugf(ctx, app.config.debug, "%s: gateway_name=%s repo_get_latency: elapsed=%v (error:%v)",
		me, gatewayName, elap, errID)

	const repoMethod = "get"

	switch errID {
	case nil:
		recordRepositoryLatency(repoMethod, repoStatusOK, elap)
	case errRepositoryGatewayNotFound:
		recordRepositoryLatency(repoMethod, repoStatusNotFound, elap)
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		recordRepositoryLatency(repoMethod, repoStatusError, elap)
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, out)
}

func gatewayPut(c *gin.Context, app *application) {
	const me = "gatewayPut"

	ctx, span := newSpanGin(c, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	zlog.CtxInfof(ctx, "%s: gateway_name=%s", me, gatewayName)

	var out gateboard.BodyPutReply
	out.GatewayName = gatewayName

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		out.Error = errVal.Error()
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
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
		zlog.CtxErrorf(ctx, out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	zlog.CtxInfof(ctx, "%s: gateway_name=%s body:%v", me, gatewayName, toJSON(ctx, in))

	out.GatewayID = in.GatewayID

	//
	// refuse blank gateway_id
	//

	gatewayID := strings.TrimSpace(in.GatewayID)
	if gatewayID == "" {
		out.Error = "invalid blank gateway_id"
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, out.Error)
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
			zlog.CtxErrorf(ctx, out.Error)
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

		errPut := repoPut(ctx, app.tracer, app.repo, gatewayName, gatewayID)

		elap := time.Since(begin)

		zlog.CtxDebugf(ctx, app.config.debug, "%s: gateway_name=%s repo_put_latency: elapsed=%v (error:%v)",
			me, gatewayName, elap, errPut)

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
		zlog.CtxErrorf(ctx, out.Error)

		if attempt < max {
			zlog.CtxInfof(ctx, "%s: attempt=%d/%d sleeping %v",
				me, attempt, app.config.writeRetry, app.config.writeRetryInterval)
			time.Sleep(app.config.writeRetryInterval)
		}
	}

	c.JSON(http.StatusInternalServerError, out)
}

func toJSON(ctx context.Context, v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		zlog.CtxErrorf(ctx, "toJSON: %v", err)
	}
	return string(b)
}

func invalidToken(ctx context.Context, app *application, gatewayName, token string) bool {
	const me = "invalidToken"

	if token == "" {
		return true // empty token is always invalid
	}

	result, errID := repoGet(ctx, app.tracer, app.repo, gatewayName)
	if errID != nil {
		zlog.CtxErrorf(ctx, "%s: error: %v", me, errID)
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

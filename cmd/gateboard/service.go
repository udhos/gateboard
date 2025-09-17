package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
	"github.com/udhos/gateboard/gateboard"
	"go.opentelemetry.io/otel/trace"
	yaml "gopkg.in/yaml.v3"
)

func randomRepo(n int) int {
	// Top-level functions, such as Float64 and Int, are safe for concurrent use by multiple goroutines.
	return rand.Intn(n)
}

// repoDumpMultiple returns merged contents from all repositories.
func repoDumpMultiple(ctx context.Context, app *application) (repoDump, error) {
	const me = "repoDumpMultiple"

	// create trace span
	ctxNew, span := newSpan(ctx, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	merge := map[string]interface{}{}
	var errLast error

	size := len(app.repoList)

	r := randomRepo(size)

	for count := 1; count <= size; count++ {
		r = (r + 1) % size
		repo := app.repoList[r]

		begin := time.Now()
		d, err := repo.dump(ctxNew)
		elap := time.Since(begin)

		if err == nil {
			recordRepositoryLatency("dump", repoStatusOK, repo.repoName(), elap)
		} else {
			errLast = err
			traceError(span, err.Error())
			recordRepositoryLatency("dump", repoStatusError, repo.repoName(), elap)
		}

		zlog.CtxDebugf(ctxNew, app.config.debug || err != nil,
			"%s: attempt=%d/%d repo=%d error:%v",
			me, count, len(app.repoList), r, err)

		// merge dump
		for _, i := range d {
			name := i["gateway_name"].(string)

			item := map[string]interface{}{
				"gateway_name": name,
				"gateway_id":   i["gateway_id"],
				"changes":      i["changes"],
				"last_update":  i["last_update"],
				"token":        i["token"],
			}

			merge[name] = item
		}
	}

	var dump repoDump

	for _, v := range merge {
		vv := v.(map[string]interface{})
		dump = append(dump, vv)
	}

	if len(dump) < 1 {
		return dump, errLast
	}

	return dump, nil
}

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

	dump, errDump := repoDumpMultiple(ctx, app)

	elap := time.Since(begin)

	zlog.CtxDebugf(ctx, app.config.debug, "%s: repo_dump_latency: elapsed=%v (error:%v)",
		me, elap, errDump)

	switch errDump {
	case nil:
	case errRepositoryGatewayNotFound:
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		out.Error = fmt.Sprintf("%s: error: %v", me, errDump)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)
		c.JSON(http.StatusInternalServerError, out)
		return
	}

	c.JSON(http.StatusOK, dump)
}

type repoAnswer struct {
	body     gateboard.BodyGetReply
	repoName string
	err      error
}

func queryOneRepo(ctx context.Context, tracer trace.Tracer, gatewayName string, r, size int, repo repository, debug bool, ch chan<- repoAnswer) {
	const me = "queryOneRepo"

	// create trace span
	ctxNew, span := newSpan(ctx, me, tracer)
	if span != nil {
		defer span.End()
	}

	begin := time.Now()
	body, err := repo.get(ctxNew, gatewayName)
	elap := time.Since(begin)

	zlog.CtxDebugf(ctxNew, debug || err != nil,
		"%s: attempt=%d/%d repo=%s gateway_name=%s error:%v",
		me, r, size, repo.repoName(), gatewayName, err)

	switch err {
	case nil:
		recordRepositoryLatency("get", repoStatusOK, repo.repoName(), elap)
	case errRepositoryGatewayNotFound:
		traceError(span, err.Error())
		recordRepositoryLatency("get", repoStatusNotFound, repo.repoName(), elap)
	default:
		traceError(span, err.Error())
		recordRepositoryLatency("get", repoStatusError, repo.repoName(), elap)
	}

	ch <- repoAnswer{body: body, repoName: repo.repoName(), err: err}
}

// repoGetMultiple returns the first non-errored result from repository list.
func repoGetMultiple(ctx context.Context, app *application, gatewayName string) (gateboard.BodyGetReply, string, error) {
	const me = "repoGetMultiple"

	// create trace span
	ctxNew, span := newSpan(ctx, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	var answer repoAnswer

	if len(app.repoList) < 1 {
		e := fmt.Errorf("%s: empty repo list", me)
		traceError(span, e.Error())
		return answer.body, "", e
	}

	size := len(app.repoList)

	var notFound bool

	ch := make(chan repoAnswer, size)

	// spawn one goroutine for each repo
	for r := range size {
		repo := app.repoList[r]
		go queryOneRepo(ctxNew, app.tracer, gatewayName, r, size, repo, app.config.debug, ch)
	}

	// get fastest answer with timeout
	for range size {
		select {
		case answer = <-ch:
			switch answer.err {
			case nil:
				return answer.body, answer.repoName, nil // done (found fastest answer)
			case errRepositoryGatewayNotFound:
				notFound = true
				// read next answer (got not found error)
			default:
				// read next answer (got other error)
			}
		case <-time.After(app.config.repoTimeout):
			e := errRepositoryTimeout
			traceError(span, e.Error())
			return answer.body, "", e // done (timeout)
		}
	}

	// All repositories errored.

	// If all repos return error, and at least one of them
	// returns not found, then not found is probably the
	// most accurate response.
	// This is useful to stabilize results for testing.
	if notFound {
		return answer.body, answer.repoName, errRepositoryGatewayNotFound
	}

	return answer.body, answer.repoName, answer.err
}

// repoPutMultiple saves in all repositories.
func repoPutMultiple(ctx context.Context, app *application, gatewayName, gatewayID string) error {
	const me = "repoPutMultiple"

	// create trace span
	ctxNew, span := newSpan(ctx, "repoPut", app.tracer)
	if span != nil {
		defer span.End()
	}

	if len(app.repoList) < 1 {
		err := fmt.Errorf("%s: empty repo list", me)
		traceError(span, err.Error())
		return err
	}

	var countSuccess int
	var errLast error

	size := len(app.repoList)

	r := randomRepo(size)

	for count := 1; count <= size; count++ {
		r = (r + 1) % size
		repo := app.repoList[r]

		begin := time.Now()
		err := repo.put(ctxNew, gatewayName, gatewayID)
		elap := time.Since(begin)

		if err == nil {
			countSuccess++
			recordRepositoryLatency("put", repoStatusOK, repo.repoName(), elap)
		} else {
			errLast = err
			traceError(span, err.Error())
			recordRepositoryLatency("put", repoStatusError, repo.repoName(), elap)
		}

		zlog.CtxDebugf(ctxNew, app.config.debug || err != nil,
			"%s: attempt=%d/%d repo=%d gateway_name=%s error:%v",
			me, count, len(app.repoList), r, gatewayName, err)
	}

	if countSuccess < 1 {
		return errLast
	}

	return nil
}

// repoPutTokenMultiple saves token in all repositories.
func repoPutTokenMultiple(ctx context.Context, app *application, gatewayName, token string) error {
	const me = "repoPutTokenMultiple"

	var errLast error

	size := len(app.repoList)

	r := randomRepo(size)

	for count := 1; count <= size; count++ {
		r = (r + 1) % size
		repo := app.repoList[r]

		err := repo.putToken(ctx, gatewayName, token)
		if err != nil {
			errLast = err
		}

		zlog.CtxDebugf(ctx, app.config.debug, "%s: attempt=%d/%d repo=%d gateway_name=%s error:%v",
			me, count, len(app.repoList), r, gatewayName, err)
	}

	return errLast
}

func isContextCanceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func gatewayGet(c *gin.Context, app *application) {
	const me = "gatewayGet"

	debugContext := app.config.debugContext

	var (
		canceledEnter       bool
		canceledAfterSpan   bool
		canceledAfterQuery  bool
		canceledAfterQuery2 bool
	)

	if debugContext {
		canceledEnter = isContextCanceled(c.Request.Context())
	}

	ctx, span := newSpanGin(c, me, app.tracer)
	if span != nil {
		defer span.End()
	}

	if debugContext {
		canceledAfterSpan = isContextCanceled(ctx)
	}

	gatewayName := strings.TrimPrefix(c.Param("gateway_name"), "/")

	zlog.CtxInfof(ctx, "%s: gateway_name=%s", me, gatewayName)

	var out gateboard.BodyGetReply
	out.GatewayName = gatewayName

	if errVal := validateInputGatewayName(gatewayName); errVal != nil {
		out.TTL = app.config.TTL
		out.Error = errVal.Error()
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)
		c.JSON(http.StatusBadRequest, out)
		return
	}

	//
	// retrieve gateway_id
	//

	begin := time.Now()

	var errID error

	// avoid gin context timeout or cancel to affect repository query
	const timeout = 5 * time.Second
	ctx2 := context.WithoutCancel(ctx)                 // avoid cancel propagation
	ctx2, cancel := context.WithTimeout(ctx2, timeout) // add timeout
	defer cancel()                                     // ensure resources are cleaned up

	if app.config.groupCache {
		// cache query
		errID = app.cache.Get(ctx2, gatewayName,
			groupcache.StringSink(&out.GatewayID), nil)
	} else {
		// direct query
		out, _, errID = repoGetMultiple(ctx2, app, gatewayName)
	}

	if debugContext {
		canceledAfterQuery = isContextCanceled(ctx)
		canceledAfterQuery2 = isContextCanceled(ctx2)
	}

	out.Token = "" // prevent token leaking
	out.TTL = app.config.TTL

	elap := time.Since(begin)

	zlog.CtxDebugf(ctx, app.config.debug, "%s: gateway_name=%s repo_get_latency: elapsed=%v (error:%v)",
		me, gatewayName, elap, errID)

	if debugContext {
		canceledAny := canceledEnter || canceledAfterSpan || canceledAfterQuery || canceledAfterQuery2
		zlog.CtxInfof(ctx, "%s: DEBUG_CONTEXT gateway_name=%s elapsed=%v cacheEnabled=%t canceledAny=%t canceledEnter=%t canceledAfterSpan=%t canceledAfterQuery=%t canceledAfterQuery2=%t (error:%v)",
			me, gatewayName, elap, app.config.groupCache, canceledAny,
			canceledEnter, canceledAfterSpan, canceledAfterQuery,
			canceledAfterQuery2, errID)
	}

	switch errID {
	case nil:
	case errRepositoryGatewayNotFound:
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: not found: %v", me, errID)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)
		c.JSON(http.StatusNotFound, out)
		return
	default:
		out.GatewayName = gatewayName
		out.Error = fmt.Sprintf("%s: error: %v", me, errID)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)
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
		zlog.CtxErrorf(ctx, "%s", out.Error)
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
		zlog.CtxErrorf(ctx, "%s", out.Error)
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
		zlog.CtxErrorf(ctx, "%s", out.Error)
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
			zlog.CtxErrorf(ctx, "%s", out.Error)
			c.JSON(http.StatusUnauthorized, out)
			return
		}
	}

	//
	// save gateway_id
	//

	maxRetry := app.config.writeRetry

	for attempt := 1; attempt <= maxRetry; attempt++ {

		begin := time.Now()

		errPut := repoPutMultiple(ctx, app, gatewayName, gatewayID)

		elap := time.Since(begin)

		zlog.CtxDebugf(ctx, app.config.debug, "%s: gateway_name=%s repo_put_latency: elapsed=%v (error:%v)",
			me, gatewayName, elap, errPut)

		if errPut == nil {

			// PUT success

			if app.config.groupCache {
				app.cache.Remove(ctx, gatewayName)
			}

			out.Error = ""
			c.JSON(http.StatusOK, out)
			return
		}

		out.Error = fmt.Sprintf("%s: attempt=%d/%d error: %v",
			me, attempt, maxRetry, errPut)
		traceError(span, out.Error)
		zlog.CtxErrorf(ctx, "%s", out.Error)

		if attempt < maxRetry {
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

	result, _, errID := repoGetMultiple(ctx, app, gatewayName)
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

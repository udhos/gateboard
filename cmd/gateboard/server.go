package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/udhos/gateboard/cmd/gateboard/zlog"
)

type serverGin struct {
	server *http.Server
	router *gin.Engine
}

func newServerGin(addr string) *serverGin {
	r := gin.New()
	return &serverGin{
		router: r,
		server: &http.Server{Addr: addr, Handler: r},
	}
}

func (s *serverGin) shutdown(label string, timeout time.Duration) {
	httpShutdown(s.server, label, timeout)
}

func httpShutdown(s *http.Server, label string, timeout time.Duration) {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		zlog.Infof("http shutdown error: %s: %v", label, err)
	}
}

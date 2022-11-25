package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

/*
type serverHTTP struct {
	server *http.Server
}

func newServerHTTP(addr string, handler http.Handler) *serverHTTP {
	return &serverHTTP{
		server: &http.Server{Addr: addr, Handler: handler},
	}
}

func (s *serverHTTP) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
*/

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

func (s *serverGin) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

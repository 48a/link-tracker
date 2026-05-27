package server

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Server struct {
	srv *http.Server
}

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 30 * time.Second
)

func NewServer(baseURL string, h *Handler) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /updates", h.SendUpdate)

	return &Server{srv: &http.Server{
		Addr:         baseURL,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  time.Minute,
	}}
}

func (s *Server) Run() error {
	return fmt.Errorf("server: %w", s.srv.ListenAndServe())
}

func (s *Server) Stop(ctx context.Context) error {
	return fmt.Errorf("stop server: %w", s.srv.Shutdown(ctx))
}

package server

import (
	"context"
	"net/http"
	"time"
)

type Server struct {
	srv *http.Server
}

func NewServer(baseURL string, h *Handler) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /updates", h.SendUpdate)

	return &Server{srv: &http.Server{
		Addr:         baseURL,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  time.Minute,
	}}
}

func (s *Server) Run() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) {
	s.srv.Shutdown(ctx)
}

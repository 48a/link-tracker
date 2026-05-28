package server

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	readTimeout  = time.Duration(10) * time.Second
	writeTimeout = time.Duration(30) * time.Second
)

type Server struct {
	srv *http.Server
}

func NewServer(baseURL string, h *Handler) Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tg-chat/{id}", h.RegisterChat)
	mux.HandleFunc("DELETE /tg-chat/{id}", h.DeleteChat)
	mux.HandleFunc("GET /links", h.GetLinks)
	mux.HandleFunc("POST /links", h.AddLink)
	mux.HandleFunc("DELETE /links", h.DeleteLink)

	return Server{srv: &http.Server{
		Addr:         baseURL,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  time.Minute,
	}}
}

func (s Server) Run() error {
	return fmt.Errorf("listen and serve: %w", s.srv.ListenAndServe())
}

func (s Server) Stop(ctx context.Context) error {
	return fmt.Errorf("shutdown server: %w", s.srv.Shutdown(ctx))
}

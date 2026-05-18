package server

import (
	"net/http"
	"time"
)

type server struct {
	srv *http.Server
}

func NewServer(baseURL string, h *handler) server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /updates", h.SendUpdate)

	return server{srv: &http.Server{
		Addr:         baseURL,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  time.Minute,
	}}
}

func (s server) Run() error {
	return s.srv.ListenAndServe()
}

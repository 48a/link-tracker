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
	mux.HandleFunc("POST /tg-chat/{id}", h.RegisterChat)
	mux.HandleFunc("DELETE /tg-chat/{id}", h.DeleteChat)
	mux.HandleFunc("GET /links", h.GetLinks)
	mux.HandleFunc("POST /links", h.AddLink)
	mux.HandleFunc("DELETE /links", h.DeleteLink)

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

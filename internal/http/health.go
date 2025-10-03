// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package httpserver

import (
	"context"
	"fmt"
	"net/http"
)

type ReadyChecker interface {
	Ready() bool
}

type Server struct {
	srv     *http.Server
	fetcher ReadyChecker
}

func NewHealthServer(port int, fetcher ReadyChecker) *Server {
	s := &Server{fetcher: fetcher}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if s.fetcher != nil && s.fetcher.Ready() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready")
		}
	})

	return &Server{
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		fetcher: fetcher,
	}
}

func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

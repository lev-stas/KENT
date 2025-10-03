// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"context"
	"event_exporter/internal/app"
	"event_exporter/internal/config"
	"event_exporter/internal/pkg/logger"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var cfg config.Config
	log := logger.New("info")

	if err := config.Load(&cfg); err != nil {
		log.Error(context.Background(), "failed to load config", "error", err)
		os.Exit(1)
	}

	//re-init logger with log level from config
	log = logger.New(cfg.Logger.Level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		log.Error(context.Background(), "exporter stopped with error", "error", err)
		os.Exit(1)
	}

	log.Info(context.Background(), "exporter exited cleanly")
}

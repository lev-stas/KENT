// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package app

import (
	"context"
	"event_exporter/internal/adapters/kubernetes"
	"event_exporter/internal/adapters/victorialogs"
	"event_exporter/internal/config"
	httpserver "event_exporter/internal/http"
	"event_exporter/internal/pkg/logger"
	"event_exporter/internal/usecase"
	"fmt"
	"net/http"
	"time"
)

func Run(ctx context.Context, cfg config.Config) error {
	log := logger.New(cfg.Logger.Level)

	fetcher, err := kubernetes.NewFetcher(
		log,
		cfg.Kubernetes.IncludeNamespaces,
		cfg.Kubernetes.ExcludeNamespaces,
	)

	if err != nil {
		return fmt.Errorf("app: failed to init fetcher: %w", err)
	}

	victoriaLogConfig := victorialogs.VictoriaLogsConfig{
		Enabled:      cfg.VictoriaLogs.Enabled,
		Endpoint:     cfg.VictoriaLogs.Endpoint,
		ClusterID:    cfg.VictoriaLogs.ClusterID,
		AccountID:    cfg.VictoriaLogs.AccountID,
		ProjectID:    cfg.VictoriaLogs.ProjectID,
		BatchSize:    cfg.VictoriaLogs.BatchSize,
		FlushTime:    cfg.VictoriaLogs.FlushTime,
		ExtraFields:  cfg.VictoriaLogs.ExtraFields,
		Timeout:      cfg.VictoriaLogs.Timeout,
		StreamFields: cfg.VictoriaLogs.StreamFields,
	}

	var writers []usecase.LogWriter

	victoriaWriter, err := victorialogs.NewWriter(victoriaLogConfig, log)
	if err != nil {
		return fmt.Errorf("app: failed to init victorialogs writer: %w", err)
	}

	if victoriaWriter != nil {
		writers = append(writers, victoriaWriter)
	}

	collector := usecase.NewCollector(fetcher, writers, log)

	healthSvs := httpserver.NewHealthServer(cfg.HealthConfig.Port, fetcher)

	go func() {
		if err := healthSvs.Start(); err != nil && err != http.ErrServerClosed {
			log.Error(ctx, "app: health server stopped", "error", err)
		}
	}()

	go func() {
		if err := collector.Run(ctx); err != nil && err != context.Canceled {
			log.Error(ctx, "app: collector stopped", "error", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := healthSvs.Stop(shutdownCtx); err != nil {
		log.Error(context.Background(), "app: failed to stop health server", "error", err)
	}

	if victoriaWriter != nil {
		victoriaWriter.Stop()
	}

	log.Info(context.Background(), "app: shutdown complete")

	return nil
}

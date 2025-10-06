// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package app

import (
	"context"
	k8sfetcher "event_exporter/internal/adapters/kubernetes"
	"event_exporter/internal/adapters/victorialogs"
	"event_exporter/internal/config"
	httpserver "event_exporter/internal/http"
	"event_exporter/internal/pkg/logger"
	"event_exporter/internal/usecase"
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func Run(ctx context.Context, cfg config.Config) error {
	log := logger.New(cfg.Logger.Level)

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("app: cannot build in-cluster config; fallback to core/v1: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("app: cannot create kube client; fallback to core/v1: %w", err)
	}

	fetcher, err := chooseFetcher(ctx, log, cfg.Kubernetes.IncludeNamespaces, cfg.Kubernetes.ExcludeNamespaces, cs)

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

	var ready httpserver.ReadyChecker

	if rc, ok := fetcher.(httpserver.ReadyChecker); ok {
		ready = rc
	}

	healthSvs := httpserver.NewHealthServer(cfg.HealthConfig.Port, ready)

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

func chooseFetcher(
	ctx context.Context,
	log logger.Logger,
	include []string,
	exclude []string,
	client *kubernetes.Clientset,
) (usecase.EventFetcher, error) {

	ok, err := supportsEventsV1(client)
	if err != nil {
		log.Warn(ctx, "app: events API detection failed; fallback to core/v1", "error", err)
		return k8sfetcher.NewFetcher(log, include, exclude, client)
	}

	if !ok {
		log.Info(ctx, "app: events.k8s.io/v1 not available; using core/v1/events")
		return k8sfetcher.NewFetcher(log, include, exclude, client)
	}

	log.Info(ctx, "app: using events.k8s.io/v1 API for event collection")
	return k8sfetcher.NewFetcherV1(log, include, exclude, client)
}

func supportsEventsV1(dc discovery.DiscoveryInterface) (bool, error) {
	groupList, err := dc.ServerGroups()
	if err != nil {
		return false, err
	}

	for _, g := range groupList.Groups {
		if g.Name == "events.k8s.io" {
			for _, v := range g.Versions {
				if v.Version == "v1" {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

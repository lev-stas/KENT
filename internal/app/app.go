// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package app

import (
	"context"
	"errors"
	k8sfetcher "event_exporter/internal/adapters/kubernetes"
	stdoutwriter "event_exporter/internal/adapters/stdout"
	"event_exporter/internal/adapters/victorialogs"
	"event_exporter/internal/config"
	"event_exporter/internal/domain"
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

	filter, err := buildEventFilter(cfg)
	if err != nil {
		return err
	}

	// The effective filter is the first thing to check when events stop
	// arriving, and a config typo is invisible otherwise. It is reported by
	// the filter itself, so what is logged is what is enforced.
	log.Info(ctx, "app: event filter configured", filter.LogFields()...)

	fetcher, err := chooseFetcher(ctx, log, filter, cs)

	if err != nil {
		return fmt.Errorf("app: failed to init fetcher: %w", err)
	}

	writers, stopWriters, err := buildWriters(cfg, log)
	if err != nil {
		return err
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

	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		if err := collector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error(ctx, "app: collector stopped", "error", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := healthSvs.Stop(shutdownCtx); err != nil {
		log.Error(context.Background(), "app: failed to stop health server", "error", err)
	}

	// Let the collector drain already-fetched events into the writers before
	// stopping them; bounded so a stuck writer cannot hang shutdown forever.
	select {
	case <-collectorDone:
	case <-time.After(10 * time.Second):
		log.Warn(context.Background(), "app: collector did not finish draining in time")
	}

	stopWriters()

	log.Info(context.Background(), "app: shutdown complete")

	return nil
}

func buildWriters(cfg config.Config, log logger.Logger) ([]usecase.LogWriter, func(), error) {
	var (
		writers  []usecase.LogWriter
		stoppers []func()
	)

	register := func(writer usecase.LogWriter) {
		if writer == nil {
			return
		}

		writers = append(writers, writer)

		if stopper, ok := writer.(interface{ Stop() }); ok {
			stoppers = append(stoppers, stopper.Stop)
		}
	}

	victoriaWriter, err := victorialogs.NewWriter(victorialogs.VictoriaLogsConfig{
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
		QueueSize:    cfg.VictoriaLogs.QueueSize,
		Headers:      cfg.VictoriaLogs.Headers,
		Auth: victorialogs.AuthConfig{
			BasicUsername: cfg.VictoriaLogs.Auth.BasicUsername,
			BasicPassword: cfg.VictoriaLogs.Auth.BasicPassword,
			BearerToken:   cfg.VictoriaLogs.Auth.BearerToken,
		},
		TLS: victorialogs.TLSConfig{
			InsecureSkipVerify: cfg.VictoriaLogs.TLS.InsecureSkipVerify,
			CAFile:             cfg.VictoriaLogs.TLS.CAFile,
			CertFile:           cfg.VictoriaLogs.TLS.CertFile,
			KeyFile:            cfg.VictoriaLogs.TLS.KeyFile,
			ServerName:         cfg.VictoriaLogs.TLS.ServerName,
		},
	}, log)
	if err != nil {
		return nil, nil, fmt.Errorf("app: failed to init victorialogs writer: %w", err)
	}
	if victoriaWriter != nil {
		register(victoriaWriter)
	}

	stdoutWriter, err := stdoutwriter.NewWriter(stdoutwriter.Config{
		Enabled: cfg.Stdout.Enabled,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("app: failed to init stdout writer: %w", err)
	}
	if stdoutWriter != nil {
		register(stdoutWriter)
	}

	return writers, func() {
		for _, stop := range stoppers {
			stop()
		}
	}, nil
}

func buildEventFilter(cfg config.Config) (domain.EventFilter, error) {
	filter, err := domain.NewEventFilter(domain.FilterSpec{
		IncludeNamespaces: cfg.Kubernetes.IncludeNamespaces,
		ExcludeNamespaces: cfg.Kubernetes.ExcludeNamespaces,
		IncludeEventTypes: cfg.Kubernetes.IncludeEventTypes,
		IncludeKinds:      cfg.Kubernetes.IncludeKinds,
		IncludeReasons:    cfg.Kubernetes.IncludeReasons,
		ExcludeReasons:    cfg.Kubernetes.ExcludeReasons,
	})
	if err != nil {
		return domain.EventFilter{}, fmt.Errorf("app: invalid event filter: %w", err)
	}

	return filter, nil
}

func chooseFetcher(
	ctx context.Context,
	log logger.Logger,
	filter domain.EventFilter,
	client *kubernetes.Clientset,
) (usecase.EventFetcher, error) {

	ok, err := supportsEventsV1(client)
	if err != nil {
		log.Warn(ctx, "app: events API detection failed; fallback to core/v1", "error", err)
		return k8sfetcher.NewFetcher(log, filter, client)
	}

	if !ok {
		log.Info(ctx, "app: events.k8s.io/v1 not available; using core/v1/events")
		return k8sfetcher.NewFetcher(log, filter, client)
	}

	log.Info(ctx, "app: using events.k8s.io/v1 API for event collection")
	return k8sfetcher.NewFetcherV1(log, filter, client)
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

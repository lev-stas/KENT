// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package kubernetes

import (
	"context"
	"event_exporter/internal/domain"
	"fmt"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type Fetcher struct {
	client    *kubernetes.Clientset
	logger    Logger
	includeNS map[string]struct{}
	excludeNS map[string]struct{}
	ready     atomic.Bool
}

func (f *Fetcher) Ready() bool {
	return f.ready.Load()
}

func NewFetcher(logger Logger, include []string, exclude []string) (*Fetcher, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("adapters:kubernetes:fetcher: failed to get in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("adapters:kubernetes:fetcher: failed to create clientset: %w", err)
	}

	return &Fetcher{
		client:    client,
		logger:    logger,
		includeNS: toSet(include),
		excludeNS: toSet(exclude),
	}, nil
}

func (f *Fetcher) Stream(ctx context.Context, out chan<- *domain.Event) error {
	f.ready.Store(true)
	defer f.ready.Store(false)

	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		watcher, err := f.client.CoreV1().Events("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			f.logger.Error(ctx, "adapters:kubernetes:fetcher: failed to start watch", "error", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second

		for evt := range watcher.ResultChan() {
			k8sEvent, ok := evt.Object.(*corev1.Event)
			if !ok {
				continue
			}

			domainEvent, err := mapK8sEventToDomain(k8sEvent)
			if err != nil {
				f.logger.Warn(ctx, "adapters:kubernetes:fetcher: failed to map event", "error", err)
				continue
			}

			f.logger.Debug(ctx, "adapters:kubernetes:fetcher: received event",
				"namespace", domainEvent.Namespace(),
				"name", domainEvent.Name(),
				"reason", domainEvent.Reason(),
				"type", domainEvent.Type(),
				"message", domainEvent.Message(),
			)

			ns := domainEvent.Namespace()

			if len(f.includeNS) > 0 {
				if _, ok := f.includeNS[ns]; !ok {
					continue
				}
			}

			if _, ok := f.excludeNS[ns]; ok {
				continue
			}

			select {
			case <-ctx.Done():
				watcher.Stop()
				return ctx.Err()
			case out <- domainEvent:
			}
		}

		if ctx.Err() != nil {
			watcher.Stop()
			return ctx.Err()
		}

		f.logger.Info(ctx, "adapters:kubernetes:fetcher: watch channel closed, reconnecting...")
		time.Sleep(backoff)

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func mapK8sEventToDomain(e *corev1.Event) (*domain.Event, error) {
	return domain.NewEvent(
		string(e.UID),
		e.Name,
		e.Namespace,
		e.Reason,
		e.Message,
		e.Type,
		domain.ObjectRef{
			Kind:      e.InvolvedObject.Kind,
			Name:      e.InvolvedObject.Name,
			Namespace: e.InvolvedObject.Namespace,
		},
		e.Source.Component,
		e.FirstTimestamp.Time,
		timePtr(e.LastTimestamp.Time),
		e.Count,
	)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func toSet(list []string) map[string]struct{} {
	set := make(map[string]struct{}, len(list))
	for _, v := range list {
		set[v] = struct{}{}
	}
	return set
}

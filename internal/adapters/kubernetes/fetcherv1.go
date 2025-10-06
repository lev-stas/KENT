// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0

package kubernetes

import (
	"context"
	"event_exporter/internal/domain"
	"sync/atomic"
	"time"

	eventv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type LoggerV1 interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type FetcherV1 struct {
	client    *kubernetes.Clientset
	logger    LoggerV1
	includeNS map[string]struct{}
	excludeNS map[string]struct{}
	ready     atomic.Bool
}

func (f *FetcherV1) Ready() bool {
	return f.ready.Load()
}

func NewFetcherV1(logger LoggerV1, include []string, exclude []string, client *kubernetes.Clientset) (*FetcherV1, error) {
	return &FetcherV1{
		client:    client,
		logger:    logger,
		includeNS: toSet(include),
		excludeNS: toSet(exclude),
	}, nil

}

func (f *FetcherV1) Stream(ctx context.Context, out chan<- *domain.Event) error {
	f.ready.Store(true)
	defer f.ready.Store(false)

	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		watcher, err := f.client.EventsV1().Events("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			f.logger.Error(ctx, "adapters:kubernetes:fetcherv1: failed to start watch", "error", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second

		for evt := range watcher.ResultChan() {
			k8sEvent, ok := evt.Object.(*eventv1.Event)
			if !ok {
				continue
			}

			domainEvent, err := mapK8sEventV1ToDomain(k8sEvent)
			if err != nil {
				f.logger.Warn(ctx, "adapters:kubernetes:fetcherv1: failed to map event", "error", err)
				continue
			}

			f.logger.Debug(
				ctx, "adapters:kubernetes:fetcherv1: received event",
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

		f.logger.Info(ctx, "adapters:kubernetes:fetcherv1: watch channel closed, reconnecting...")
		time.Sleep(backoff)

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func mapK8sEventV1ToDomain(e *eventv1.Event) (*domain.Event, error) {
	obj := domain.ObjectRef{
		Kind:      e.Regarding.Kind,
		Name:      e.Regarding.Name,
		Namespace: e.Regarding.Namespace,
	}

	eventTime := extractEventTime(e)
	lastTime := timePtr(e.DeprecatedLastTimestamp.Time)

	return domain.NewEvent(
		string(e.UID),
		e.Name,
		e.Namespace,
		e.Reason,
		e.Note,
		e.Type,
		obj,
		e.ReportingController,
		eventTime,
		lastTime,
		safeCount(e),
	)
}

func safeCount(e *eventv1.Event) int32 {
	if e.Series != nil {
		return e.Series.Count
	}
	if e.DeprecatedCount > 0 {
		return e.DeprecatedCount
	}
	return 1
}

func extractEventTime(e *eventv1.Event) time.Time {
	if !e.EventTime.Time.IsZero() {
		return e.EventTime.Time
	}
	if !e.DeprecatedFirstTimestamp.Time.IsZero() {
		return e.DeprecatedFirstTimestamp.Time
	}

	return time.Now().UTC()
}

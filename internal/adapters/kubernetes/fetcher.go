// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package kubernetes

import (
	"context"
	"event_exporter/internal/domain"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type Fetcher struct {
	client    kubernetes.Interface
	logger    Logger
	includeNS map[string]struct{}
	excludeNS map[string]struct{}
	ready     atomic.Bool
}

func (f *Fetcher) Ready() bool {
	return f.ready.Load()
}

func NewFetcher(logger Logger, include []string, exclude []string, client kubernetes.Interface) (*Fetcher, error) {
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

	session := &watchSession{
		logger: f.logger,
		scope:  "adapters:kubernetes:fetcher",
		startWatch: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			return f.client.CoreV1().Events("").Watch(ctx, opts)
		},
		mapEvent: func(obj runtime.Object) (*domain.Event, error) {
			k8sEvent, ok := obj.(*corev1.Event)
			if !ok {
				return nil, nil
			}
			return mapK8sEventToDomain(k8sEvent)
		},
		includeNS: f.includeNS,
		excludeNS: f.excludeNS,
	}

	return session.run(ctx, out)
}

func mapK8sEventToDomain(e *corev1.Event) (*domain.Event, error) {

	obj := domain.ObjectRef{
		Kind:      e.InvolvedObject.Kind,
		Name:      e.InvolvedObject.Name,
		Namespace: e.InvolvedObject.Namespace,
	}

	eventTime := extractCoreEventTime(e)
	lastTime := extractCoreLastTime(e)

	return domain.NewEvent(
		string(e.UID),
		e.Name,
		e.Namespace,
		e.Reason,
		e.Message,
		e.Type,
		obj,
		e.Source.Component,
		eventTime,
		lastTime,
		safeCoreCount(e),
	)
}

// Events recorded through events.k8s.io/v1 recorders surface via core/v1 with
// Count=0 and the real counter in Series; mirror the events/v1 fallbacks so
// such events keep valid timestamps and dedup-relevant counts.
func safeCoreCount(e *corev1.Event) int32 {
	if e.Series != nil {
		return e.Series.Count
	}
	if e.Count > 0 {
		return e.Count
	}
	return 1
}

func extractCoreEventTime(e *corev1.Event) time.Time {
	if !e.FirstTimestamp.Time.IsZero() {
		return e.FirstTimestamp.Time
	}
	if !e.EventTime.Time.IsZero() {
		return e.EventTime.Time
	}
	return time.Now().UTC()
}

func extractCoreLastTime(e *corev1.Event) *time.Time {
	if !e.LastTimestamp.Time.IsZero() {
		return timePtr(e.LastTimestamp.Time)
	}
	if e.Series != nil && !e.Series.LastObservedTime.Time.IsZero() {
		return timePtr(e.Series.LastObservedTime.Time)
	}
	return nil
}

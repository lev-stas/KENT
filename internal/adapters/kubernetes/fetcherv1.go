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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type FetcherV1 struct {
	client kubernetes.Interface
	logger Logger
	filter domain.EventFilter
	ready  atomic.Bool
}

func (f *FetcherV1) Ready() bool {
	return f.ready.Load()
}

func NewFetcherV1(logger Logger, filter domain.EventFilter, client kubernetes.Interface) (*FetcherV1, error) {
	return &FetcherV1{
		client: client,
		logger: logger,
		filter: filter,
	}, nil

}

func (f *FetcherV1) Stream(ctx context.Context, out chan<- *domain.Event) error {
	f.ready.Store(true)
	defer f.ready.Store(false)

	session := &watchSession{
		logger: f.logger,
		scope:  "adapters:kubernetes:fetcherv1",
		startWatch: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			return f.client.EventsV1().Events("").Watch(ctx, opts)
		},
		mapEvent: func(obj runtime.Object) (*domain.Event, error) {
			k8sEvent, ok := obj.(*eventv1.Event)
			if !ok {
				return nil, nil
			}
			return mapK8sEventV1ToDomain(k8sEvent)
		},
		filter: f.filter,
	}

	return session.run(ctx, out)
}

func mapK8sEventV1ToDomain(e *eventv1.Event) (*domain.Event, error) {
	obj := domain.ObjectRef{
		Kind:      e.Regarding.Kind,
		Name:      e.Regarding.Name,
		Namespace: e.Regarding.Namespace,
	}

	return domain.NewEvent(domain.EventInput{
		UID:               string(e.UID),
		Name:              e.Name,
		Namespace:         e.Namespace,
		Reason:            e.Reason,
		Message:           e.Note,
		Type:              e.Type,
		Object:            obj,
		Source:            e.ReportingController,
		Action:            e.Action,
		ReportingInstance: extractReportingInstance(e),
		EventTime:         extractEventTime(e),
		LastTimestamp:     extractLastTime(e),
		Count:             safeCount(e),
	})
}

// extractReportingInstance falls back to the deprecated source host: events
// mirrored from core/v1 recorders carry the reporting node only there.
func extractReportingInstance(e *eventv1.Event) string {
	if e.ReportingInstance != "" {
		return e.ReportingInstance
	}
	return e.DeprecatedSource.Host
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

// extractLastTime prefers the series' last-observed time: for recurring
// events it tracks the latest occurrence, which DeprecatedLastTimestamp
// may miss for series-style recorders.
func extractLastTime(e *eventv1.Event) *time.Time {
	if e.Series != nil && !e.Series.LastObservedTime.Time.IsZero() {
		return timePtr(e.Series.LastObservedTime.Time)
	}
	return timePtr(e.DeprecatedLastTimestamp.Time)
}

// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package kubernetes

import (
	"context"
	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/metrics"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

// watchSession runs the shared watch-reconnect loop for both event APIs.
// It resumes from the last seen resourceVersion after reconnects, so events
// emitted while disconnected are not lost, and falls back to a fresh watch
// when the stored resourceVersion has expired (HTTP 410 Gone).
type watchSession struct {
	logger     Logger
	scope      string // log message prefix, e.g. "adapters:kubernetes:fetcher"
	startWatch func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	// mapEvent converts a watch object to a domain event; (nil, nil) means
	// the object is not an event of the watched type and must be skipped.
	mapEvent func(obj runtime.Object) (*domain.Event, error)
	// filter drops events the operator is not interested in. Its zero value
	// exports everything.
	filter domain.EventFilter

	resourceVersion string
	initialBackoff  time.Duration
	maxBackoff      time.Duration
}

func (s *watchSession) run(ctx context.Context, out chan<- *domain.Event) error {
	if s.initialBackoff <= 0 {
		s.initialBackoff = time.Second
	}
	if s.maxBackoff <= 0 {
		s.maxBackoff = 30 * time.Second
	}

	backoff := s.initialBackoff

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		watcher, err := s.startWatch(ctx, metav1.ListOptions{
			ResourceVersion:     s.resourceVersion,
			AllowWatchBookmarks: true,
		})
		if err != nil {
			if s.resourceVersion != "" && (apierrors.IsResourceExpired(err) || apierrors.IsGone(err)) {
				s.logger.Warn(ctx, s.scope+": resourceVersion expired, restarting from latest state")
				s.resourceVersion = ""
				continue
			}
			s.logger.Error(ctx, s.scope+": failed to start watch", "error", err)
			if !s.sleep(ctx, backoff) {
				return ctx.Err()
			}
			backoff = min(backoff*2, s.maxBackoff)
			continue
		}

		backoff = s.initialBackoff

		expired := s.consume(ctx, watcher, out)
		watcher.Stop()

		if ctx.Err() != nil {
			return ctx.Err()
		}

		metrics.WatchReconnects.Inc()

		if expired {
			s.logger.Warn(ctx, s.scope+": resourceVersion expired mid-watch, restarting from latest state")
			s.resourceVersion = ""
			continue
		}

		s.logger.Info(ctx, s.scope+": watch channel closed, reconnecting...",
			"resume_resource_version", s.resourceVersion)

		if !s.sleep(ctx, backoff) {
			return ctx.Err()
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

// consume reads the watch channel until it closes or ctx is cancelled.
// It returns true when the server reported an expired resourceVersion and
// the next watch must start from the latest state.
func (s *watchSession) consume(ctx context.Context, watcher watch.Interface, out chan<- *domain.Event) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return false
			}

			if evt.Type == watch.Error {
				statusErr := apierrors.FromObject(evt.Object)
				if apierrors.IsResourceExpired(statusErr) || apierrors.IsGone(statusErr) {
					return true
				}
				s.logger.Error(ctx, s.scope+": watch error, reconnecting", "error", statusErr)
				return false
			}

			if obj, err := meta.Accessor(evt.Object); err == nil {
				if rv := obj.GetResourceVersion(); rv != "" {
					s.resourceVersion = rv
				}
			}

			// Bookmarks only carry a fresh resourceVersion; deletions of Event
			// objects (TTL cleanup) are not new cluster events.
			if evt.Type == watch.Bookmark || evt.Type == watch.Deleted {
				continue
			}

			domainEvent, err := s.mapEvent(evt.Object)
			if err != nil {
				s.logger.Warn(ctx, s.scope+": failed to map event", "error", err)
				continue
			}
			if domainEvent == nil {
				continue
			}

			metrics.EventsReceived.Inc()

			s.logger.Debug(ctx, s.scope+": received event",
				"namespace", domainEvent.Namespace(),
				"name", domainEvent.Name(),
				"reason", domainEvent.Reason(),
				"type", domainEvent.Type(),
			)

			if !s.filter.Allow(domainEvent) {
				metrics.EventsFiltered.Inc()
				continue
			}

			select {
			case <-ctx.Done():
				return false
			case out <- domainEvent:
			}
		}
	}
}

func (s *watchSession) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package usecase

import (
	"context"
	"errors"
	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/metrics"
	"strconv"
	"time"
)

// dedupCacheSize bounds dedup memory to two generations of this many UIDs.
const dedupCacheSize = 8192

// drainGracePeriod bounds how long writes may continue after shutdown starts,
// so already-fetched events reach the writers without hanging termination.
const drainGracePeriod = 5 * time.Second

type EventFetcher interface {
	Stream(ctx context.Context, out chan<- *domain.Event) error
}

// LogWriter delivers converted events. Write may block: writers apply
// backpressure when their internal queue is full (e.g. during a destination
// outage). That intentionally pauses the whole pipeline — including other
// writers — instead of dropping events.
type LogWriter interface {
	Write(ctx context.Context, logs []*domain.LogEntry) error
}

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type Collector struct {
	fetcher EventFetcher
	writers []LogWriter
	logger  Logger
}

func NewCollector(fetcher EventFetcher, writers []LogWriter, logger Logger) *Collector {
	return &Collector{
		fetcher: fetcher,
		writers: writers,
		logger:  logger,
	}
}

func (c *Collector) Run(ctx context.Context) error {

	if len(c.writers) == 0 {
		c.logger.Warn(ctx, "usecase:collector: no active writers configured — events will be ignored")
	}

	events := make(chan *domain.Event, 100)
	dedup := newDedupCache(dedupCacheSize)

	go func() {
		defer close(events)
		if err := c.fetcher.Stream(ctx, events); err != nil && !errors.Is(err, context.Canceled) {
			c.logger.Error(ctx, "usecase:collector: fetcher stream stopped", "error", err)
		}
	}()

	// writeCtx survives ctx cancellation for drainGracePeriod, so events
	// already fetched from the watch are handed to the writers on shutdown
	// instead of being dropped.
	writeCtx, cancelWrites := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWrites()
	go func() {
		select {
		case <-ctx.Done():
		case <-writeCtx.Done():
			return
		}
		select {
		case <-time.After(drainGracePeriod):
			cancelWrites()
		case <-writeCtx.Done():
		}
	}()

	entries := make([]*domain.LogEntry, 1)

	for ev := range events {
		if dedup.isDuplicate(ev.UID(), ev.Count()) {
			metrics.EventsDeduplicated.Inc()
			c.logger.Debug(ctx, "usecase:collector: duplicate event skipped",
				"uid", ev.UID(), "count", ev.Count())
			continue
		}

		logEntry, err := convertEventToLogEntry(ev)
		if err != nil {
			c.logger.Error(ctx, "failed to convert event to log entry", "error", err)
			continue
		}

		if len(c.writers) == 0 {
			continue
		}

		entries[0] = logEntry
		for _, w := range c.writers {
			if w == nil {
				continue
			}
			if err := w.Write(writeCtx, entries); err != nil {
				c.logger.Error(ctx, "usecase:collector: failed to write log entry", "error", err)
			}
		}
	}

	return ctx.Err()
}

func convertEventToLogEntry(e *domain.Event) (*domain.LogEntry, error) {
	fields := map[string]string{
		"k8s.namespace": e.Namespace(),
		"k8s.name":      e.Name(),
		"k8s.kind":      e.Object().Kind,
		"event.reason":  e.Reason(),
		"event.type":    e.Type(),
		"event.source":  e.Source(),
		"event.count":   strconv.FormatInt(int64(e.Count()), 10),
	}

	level := mapEventTypeToLevel(e.Type())
	logType := "event" //Hardcoded type. In future may be several types.

	// For recurring events each count increment is a fresh occurrence: stamp
	// it with the last-observed time, not the first one — otherwise records
	// of a long-lived event all land at its first occurrence in the past.
	ts := e.EventTime()
	if lt := e.LastTimestamp(); lt != nil && lt.After(ts) {
		ts = *lt
	}

	return domain.NewLogEntry(
		ts,
		level,
		logType,
		e.Message(),
		fields,
	)

}

func mapEventTypeToLevel(eventType string) string {
	// See: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#event-v1-core

	switch eventType {
	case "Warning":
		return "warning"
	case "Normal":
		return "info"
	default:
		return "info"
	}
}

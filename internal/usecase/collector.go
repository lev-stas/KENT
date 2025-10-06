// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package usecase

import (
	"context"
	"event_exporter/internal/domain"
	"fmt"
)

type EventFetcher interface {
	Stream(ctx context.Context, out chan<- *domain.Event) error
}

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

	go func() {
		if err := c.fetcher.Stream(ctx, events); err != nil {
			c.logger.Error(ctx, "usecase:collector: fetcher stream stopped", "error", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}

			logEntry, err := convertEventToLogEntry(ev)
			if err != nil {
				c.logger.Error(ctx, "failed to convert event to log entry", "error", err)
				continue
			}

			if len(c.writers) == 0 {
				continue
			}

			entries := []*domain.LogEntry{logEntry}
			for _, w := range c.writers {
				if w == nil {
					continue
				}
				if err := w.Write(ctx, entries); err != nil {
					c.logger.Error(ctx, "usecase:collector: failed to write log entry", "error", err)
				}
			}

		}
	}
}

func convertEventToLogEntry(e *domain.Event) (*domain.LogEntry, error) {
	fields := map[string]string{
		"k8s.namespace": e.Namespace(),
		"k8s.name":      e.Name(),
		"k8s.kind":      e.Object().Kind,
		"event.reason":  e.Reason(),
		"event.type":    e.Type(),
		"event.source":  e.Source(),
		"event.count":   fmt.Sprintf("%d", e.Count()),
	}

	level := mapEventTypeToLevel(e.Type())
	logType := "event" //Hardcoded type. In future may be several types.

	return domain.NewLogEntry(
		e.EventTime(),
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

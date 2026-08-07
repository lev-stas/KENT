// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package stdout

import (
	"context"
	"encoding/json"
	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/metrics"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const writerName = "stdout"

type Config struct {
	Enabled bool
}

type Writer struct {
	output io.Writer
	mu     sync.Mutex
}

func NewWriter(cfg Config) (*Writer, error) {
	return newWriter(cfg, os.Stdout)
}

func newWriter(cfg Config, output io.Writer) (*Writer, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if output == nil {
		return nil, fmt.Errorf("adapters:stdout:writer: output is required")
	}

	return &Writer{output: output}, nil
}

func (w *Writer) Write(ctx context.Context, logs []*domain.LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, entry := range logs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := json.Marshal(toDocument(entry))
		if err != nil {
			metrics.SendErrors.WithLabelValues(writerName).Inc()
			return fmt.Errorf("adapters:stdout:writer: marshal log entry: %w", err)
		}
		if _, err := w.output.Write(append(line, '\n')); err != nil {
			metrics.SendErrors.WithLabelValues(writerName).Inc()
			return fmt.Errorf("adapters:stdout:writer: write log entry: %w", err)
		}
		sentCounter.Inc()
	}

	return nil
}

var sentCounter = metrics.EventsSent.WithLabelValues(writerName)

func toDocument(entry *domain.LogEntry) map[string]any {
	doc := map[string]any{
		"@timestamp": entry.Timestamp().UTC().Format(time.RFC3339),
		"recordType": "kubernetes_event",
		"message":    entry.Message(),
		"level":      entry.Level(),
		"logType":    entry.LogType(),
	}

	for k, v := range entry.Fields() {
		doc[k] = v
	}

	return doc
}

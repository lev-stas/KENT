// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package victorialogs

import (
	"bytes"
	"context"
	"encoding/json"
	"event_exporter/internal/domain"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type VictoriaLogsConfig struct {
	Enabled      bool
	Endpoint     string
	ClusterID    string
	BatchSize    int
	FlushTime    time.Duration
	ExtraFields  map[string]string
	Timeout      time.Duration
	AccountID    string
	ProjectID    string
	StreamFields []string
}

type Writer struct {
	client       *http.Client
	logger       Logger
	endpoint     string
	clusterID    string
	batchSize    int
	flushTime    time.Duration
	extra        map[string]string
	input        chan *domain.LogEntry
	cancelFunc   context.CancelFunc
	accountID    string
	projectID    string
	streamFields []string
}

func NewWriter(cfg VictoriaLogsConfig, logger Logger) (*Writer, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("adapters:victorialogs:writer: endpoint is required")
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 300
	}
	if cfg.FlushTime <= 0 {
		cfg.FlushTime = 30 * time.Second
	}
	if cfg.ExtraFields == nil {
		cfg.ExtraFields = make(map[string]string)
	}

	if cfg.AccountID == "" {
		cfg.AccountID = "0"
	}
	if cfg.ProjectID == "" {
		cfg.ProjectID = "0"
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	w := &Writer{
		client:       &http.Client{Timeout: cfg.Timeout},
		logger:       logger,
		endpoint:     cfg.Endpoint,
		clusterID:    cfg.ClusterID,
		batchSize:    cfg.BatchSize,
		flushTime:    cfg.FlushTime,
		extra:        cfg.ExtraFields,
		input:        make(chan *domain.LogEntry, 5000),
		accountID:    cfg.AccountID,
		projectID:    cfg.ProjectID,
		streamFields: cfg.StreamFields,
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.cancelFunc = cancel
	go w.run(ctx)

	logger.Info(
		context.Background(),
		"adapters:victorialogs:writer: writer started",
		"endpoint", cfg.Endpoint,
		"batch_size", cfg.BatchSize,
		"flush_time", cfg.FlushTime.String(),
	)
	return w, nil
}

func (w *Writer) Write(ctx context.Context, logs []*domain.LogEntry) error {
	for _, l := range logs {
		select {
		case w.input <- l:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (w *Writer) run(ctx context.Context) {
	ticker := time.NewTicker(w.flushTime)
	defer ticker.Stop()

	var buffer []*domain.LogEntry

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		if err := w.sendBatch(ctx, buffer); err != nil {
			w.logger.Error(ctx, "adapters:victorialogs:writer: failed to send batch", "error", err)
		}
		buffer = nil
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return

		case logEntry := <-w.input:
			buffer = append(buffer, logEntry)
			if len(buffer) >= w.batchSize {
				ticker.Reset(w.flushTime)
				flush()
			}
		case <-ticker.C:
			ticker.Reset(w.flushTime)
			flush()
		}
	}
}

func (w *Writer) sendBatch(ctx context.Context, batch []*domain.LogEntry) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	streamFields := append([]string{"clusterID"}, w.streamFields...)

	for _, entry := range batch {
		doc := map[string]any{
			"@timestamp": entry.Timestamp().UTC().Format(time.RFC3339),
			"message":    entry.Message(),
			"level":      entry.Level(),
			"logType":    entry.LogType(),
			"clusterID":  w.clusterID,
		}

		for k, v := range entry.Fields() {
			doc[k] = v
		}
		for k, v := range w.extra {
			doc[k] = v
		}

		if err := enc.Encode(doc); err != nil {
			return fmt.Errorf("failed to encode log entry: %w", err)
		}
	}

	url := fmt.Sprintf("%s/insert/jsonline?_msg_field=message&_time_field=@timestamp&_stream_fields=%s", w.endpoint, strings.Join(streamFields, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/stream+json")
	req.Header.Set("AccountID", w.accountID)
	req.Header.Set("ProjectID", w.projectID)

	w.logger.Debug(ctx,
		"adapters:victorialogs: sending request",
		"url", url,
		"headers", req.Header,
		"payload_preview", buf.String(),
	)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	w.logger.Debug(ctx,
		"adapters:victorialogs: received response",
		"status", resp.Status,
		"body", string(body),
	)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("victorialogs returned non-2xx status: %s, body: %s", resp.Status, string(body))
	}

	w.logger.Info(ctx, "adapters:victorialogs: batch sent", "count", len(batch))
	return nil
}

func (w *Writer) Stop() {
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
}

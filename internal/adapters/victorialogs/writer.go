// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package victorialogs

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/metrics"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const writerName = "victorialogs"

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type AuthConfig struct {
	BasicUsername string
	BasicPassword string
	BearerToken   string
}

type TLSConfig struct {
	InsecureSkipVerify bool
	CAFile             string
	CertFile           string
	KeyFile            string
	ServerName         string
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
	QueueSize    int
	Headers      map[string]string
	Auth         AuthConfig
	TLS          TLSConfig
}

type Writer struct {
	client         *http.Client
	logger         Logger
	insertURL      string
	clusterID      string
	batchSize      int
	flushTime      time.Duration
	extra          map[string]string
	headers        map[string]string
	timeout        time.Duration
	input          chan *domain.LogEntry
	cancelFunc     context.CancelFunc
	done           chan struct{}
	accountID      string
	projectID      string
	initialBackoff time.Duration
	maxBackoff     time.Duration

	sentCounter     prometheus.Counter
	errorCounter    prometheus.Counter
	queueGauge      prometheus.Gauge
	droppedRejected prometheus.Counter
	droppedShutdown prometheus.Counter
}

// httpStatusError marks a non-2xx response so retry logic can distinguish
// transient failures (429, 5xx) from permanent rejections (other 4xx).
type httpStatusError struct {
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("victorialogs returned status %d: %s", e.code, e.body)
}

// permanentError marks a deterministic local failure (request construction,
// payload encoding) that retrying can never fix.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func NewWriter(cfg VictoriaLogsConfig, logger Logger) (*Writer, error) {
	w, err := newWriter(cfg, logger)
	if err != nil || w == nil {
		return nil, err
	}

	w.start()

	logger.Info(
		context.Background(),
		"adapters:victorialogs:writer: writer started",
		"endpoint", cfg.Endpoint,
		"batch_size", w.batchSize,
		"flush_time", w.flushTime.String(),
		"queue_size", cap(w.input),
	)
	return w, nil
}

// newWriter builds a fully configured but not yet running writer; start()
// launches the delivery loop. Split so tests can tune backoff first.
func newWriter(cfg VictoriaLogsConfig, logger Logger) (*Writer, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("adapters:victorialogs:writer: endpoint is required")
	}
	if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("adapters:victorialogs:writer: invalid endpoint %q: %w", cfg.Endpoint, err)
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
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 5000
	}

	headers, err := buildHeaders(cfg.Headers, cfg.Auth)
	if err != nil {
		return nil, err
	}

	transport, err := buildTransport(cfg.TLS)
	if err != nil {
		return nil, err
	}

	streamFields := append([]string{"clusterID"}, cfg.StreamFields...)
	insertURL := fmt.Sprintf("%s/insert/jsonline?_msg_field=message&_time_field=@timestamp&_stream_fields=%s",
		cfg.Endpoint, strings.Join(streamFields, ","))

	return &Writer{
		client:         &http.Client{Timeout: cfg.Timeout, Transport: transport},
		logger:         logger,
		insertURL:      insertURL,
		clusterID:      cfg.ClusterID,
		batchSize:      cfg.BatchSize,
		flushTime:      cfg.FlushTime,
		extra:          cfg.ExtraFields,
		headers:        headers,
		timeout:        cfg.Timeout,
		input:          make(chan *domain.LogEntry, cfg.QueueSize),
		done:           make(chan struct{}),
		accountID:      cfg.AccountID,
		projectID:      cfg.ProjectID,
		initialBackoff: time.Second,
		maxBackoff:     30 * time.Second,

		sentCounter:     metrics.EventsSent.WithLabelValues(writerName),
		errorCounter:    metrics.SendErrors.WithLabelValues(writerName),
		queueGauge:      metrics.QueueLength.WithLabelValues(writerName),
		droppedRejected: metrics.EventsDropped.WithLabelValues(writerName, "rejected"),
		droppedShutdown: metrics.EventsDropped.WithLabelValues(writerName, "shutdown"),
	}, nil
}

func (w *Writer) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancelFunc = cancel
	go w.run(ctx)
}

func buildHeaders(custom map[string]string, auth AuthConfig) (map[string]string, error) {
	if auth.BearerToken != "" && (auth.BasicUsername != "" || auth.BasicPassword != "") {
		return nil, fmt.Errorf("adapters:victorialogs:writer: configure either basic auth or bearer token, not both")
	}
	if auth.BasicPassword != "" && auth.BasicUsername == "" {
		return nil, fmt.Errorf("adapters:victorialogs:writer: basic auth password is set but username is empty")
	}

	headers := make(map[string]string, len(custom)+1)
	for k, v := range custom {
		headers[k] = v
	}

	switch {
	case auth.BearerToken != "":
		headers["Authorization"] = "Bearer " + auth.BearerToken
	case auth.BasicUsername != "":
		creds := auth.BasicUsername + ":" + auth.BasicPassword
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	}

	return headers, nil
}

// buildTransport clones http.DefaultTransport so proxy settings, dial
// timeouts and keep-alives survive enabling TLS options.
func buildTransport(cfg TLSConfig) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if !cfg.InsecureSkipVerify && cfg.CAFile == "" && cfg.CertFile == "" && cfg.KeyFile == "" && cfg.ServerName == "" {
		return transport, nil
	}

	tlsCfg := transport.TLSClientConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	}
	tlsCfg.InsecureSkipVerify = cfg.InsecureSkipVerify
	tlsCfg.ServerName = cfg.ServerName

	if cfg.CAFile != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("adapters:victorialogs:writer: read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("adapters:victorialogs:writer: no certificates parsed from CA file %s", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	if (cfg.CertFile == "") != (cfg.KeyFile == "") {
		return nil, fmt.Errorf("adapters:victorialogs:writer: tls cert_file and key_file must be set together")
	}
	if cfg.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("adapters:victorialogs:writer: load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	transport.TLSClientConfig = tlsCfg
	return transport, nil
}

// Write enqueues entries for asynchronous delivery. It blocks when the queue
// is full (backpressure) until space frees up or ctx is cancelled.
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
	defer close(w.done)

	ticker := time.NewTicker(w.flushTime)
	defer ticker.Stop()

	var buffer []*domain.LogEntry

	for {
		w.queueGauge.Set(float64(len(w.input)))

		select {
		case <-ctx.Done():
			buffer = append(buffer, w.drain()...)
			w.finalFlush(buffer)
			return

		case logEntry := <-w.input:
			buffer = append(buffer, logEntry)
			if len(buffer) >= w.batchSize {
				buffer = w.flushWithRetry(ctx, buffer)
				ticker.Reset(w.flushTime)
			}
		case <-ticker.C:
			buffer = w.flushWithRetry(ctx, buffer)
		}
	}
}

// flushWithRetry keeps the batch until it is delivered or permanently
// rejected. On success or rejection it returns the emptied batch slice for
// reuse; when ctx is cancelled mid-retry it returns the undelivered batch so
// the shutdown path can attempt a final delivery.
func (w *Writer) flushWithRetry(ctx context.Context, batch []*domain.LogEntry) []*domain.LogEntry {
	if len(batch) == 0 {
		return batch
	}

	body, err := w.encodeBatch(batch)
	if err != nil {
		w.droppedRejected.Add(float64(len(batch)))
		w.logger.Error(ctx, "adapters:victorialogs: failed to encode batch, dropping",
			"error", err, "count", len(batch))
		return batch[:0]
	}

	backoff := w.initialBackoff

	for {
		err := w.sendBody(ctx, body)
		if err == nil {
			w.sentCounter.Add(float64(len(batch)))
			metrics.BatchSize.Observe(float64(len(batch)))
			w.logger.Info(ctx, "adapters:victorialogs: batch sent", "count", len(batch))
			return batch[:0]
		}

		w.errorCounter.Inc()

		if !isRetriable(err) {
			w.droppedRejected.Add(float64(len(batch)))
			w.logger.Error(ctx, "adapters:victorialogs: batch rejected, dropping",
				"error", err, "count", len(batch))
			return batch[:0]
		}

		// Keep the queue gauge honest while this goroutine is stuck retrying.
		w.queueGauge.Set(float64(len(w.input)))

		w.logger.Warn(ctx, "adapters:victorialogs: send failed, will retry",
			"error", err, "count", len(batch), "backoff", backoff.String())

		select {
		case <-ctx.Done():
			return batch
		case <-time.After(backoff):
		}

		backoff = min(backoff*2, w.maxBackoff)
	}
}

func isRetriable(err error) bool {
	var permErr *permanentError
	if errors.As(err, &permErr) {
		return false
	}
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.code == http.StatusTooManyRequests || statusErr.code >= 500
	}
	// Network-level failures (connection refused, timeout, DNS) are transient.
	return true
}

// drain empties the input queue without blocking; used on shutdown.
func (w *Writer) drain() []*domain.LogEntry {
	var out []*domain.LogEntry
	for {
		select {
		case logEntry := <-w.input:
			out = append(out, logEntry)
		default:
			return out
		}
	}
}

// finalFlush delivers what is left in the buffer during shutdown. Each chunk
// gets a fresh bounded context because the run context is already cancelled.
func (w *Writer) finalFlush(buffer []*domain.LogEntry) {
	if len(buffer) == 0 {
		return
	}

	for start := 0; start < len(buffer); start += w.batchSize {
		end := min(start+w.batchSize, len(buffer))
		chunk := buffer[start:end]

		err := w.sendChunkBounded(chunk)
		if err != nil {
			remaining := len(buffer) - start
			w.errorCounter.Inc()
			w.droppedShutdown.Add(float64(remaining))
			w.logger.Error(context.Background(), "adapters:victorialogs: final flush failed, events lost",
				"error", err, "count", remaining)
			return
		}

		w.sentCounter.Add(float64(len(chunk)))
		metrics.BatchSize.Observe(float64(len(chunk)))
	}

	w.logger.Info(context.Background(), "adapters:victorialogs: final flush complete", "count", len(buffer))
}

func (w *Writer) sendChunkBounded(chunk []*domain.LogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	body, err := w.encodeBatch(chunk)
	if err != nil {
		return err
	}
	return w.sendBody(ctx, body)
}

func (w *Writer) encodeBatch(batch []*domain.LogEntry) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

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
			return nil, &permanentError{err: fmt.Errorf("failed to encode log entry: %w", err)}
		}
	}

	return buf.Bytes(), nil
}

func (w *Writer) sendBody(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.insertURL, bytes.NewReader(body))
	if err != nil {
		return &permanentError{err: fmt.Errorf("failed to create request: %w", err)}
	}

	req.Header.Set("Content-Type", "application/stream+json")
	req.Header.Set("AccountID", w.accountID)
	req.Header.Set("ProjectID", w.projectID)
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	w.logger.Debug(ctx,
		"adapters:victorialogs: sending request",
		"url", w.insertURL,
		"bytes", len(body),
	)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpStatusError{code: resp.StatusCode, body: string(respBody)}
	}

	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	return nil
}

// Stop cancels the writer loop and blocks until the remaining buffer is
// flushed (each chunk bounded by the writer timeout).
func (w *Writer) Stop() {
	if w.cancelFunc == nil {
		return
	}
	w.cancelFunc()
	<-w.done
}

package stdout

import (
	"bytes"
	"context"
	"encoding/json"
	"event_exporter/internal/domain"
	"testing"
	"time"
)

func TestNewWriterDisabled(t *testing.T) {
	t.Parallel()

	writer, err := newWriter(Config{Enabled: false}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("newWriter returned error: %v", err)
	}
	if writer != nil {
		t.Fatal("expected nil writer when stdout export is disabled")
	}
}

func TestWriteOutputsJSONLine(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	writer, err := newWriter(Config{Enabled: true}, output)
	if err != nil {
		t.Fatalf("newWriter returned error: %v", err)
	}

	entry, err := domain.NewLogEntry(
		time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC),
		"warning",
		"event",
		"pod restarted",
		map[string]string{
			"k8s.namespace": "monitoring",
			"event.reason":  "BackOff",
		},
	)
	if err != nil {
		t.Fatalf("NewLogEntry returned error: %v", err)
	}

	if err := writer.Write(context.Background(), []*domain.LogEntry{entry}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout output is not valid JSON: %v", err)
	}

	if got["@timestamp"] != "2026-03-29T12:00:00Z" {
		t.Fatalf("unexpected timestamp: %v", got["@timestamp"])
	}
	if got["recordType"] != "kubernetes_event" {
		t.Fatalf("unexpected recordType: %v", got["recordType"])
	}
	if got["message"] != "pod restarted" {
		t.Fatalf("unexpected message: %v", got["message"])
	}
	if got["level"] != "warning" {
		t.Fatalf("unexpected level: %v", got["level"])
	}
	if got["logType"] != "event" {
		t.Fatalf("unexpected log type: %v", got["logType"])
	}
	if got["k8s.namespace"] != "monitoring" {
		t.Fatalf("unexpected namespace: %v", got["k8s.namespace"])
	}
	if got["event.reason"] != "BackOff" {
		t.Fatalf("unexpected reason: %v", got["event.reason"])
	}
}

func TestWriteRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	writer, err := newWriter(Config{Enabled: true}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("newWriter returned error: %v", err)
	}

	entry, err := domain.NewLogEntry(
		time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC),
		"info",
		"event",
		"node recovered",
		nil,
	)
	if err != nil {
		t.Fatalf("NewLogEntry returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := writer.Write(ctx, []*domain.LogEntry{entry}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

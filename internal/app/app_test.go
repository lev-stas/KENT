package app

import (
	"event_exporter/internal/config"
	"event_exporter/internal/pkg/logger"
	"testing"
)

func TestBuildWritersWithStdoutOnly(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.Stdout.Enabled = true

	writers, stop, err := buildWriters(cfg, logger.New("error"))
	if err != nil {
		t.Fatalf("buildWriters returned error: %v", err)
	}
	defer stop()

	if len(writers) != 1 {
		t.Fatalf("expected 1 writer, got %d", len(writers))
	}
}

func TestBuildWritersWithVictoriaLogsAndStdout(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.VictoriaLogs.Enabled = true
	cfg.VictoriaLogs.Endpoint = "http://victoria-logs.example:9428"
	cfg.Stdout.Enabled = true

	writers, stop, err := buildWriters(cfg, logger.New("error"))
	if err != nil {
		t.Fatalf("buildWriters returned error: %v", err)
	}
	defer stop()

	if len(writers) != 2 {
		t.Fatalf("expected 2 writers, got %d", len(writers))
	}
}

func TestBuildWritersReturnsVictoriaLogsConfigError(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.VictoriaLogs.Enabled = true

	writers, stop, err := buildWriters(cfg, logger.New("error"))
	if stop != nil {
		defer stop()
	}

	if err == nil {
		t.Fatalf("expected error, got writers: %d", len(writers))
	}
}

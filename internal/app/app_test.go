package app

import (
	"event_exporter/internal/config"
	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/logger"
	"testing"
	"time"
)

// buildEventFilter is the only seam between the config keys and the filter
// rules, and a forgotten assignment there compiles, passes every other test,
// and silently ignores the rule at runtime. Each case below fails exactly one
// rule, so a missing line cannot hide behind the others.
func TestBuildEventFilterWiresEveryRule(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.Kubernetes.IncludeNamespaces = []string{"prod", "prod-legacy"}
	cfg.Kubernetes.ExcludeNamespaces = []string{"prod-legacy"}
	cfg.Kubernetes.IncludeEventTypes = []string{"Warning"}
	cfg.Kubernetes.IncludeKinds = []string{"Pod"}
	cfg.Kubernetes.IncludeReasons = "^(Failed|BackOff)"
	cfg.Kubernetes.ExcludeReasons = "^FailedMount$"

	filter, err := buildEventFilter(cfg)
	if err != nil {
		t.Fatalf("buildEventFilter returned error: %v", err)
	}

	tests := []struct {
		name      string
		namespace string
		eventType string
		kind      string
		reason    string
		want      bool
	}{
		{name: "passes every rule", want: true},
		{name: "include_namespaces", namespace: "default"},
		{name: "exclude_namespaces", namespace: "prod-legacy"},
		{name: "include_event_types", eventType: "Normal"},
		{name: "include_kinds", kind: "Deployment"},
		{name: "include_reasons", reason: "Started"},
		{name: "exclude_reasons", reason: "FailedMount"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			in := domain.EventInput{
				UID:       "uid-1",
				Name:      "pod-1.abc",
				Namespace: "prod",
				Reason:    "BackOff",
				Message:   "message",
				Type:      "Warning",
				Object:    domain.ObjectRef{Kind: "Pod", Name: "pod-1"},
				EventTime: time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC),
				Count:     1,
			}

			if tc.namespace != "" {
				in.Namespace = tc.namespace
			}
			if tc.eventType != "" {
				in.Type = tc.eventType
			}
			if tc.kind != "" {
				in.Object.Kind = tc.kind
			}
			if tc.reason != "" {
				in.Reason = tc.reason
			}

			ev, err := domain.NewEvent(in)
			if err != nil {
				t.Fatalf("NewEvent returned error: %v", err)
			}

			if got := filter.Allow(ev); got != tc.want {
				t.Fatalf("Allow() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildEventFilterRejectsInvalidPattern(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.Kubernetes.IncludeReasons = "("

	if _, err := buildEventFilter(cfg); err == nil {
		t.Fatal("expected an invalid pattern to fail at startup")
	}
}

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

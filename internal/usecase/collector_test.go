package usecase

import (
	"testing"
	"time"

	"event_exporter/internal/domain"
)

func TestConvertEventUsesLastTimestampForRecurringEvents(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	last := time.Date(2026, time.August, 6, 18, 30, 0, 0, time.UTC)

	ev, err := domain.NewEvent(domain.EventInput{
		UID:           "uid-1",
		Name:          "pod-1.abc",
		Namespace:     "default",
		Reason:        "BackOff",
		Message:       "Back-off restarting",
		Type:          "Warning",
		Object:        domain.ObjectRef{Kind: "Pod", Name: "pod-1", Namespace: "default"},
		Source:        "kubelet",
		EventTime:     first,
		LastTimestamp: &last,
		Count:         42,
	})
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}

	entry, err := convertEventToLogEntry(ev)
	if err != nil {
		t.Fatalf("convertEventToLogEntry returned error: %v", err)
	}
	if !entry.Timestamp().Equal(last) {
		t.Fatalf("expected last-observed timestamp %v, got %v", last, entry.Timestamp())
	}
}

// The involved object name must travel in its own field: k8s.name is the name
// of the Event object ("<object>.<hex>"), which cannot be grouped on.
func TestConvertEventExportsInvolvedObjectName(t *testing.T) {
	t.Parallel()

	entry := convertTestEvent(t, domain.EventInput{
		UID:       "uid-3",
		Name:      "pod-3.18c9a5d3099f6e69",
		Namespace: "default",
		Message:   "Back-off restarting",
		Object:    domain.ObjectRef{Kind: "Pod", Name: "pod-3", Namespace: "default"},
		EventTime: time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC),
		Count:     1,
	})

	if got := entry.Fields()["k8s.object.name"]; got != "pod-3" {
		t.Fatalf("expected involved object name pod-3, got %q", got)
	}
	if got := entry.Fields()["k8s.name"]; got != "pod-3.18c9a5d3099f6e69" {
		t.Fatalf("k8s.name must keep the Event object name, got %q", got)
	}
}

func TestConvertEventOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	base := domain.EventInput{
		UID:       "uid-4",
		Name:      "pod-4.abc",
		Namespace: "default",
		Message:   "Started container",
		Object:    domain.ObjectRef{Kind: "Pod", Name: "pod-4", Namespace: "default"},
		EventTime: time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC),
		Count:     1,
	}

	filled := base
	filled.Action = "Binding"
	filled.ReportingInstance = "node-3"

	tests := []struct {
		name  string
		input domain.EventInput
		want  map[string]string
	}{
		{
			name:  "events/v1 recorder fills both",
			input: filled,
			want:  map[string]string{"event.action": "Binding", "event.reporting_instance": "node-3"},
		},
		{
			name:  "legacy recorder leaves both out",
			input: base,
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fields := convertTestEvent(t, tc.input).Fields()

			for _, key := range []string{"event.action", "event.reporting_instance"} {
				got, present := fields[key]
				want, expected := tc.want[key]

				if present != expected {
					t.Fatalf("field %s: present=%v, expected=%v", key, present, expected)
				}
				if got != want {
					t.Fatalf("field %s: got %q, want %q", key, got, want)
				}
			}
		})
	}
}

func convertTestEvent(t *testing.T, in domain.EventInput) *domain.LogEntry {
	t.Helper()

	ev, err := domain.NewEvent(in)
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}

	entry, err := convertEventToLogEntry(ev)
	if err != nil {
		t.Fatalf("convertEventToLogEntry returned error: %v", err)
	}

	return entry
}

func TestConvertEventKeepsEventTimeForFirstOccurrence(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC)

	ev, err := domain.NewEvent(domain.EventInput{
		UID:           "uid-2",
		Name:          "pod-2.abc",
		Namespace:     "default",
		Reason:        "Started",
		Message:       "Started container",
		Type:          "Normal",
		Object:        domain.ObjectRef{Kind: "Pod", Name: "pod-2", Namespace: "default"},
		Source:        "kubelet",
		EventTime:     first,
		LastTimestamp: &first,
		Count:         1,
	})
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}

	entry, err := convertEventToLogEntry(ev)
	if err != nil {
		t.Fatalf("convertEventToLogEntry returned error: %v", err)
	}
	if !entry.Timestamp().Equal(first) {
		t.Fatalf("expected event time %v, got %v", first, entry.Timestamp())
	}
}

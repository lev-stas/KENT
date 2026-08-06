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

	ev, err := domain.NewEvent(
		"uid-1", "pod-1.abc", "default", "BackOff", "Back-off restarting", "Warning",
		domain.ObjectRef{Kind: "Pod", Name: "pod-1", Namespace: "default"},
		"kubelet", first, &last, 42,
	)
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

func TestConvertEventKeepsEventTimeForFirstOccurrence(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC)

	ev, err := domain.NewEvent(
		"uid-2", "pod-2.abc", "default", "Started", "Started container", "Normal",
		domain.ObjectRef{Kind: "Pod", Name: "pod-2", Namespace: "default"},
		"kubelet", first, &first, 1,
	)
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

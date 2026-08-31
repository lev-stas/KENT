package domain

import (
	"slices"
	"testing"
	"time"
)

func testEvent(t *testing.T, namespace, eventType, kind, reason string) *Event {
	t.Helper()

	ev, err := NewEvent(EventInput{
		UID:       "uid-1",
		Name:      "obj-1.abc",
		Namespace: namespace,
		Reason:    reason,
		Message:   "message",
		Type:      eventType,
		Object:    ObjectRef{Kind: kind, Name: "obj-1", Namespace: namespace},
		EventTime: time.Date(2026, time.August, 6, 18, 0, 0, 0, time.UTC),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}

	return ev
}

func TestEventFilterAllow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spec      FilterSpec
		namespace string
		eventType string
		kind      string
		reason    string
		want      bool
	}{
		{
			name: "empty spec exports everything",
			want: true,
		},
		{
			name:      "namespace not in include list is dropped",
			spec:      FilterSpec{IncludeNamespaces: []string{"prod"}},
			namespace: "default",
		},
		{
			name:      "namespace in include list passes",
			spec:      FilterSpec{IncludeNamespaces: []string{"prod", "default"}},
			namespace: "default",
			want:      true,
		},
		{
			name:      "excluded namespace is dropped",
			spec:      FilterSpec{ExcludeNamespaces: []string{"kube-system"}},
			namespace: "kube-system",
		},
		{
			name:      "exclude wins over include for the same namespace",
			spec:      FilterSpec{IncludeNamespaces: []string{"kube-system"}, ExcludeNamespaces: []string{"kube-system"}},
			namespace: "kube-system",
		},
		{
			name:      "only warnings exported",
			spec:      FilterSpec{IncludeEventTypes: []string{"Warning"}},
			eventType: "Normal",
		},
		{
			name:      "event type matched case-insensitively",
			spec:      FilterSpec{IncludeEventTypes: []string{"warning"}},
			eventType: "Warning",
			want:      true,
		},
		{
			name: "kind not in include list is dropped",
			spec: FilterSpec{IncludeKinds: []string{"Node"}},
			kind: "Pod",
		},
		{
			name: "kind matched case-insensitively",
			spec: FilterSpec{IncludeKinds: []string{"pod", "node"}},
			kind: "Pod",
			want: true,
		},
		{
			name:   "reason outside the include pattern is dropped",
			spec:   FilterSpec{IncludeReasons: "^(Failed|BackOff)"},
			reason: "Started",
		},
		{
			name:   "reason matching the include pattern passes",
			spec:   FilterSpec{IncludeReasons: "^(Failed|BackOff)"},
			reason: "BackOff",
			want:   true,
		},
		{
			name:   "reason matching the exclude pattern is dropped",
			spec:   FilterSpec{ExcludeReasons: "^(Pulled|Created|Started)$"},
			reason: "Started",
		},
		{
			name:   "exclude pattern wins over include pattern",
			spec:   FilterSpec{IncludeReasons: "^Started", ExcludeReasons: "^Started$"},
			reason: "Started",
		},
		{
			name:   "patterns are unanchored by default",
			spec:   FilterSpec{IncludeReasons: "Back"},
			reason: "BackOff",
			want:   true,
		},
		{
			name:      "blank values are ignored, not treated as a rule",
			spec:      FilterSpec{IncludeNamespaces: []string{""}, IncludeKinds: []string{" "}},
			namespace: "default",
			want:      true,
		},
		{
			name:      "surrounding whitespace in list values is trimmed",
			spec:      FilterSpec{IncludeEventTypes: []string{" Warning "}},
			eventType: "Warning",
			want:      true,
		},
		{
			// A YAML block scalar keeps its trailing newline, and "^Started$\n"
			// compiles but can never match.
			name:   "trailing newline in a pattern does not break matching",
			spec:   FilterSpec{IncludeReasons: "^Started$\n"},
			reason: "Started",
			want:   true,
		},
		{
			name:   "whitespace-only pattern is no rule at all",
			spec:   FilterSpec{IncludeReasons: "  \n"},
			reason: "Started",
			want:   true,
		},
		{
			name:      "all rules must pass",
			spec:      FilterSpec{IncludeNamespaces: []string{"default"}, IncludeEventTypes: []string{"Warning"}, IncludeKinds: []string{"Pod"}, ExcludeReasons: "^Unhealthy$"},
			namespace: "default",
			eventType: "Warning",
			kind:      "Pod",
			reason:    "Unhealthy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter, err := NewEventFilter(tc.spec)
			if err != nil {
				t.Fatalf("NewEventFilter returned error: %v", err)
			}

			namespace := tc.namespace
			if namespace == "" {
				namespace = "default"
			}
			eventType := tc.eventType
			if eventType == "" {
				eventType = "Normal"
			}
			kind := tc.kind
			if kind == "" {
				kind = "Pod"
			}
			reason := tc.reason
			if reason == "" {
				reason = "Started"
			}

			if got := filter.Allow(testEvent(t, namespace, eventType, kind, reason)); got != tc.want {
				t.Fatalf("Allow() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewEventFilterRejectsInvalidPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec FilterSpec
	}{
		{name: "invalid include pattern", spec: FilterSpec{IncludeReasons: "("}},
		{name: "invalid exclude pattern", spec: FilterSpec{ExcludeReasons: "[a-"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewEventFilter(tc.spec); err == nil {
				t.Fatal("expected an error for an invalid pattern")
			}
		})
	}
}

// The startup log exists so an operator can see what is enforced; a rule that
// was dropped as blank must not appear there as if it were active.
func TestEventFilterLogFieldsReportEnforcedRules(t *testing.T) {
	t.Parallel()

	filter, err := NewEventFilter(FilterSpec{
		IncludeNamespaces: []string{"prod", ""},
		IncludeKinds:      []string{" "},
		IncludeReasons:    "^Started$\n",
		ExcludeReasons:    "   ",
	})
	if err != nil {
		t.Fatalf("NewEventFilter returned error: %v", err)
	}

	fields := filter.LogFields()
	if len(fields)%2 != 0 {
		t.Fatalf("LogFields must be key-value pairs, got %d elements", len(fields))
	}

	logged := make(map[string]any, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			t.Fatalf("LogFields key at %d is not a string: %#v", i, fields[i])
		}
		logged[key] = fields[i+1]
	}

	if got := logged["include_namespaces"]; !slices.Equal(got.([]string), []string{"prod"}) {
		t.Fatalf("include_namespaces logged as %#v, want [prod]", got)
	}
	if got := logged["include_kinds"]; len(got.([]string)) != 0 {
		t.Fatalf("a blank kind must not be logged as a rule, got %#v", got)
	}
	if got := logged["include_reasons"]; got != "^Started$" {
		t.Fatalf("include_reasons logged as %#v, want the trimmed pattern", got)
	}
	if got := logged["exclude_reasons"]; got != "" {
		t.Fatalf("a whitespace-only pattern must not be logged as a rule, got %#v", got)
	}
}

func TestZeroEventFilterAllowsEverything(t *testing.T) {
	t.Parallel()

	var filter EventFilter

	if !filter.Allow(testEvent(t, "kube-system", "Normal", "Pod", "Started")) {
		t.Fatal("the zero filter must export everything")
	}
}

func TestEventFilterRejectsNilEvent(t *testing.T) {
	t.Parallel()

	var filter EventFilter

	if filter.Allow(nil) {
		t.Fatal("a nil event must not be exported")
	}
}

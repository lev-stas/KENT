// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package domain

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// FilterSpec describes, in configuration terms, which events are exported.
// Every rule is optional: an empty rule imposes no restriction, so the zero
// spec exports everything.
type FilterSpec struct {
	IncludeNamespaces []string
	ExcludeNamespaces []string
	// IncludeEventTypes limits export to these event types ("Normal",
	// "Warning"); matched case-insensitively.
	IncludeEventTypes []string
	// IncludeKinds limits export to events about these involved-object kinds
	// ("Pod", "Node"); matched case-insensitively.
	IncludeKinds []string
	// IncludeReasons and ExcludeReasons are RE2 patterns matched against the
	// event reason. Both are unanchored: use ^...$ for an exact match.
	IncludeReasons string
	ExcludeReasons string
}

// EventFilter decides whether an event is exported. Its zero value exports
// everything. It is immutable after construction and safe for concurrent use.
type EventFilter struct {
	includeNamespaces map[string]struct{}
	excludeNamespaces map[string]struct{}
	// Event types and kinds are compared case-insensitively, and a config
	// never holds more than a handful, so a linear scan beats a map.
	includeEventTypes []string
	includeKinds      []string
	includeReasons    *regexp.Regexp
	excludeReasons    *regexp.Regexp
}

// NewEventFilter compiles a spec into a filter, rejecting invalid reason
// patterns so a typo fails at startup instead of silently dropping events.
func NewEventFilter(spec FilterSpec) (EventFilter, error) {
	includeReasons, err := compilePattern(spec.IncludeReasons, "include_reasons")
	if err != nil {
		return EventFilter{}, err
	}

	excludeReasons, err := compilePattern(spec.ExcludeReasons, "exclude_reasons")
	if err != nil {
		return EventFilter{}, err
	}

	return EventFilter{
		includeNamespaces: newSet(spec.IncludeNamespaces),
		excludeNamespaces: newSet(spec.ExcludeNamespaces),
		includeEventTypes: newList(spec.IncludeEventTypes),
		includeKinds:      newList(spec.IncludeKinds),
		includeReasons:    includeReasons,
		excludeReasons:    excludeReasons,
	}, nil
}

// LogFields returns the rules the filter actually enforces, as key-value pairs
// for structured logging. It reports the compiled state rather than the raw
// configuration: blank entries are already dropped by then, so a rule that
// looked configured but is not shows up here as absent.
func (f *EventFilter) LogFields() []any {
	return []any{
		"include_namespaces", slices.Sorted(maps.Keys(f.includeNamespaces)),
		"exclude_namespaces", slices.Sorted(maps.Keys(f.excludeNamespaces)),
		"include_event_types", f.includeEventTypes,
		"include_kinds", f.includeKinds,
		"include_reasons", patternString(f.includeReasons),
		"exclude_reasons", patternString(f.excludeReasons),
	}
}

// Allow reports whether the event passes every configured rule.
func (f *EventFilter) Allow(e *Event) bool {
	if e == nil {
		return false
	}

	return f.allowNamespace(e.Namespace()) &&
		allowedByList(f.includeEventTypes, e.Type()) &&
		allowedByList(f.includeKinds, e.Object().Kind) &&
		f.allowReason(e.Reason())
}

func (f *EventFilter) allowNamespace(ns string) bool {
	if len(f.includeNamespaces) > 0 {
		if _, ok := f.includeNamespaces[ns]; !ok {
			return false
		}
	}

	_, excluded := f.excludeNamespaces[ns]

	return !excluded
}

func (f *EventFilter) allowReason(reason string) bool {
	if f.includeReasons != nil && !f.includeReasons.MatchString(reason) {
		return false
	}

	return f.excludeReasons == nil || !f.excludeReasons.MatchString(reason)
}

func allowedByList(list []string, value string) bool {
	if len(list) == 0 {
		return true
	}

	return slices.ContainsFunc(list, func(allowed string) bool {
		return strings.EqualFold(allowed, value)
	})
}

// compilePattern trims the pattern before compiling: a value written as a YAML
// block scalar keeps its trailing newline all the way into the ConfigMap, and
// "^Started$\n" compiles fine but can never match, silently dropping every
// event when used as include_reasons.
func compilePattern(pattern, key string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("domain: invalid %s pattern %q: %w", key, pattern, err)
	}

	return re, nil
}

func patternString(re *regexp.Regexp) string {
	if re == nil {
		return ""
	}

	return re.String()
}

// newSet indexes the values, skipping blanks: a stray empty entry — a
// trailing comma in an env var, an empty item in a values list — would
// otherwise become a rule that matches nothing but still restricts the filter.
func newSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))

	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = struct{}{}
		}
	}

	return set
}

func newList(values []string) []string {
	list := make([]string, 0, len(values))

	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			list = append(list, v)
		}
	}

	return list
}

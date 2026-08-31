// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidEventUID            = errors.New("domain: invalid event uid")
	ErrInvalidEventMessage        = errors.New("domain: invalid event message")
	ErrInvalidEventFirstTimestamp = errors.New("domain: invalid event time")
	ErrInvalidEventLastTimestamp  = errors.New("domain: invalid event last timestamp")
	ErrInvalidEventCount          = errors.New("domain: invalid event count")
)

type ObjectRef struct {
	Kind      string
	Name      string
	Namespace string
}

type Event struct {
	uid               string
	name              string
	namespace         string
	reason            string
	message           string
	eventType         string
	involvedObject    ObjectRef
	source            string
	action            string
	reportingInstance string
	eventTime         time.Time
	lastTimestamp     *time.Time
	count             int32
}

// EventInput carries the fields of an event to NewEvent. Named fields keep
// the many same-typed values from silently swapping places at the call site.
// Action and ReportingInstance are optional: legacy recorders leave them empty.
type EventInput struct {
	UID               string
	Name              string
	Namespace         string
	Reason            string
	Message           string
	Type              string
	Object            ObjectRef
	Source            string
	Action            string
	ReportingInstance string
	EventTime         time.Time
	LastTimestamp     *time.Time
	Count             int32
}

func NewEvent(in EventInput) (*Event, error) {
	if in.UID == "" {
		return nil, ErrInvalidEventUID
	}
	if in.Message == "" {
		return nil, ErrInvalidEventMessage
	}
	if in.EventTime.IsZero() {
		return nil, ErrInvalidEventFirstTimestamp
	}
	if in.Count < 0 {
		return nil, ErrInvalidEventCount
	}

	return &Event{
		uid:               in.UID,
		name:              in.Name,
		namespace:         in.Namespace,
		reason:            in.Reason,
		message:           in.Message,
		eventType:         in.Type,
		involvedObject:    in.Object,
		source:            in.Source,
		action:            in.Action,
		reportingInstance: in.ReportingInstance,
		eventTime:         in.EventTime,
		lastTimestamp:     in.LastTimestamp,
		count:             in.Count,
	}, nil
}

func (e *Event) UID() string               { return e.uid }
func (e *Event) Name() string              { return e.name }
func (e *Event) Namespace() string         { return e.namespace }
func (e *Event) Reason() string            { return e.reason }
func (e *Event) Message() string           { return e.message }
func (e *Event) Type() string              { return e.eventType }
func (e *Event) Object() ObjectRef         { return e.involvedObject }
func (e *Event) Source() string            { return e.source }
func (e *Event) Action() string            { return e.action }
func (e *Event) ReportingInstance() string { return e.reportingInstance }
func (e *Event) FirstTimestamp() time.Time { return e.eventTime }
func (e *Event) EventTime() time.Time      { return e.eventTime }
func (e *Event) LastTimestamp() *time.Time { return e.lastTimestamp }
func (e *Event) Count() int32              { return e.count }

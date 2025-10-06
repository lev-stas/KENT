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
	uid            string
	name           string
	namespace      string
	reason         string
	message        string
	eventType      string
	involvedObject ObjectRef
	source         string
	eventTime      time.Time
	lastTimestamp  *time.Time
	count          int32
}

func NewEvent(
	id string,
	name string,
	ns string,
	reason string,
	msg string,
	t string,
	object ObjectRef,
	source string,
	eventTime time.Time,
	last *time.Time,
	count int32,
) (*Event, error) {
	if id == "" {
		return nil, ErrInvalidEventUID
	}
	if msg == "" {
		return nil, ErrInvalidEventMessage
	}
	if eventTime.IsZero() {
		return nil, ErrInvalidEventFirstTimestamp
	}
	if count < 0 {
		return nil, ErrInvalidEventCount
	}

	return &Event{
		uid:            id,
		name:           name,
		namespace:      ns,
		reason:         reason,
		message:        msg,
		eventType:      t,
		involvedObject: object,
		source:         source,
		eventTime:      eventTime,
		lastTimestamp:  last,
		count:          count,
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
func (e *Event) FirstTimestamp() time.Time { return e.eventTime }
func (e *Event) EventTime() time.Time      { return e.eventTime }
func (e *Event) LastTimestamp() *time.Time { return e.lastTimestamp }
func (e *Event) Count() int32              { return e.count }

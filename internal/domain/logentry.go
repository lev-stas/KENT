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
	ErrInvalidLogTimestamp = errors.New("domain: invalid logentry timestamp")
	ErrInvalidLogMessage   = errors.New("domain: invalid logentry message")
)

type LogEntry struct {
	timestamp time.Time
	level     string
	logType   string
	message   string
	fields    map[string]string
}

func NewLogEntry(
	ts time.Time,
	lvl string,
	logType string,
	msg string,
	fields map[string]string,
) (*LogEntry, error) {
	if ts.IsZero() {
		return nil, ErrInvalidLogTimestamp
	}
	if lvl == "" {
		lvl = "info"
	}
	if logType == "" {
		logType = "log"
	}

	if msg == "" {
		return nil, ErrInvalidLogMessage
	}
	if fields == nil {
		fields = make(map[string]string)
	}
	return &LogEntry{
		timestamp: ts,
		level:     lvl,
		logType:   logType,
		message:   msg,
		fields:    fields,
	}, nil
}

func (l *LogEntry) Timestamp() time.Time {
	return l.timestamp
}
func (l *LogEntry) Level() string {
	return l.level
}
func (l *LogEntry) Message() string {
	return l.message
}
func (l *LogEntry) LogType() string {
	return l.logType
}
func (l *LogEntry) Fields() map[string]string {
	return l.fields
}

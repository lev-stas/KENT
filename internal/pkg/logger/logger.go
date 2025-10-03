// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type Logger interface {
	Debug(ctx context.Context, msg string, kv ...any)
	Info(ctx context.Context, msg string, kv ...any)
	Warn(ctx context.Context, msg string, kv ...any)
	Error(ctx context.Context, msg string, kv ...any)
}

type slogLogger struct {
	l *slog.Logger
}

func New(level string) Logger {
	var lvl slog.Level

	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return &slogLogger{l: slog.New(handler)}
}

func (s *slogLogger) Debug(ctx context.Context, msg string, kv ...any) {
	s.l.DebugContext(ctx, msg, kv...)
}

func (s *slogLogger) Info(ctx context.Context, msg string, kv ...any) {
	s.l.InfoContext(ctx, msg, kv...)
}

func (s *slogLogger) Warn(ctx context.Context, msg string, kv ...any) {
	s.l.WarnContext(ctx, msg, kv...)
}

func (s *slogLogger) Error(ctx context.Context, msg string, kv ...any) {
	s.l.ErrorContext(ctx, msg, kv...)
}

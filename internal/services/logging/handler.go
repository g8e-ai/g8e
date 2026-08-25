// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package logging centralizes platform log file lifecycle and slog handler
// configuration. The slog handler, level parsing, and logger constructors
// live here so that LogService is the single owner of all logging concerns.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// parseLogLevel validates and converts CLI input into slog levels.
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info":
		return slog.LevelInfo, nil
	case "error":
		return slog.LevelError, nil
	case "debug":
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, fmt.Errorf("%w: supported values are: info, error, debug", constants.ErrInvalidLogLevel)
	}
}

// logHandler is a custom slog.Handler for platform log formatting. It is not
// operator-specific — it is the single slog handler used by both daemon-mode
// logging (LogService) and CLI client logging (NewStdoutLogger).
type logHandler struct {
	level  slog.Level
	output io.Writer
	attrs  []slog.Attr
	groups []string
}

func newLogHandler(output io.Writer, level slog.Level) *logHandler {
	return &logHandler{
		level:  level,
		output: output,
	}
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := r.Time.In(time.Local).Format(time.RFC3339)
	levelStr := strings.ToUpper(r.Level.String())

	msg := fmt.Sprintf("%s %s: %s", timestamp, levelStr, r.Message)

	attrs := make([]slog.Attr, 0, r.NumAttrs()+len(h.attrs))
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	if len(attrs) > 0 {
		groupPrefix := ""
		if len(h.groups) > 0 {
			groupPrefix = strings.Join(h.groups, ".") + "."
		}
		for _, attr := range attrs {
			msg += fmt.Sprintf("\n  - %s%s: %v", groupPrefix, attr.Key, attr.Value.Any())
		}
	}

	msg += "\n"
	_, err := h.output.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("logging: write: %w", err)
	}
	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &logHandler{
		level:  h.level,
		output: h.output,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &logHandler{
		level:  h.level,
		output: h.output,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

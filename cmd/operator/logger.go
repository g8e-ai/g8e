// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// configureLogger returns a slog logger configured with operator-friendly formatting
func configureLogger(level string) (*slog.Logger, error) {
	return configureLoggerWithOutput(level, os.Stdout)
}

// configureLoggerWithOutput returns a slog logger configured with operator-friendly formatting
// that writes to the specified output writer
func configureLoggerWithOutput(level string, output io.Writer) (*slog.Logger, error) {
	parsedLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}

	handler := newOperatorHandler(output, parsedLevel)
	logger := slog.New(handler)

	return logger, nil
}

// parseLogLevel validates and converts CLI input into slog levels
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info":
		return slog.LevelInfo, nil
	case string(constants.ConnectionStateError):
		return slog.LevelError, nil
	case "debug":
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, fmt.Errorf("%w: supported values are: info, error, debug", constants.ErrInvalidLogLevel)
	}
}

// operatorHandler is a custom slog.Handler for operator-friendly log formatting
type operatorHandler struct {
	level  slog.Level
	output io.Writer
	attrs  []slog.Attr
	groups []string
}

func newOperatorHandler(output io.Writer, level slog.Level) *operatorHandler {
	return &operatorHandler{
		level:  level,
		output: output,
	}
}

func (h *operatorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *operatorHandler) Handle(_ context.Context, r slog.Record) error {
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
		for _, attr := range attrs {
			msg += fmt.Sprintf("\n  - %s: %v", attr.Key, attr.Value.Any())
		}
	}

	msg += "\n"
	_, err := h.output.Write([]byte(msg))
	return err
}

func (h *operatorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *operatorHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

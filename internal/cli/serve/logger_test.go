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

package serve

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"info", "info", slog.LevelInfo, false},
		{"info uppercase", "INFO", slog.LevelInfo, false},
		{"info with spaces", "  info  ", slog.LevelInfo, false},
		{"error", "error", slog.LevelError, false},
		{"debug", "debug", slog.LevelDebug, false},
		{"invalid", "invalid", slog.LevelInfo, true},
		{"empty", "", slog.LevelInfo, true},
		{"warn not supported", "warn", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := parseLogLevel(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "supported values are")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestConfigureLogger(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  bool
	}{
		{"info", "info", false},
		{"error", "error", false},
		{"debug", "debug", false},
		{"invalid", "invalid", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := ConfigureLogger(tt.logLevel)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, logger)
			} else {
				require.NoError(t, err)
				require.NotNil(t, logger)
			}
		})
	}
}

func TestOperatorHandler_Handle(t *testing.T) {
	t.Run("formats message with timestamp and level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
		err := handler.Handle(context.Background(), record)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "INFO")
		assert.Contains(t, output, "test message")
		assert.Contains(t, output, "\n")
	})

	t.Run("formats error level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)

		record := slog.NewRecord(time.Now(), slog.LevelError, "something failed", 0)
		err := handler.Handle(context.Background(), record)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "ERROR")
		assert.Contains(t, buf.String(), "something failed")
	})

	t.Run("formats debug level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelDebug)

		record := slog.NewRecord(time.Now(), slog.LevelDebug, "debug detail", 0)
		err := handler.Handle(context.Background(), record)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "DEBUG")
		assert.Contains(t, buf.String(), "debug detail")
	})

	t.Run("appends string and int attributes as indented lines", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)

		record := slog.NewRecord(time.Now(), slog.LevelError, "error occurred", 0)
		record.AddAttrs(
			slog.String("error", "something went wrong"),
			slog.Int("code", 500),
		)
		err := handler.Handle(context.Background(), record)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "error:")
		assert.Contains(t, output, "something went wrong")
		assert.Contains(t, output, "code:")
		assert.Contains(t, output, "500")
	})

	t.Run("filters records below handler level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelError)

		assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
		assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
		assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
	})

	t.Run("logger respects level filter end-to-end", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelError)
		logger := slog.New(handler)

		logger.Info("should be filtered")

		assert.Empty(t, buf.String())
	})
}

func TestOperatorHandler_WithAttrs(t *testing.T) {
	t.Run("returns a new distinct handler instance", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelDebug)

		newHandler := handler.WithAttrs([]slog.Attr{slog.String("svc", "g8eo")})
		require.NotNil(t, newHandler)
		assert.NotSame(t, handler, newHandler)
	})

	t.Run("pre-attached attrs appear in every record", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)
		withAttrs := handler.WithAttrs([]slog.Attr{slog.String("component", "g8eo")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "started", 0)
		err := withAttrs.Handle(context.Background(), record)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "component:")
		assert.Contains(t, buf.String(), "g8eo")
	})

	t.Run("original handler is not mutated", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)
		_ = handler.WithAttrs([]slog.Attr{slog.String("key", "value")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		err := handler.Handle(context.Background(), record)

		require.NoError(t, err)
		assert.NotContains(t, buf.String(), "key:")
	})

	t.Run("empty attrs slice returns new handler without panicking", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)
		newHandler := handler.WithAttrs([]slog.Attr{})
		assert.NotNil(t, newHandler)
	})
}

func TestOperatorHandler_WithGroup(t *testing.T) {
	t.Run("returns a new distinct handler instance", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)

		grouped := handler.WithGroup("requests")
		require.NotNil(t, grouped)
		assert.NotSame(t, handler, grouped)
	})

	t.Run("original handler is not mutated", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)
		_ = handler.WithGroup("requests")

		assert.Empty(t, handler.groups)
	})

	t.Run("group name is stored on returned handler", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)

		grouped := handler.WithGroup("audit")
		oh, ok := grouped.(*operatorHandler)
		require.True(t, ok)
		assert.Equal(t, []string{"audit"}, oh.groups)
	})
}

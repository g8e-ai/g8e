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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
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

func TestConfigureLoggerWithOutput(t *testing.T) {
	t.Run("writes to provided output", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := ConfigureLoggerWithOutput("info", &buf)
		require.NoError(t, err)
		require.NotNil(t, logger)

		logger.Info("hello world")

		output := buf.String()
		assert.Contains(t, output, "hello world")
		assert.Contains(t, output, "INFO")
	})

	t.Run("invalid level returns error and nil logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := ConfigureLoggerWithOutput("trace", &buf)

		require.Error(t, err)
		assert.Nil(t, logger)
		assert.Contains(t, err.Error(), "logger: configure:")
	})

	t.Run("debug level allows debug messages", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := ConfigureLoggerWithOutput("debug", &buf)
		require.NoError(t, err)

		logger.Debug("debug msg")
		assert.Contains(t, buf.String(), "debug msg")
		assert.Contains(t, buf.String(), "DEBUG")
	})

	t.Run("error level filters info and debug", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := ConfigureLoggerWithOutput("error", &buf)
		require.NoError(t, err)

		logger.Info("info msg")
		logger.Debug("debug msg")
		assert.Empty(t, buf.String())

		logger.Error("error msg")
		assert.Contains(t, buf.String(), "error msg")
	})
}

func TestParseLogLevel_ErrorsIs(t *testing.T) {
	_, err := parseLogLevel("bogus")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidLogLevel), "error should wrap constants.ErrInvalidLogLevel")
}

func TestOperatorHandler_Handle_NoAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "plain message", 0)
	err := handler.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "plain message")
	assert.NotContains(t, output, "  - ")
}

func TestOperatorHandler_Handle_WriteError(t *testing.T) {
	handler := newOperatorHandler(&failingWriter{}, slog.LevelInfo)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := handler.Handle(context.Background(), record)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger: write:")
}

func TestOperatorHandler_Handle_VariousAttrTypes(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "multi attrs", 0)
	record.AddAttrs(
		slog.Bool("enabled", true),
		slog.Float64("ratio", 0.95),
		slog.Duration("elapsed", 150*time.Millisecond),
		slog.Any("custom", fmt.Sprintf("val-%d", 42)),
	)
	err := handler.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "enabled:")
	assert.Contains(t, output, "true")
	assert.Contains(t, output, "ratio:")
	assert.Contains(t, output, "0.95")
	assert.Contains(t, output, "elapsed:")
	assert.Contains(t, output, "custom:")
	assert.Contains(t, output, "val-42")
}

func TestOperatorHandler_Handle_PreAttachedAndRecordAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)
	withPre := handler.WithAttrs([]slog.Attr{slog.String("svc", "g8eo")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "combined", 0)
	record.AddAttrs(slog.Int("code", 200))
	err := withPre.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "svc:")
	assert.Contains(t, output, "g8eo")
	assert.Contains(t, output, "code:")
	assert.Contains(t, output, "200")
}

func TestOperatorHandler_Handle_TimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)

	ts := time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)
	record := slog.NewRecord(ts, slog.LevelInfo, "ts test", 0)
	err := handler.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	_, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(strings.Split(output, " ")[0]))
	require.NoError(t, parseErr, "timestamp should be RFC3339-parseable")
}

func TestOperatorHandler_Enabled_Boundary(t *testing.T) {
	handler := newOperatorHandler(&bytes.Buffer{}, slog.LevelInfo)

	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
}

func TestOperatorHandler_WithAttrs_Chaining(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelDebug)

	h1 := handler.WithAttrs([]slog.Attr{slog.String("a", "1")})
	h2 := h1.WithAttrs([]slog.Attr{slog.String("b", "2")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "chained", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "a:")
	assert.Contains(t, output, "1")
	assert.Contains(t, output, "b:")
	assert.Contains(t, output, "2")
}

func TestOperatorHandler_WithAttrs_PreservesLevelAndOutput(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelError)
	derived := handler.WithAttrs([]slog.Attr{slog.String("k", "v")})

	oh, ok := derived.(*operatorHandler)
	require.True(t, ok)
	assert.Equal(t, slog.LevelError, oh.level)
	assert.Same(t, &buf, oh.output)
}

func TestOperatorHandler_WithGroup_Chaining(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)

	g1 := handler.WithGroup("outer")
	g2 := g1.WithGroup("inner")

	oh, ok := g2.(*operatorHandler)
	require.True(t, ok)
	assert.Equal(t, []string{"outer", "inner"}, oh.groups)
}

func TestOperatorHandler_WithGroup_PreservesLevelOutputAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelDebug)
	withAttrs := handler.WithAttrs([]slog.Attr{slog.String("x", "y")})
	grouped := withAttrs.WithGroup("grp")

	oh, ok := grouped.(*operatorHandler)
	require.True(t, ok)
	assert.Equal(t, slog.LevelDebug, oh.level)
	assert.Same(t, &buf, oh.output)
	assert.Len(t, oh.attrs, 1)
	assert.Equal(t, "x", oh.attrs[0].Key)
}

func TestOperatorHandler_Handle_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)
	grouped := handler.WithGroup("requests")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "incoming", 0)
	record.AddAttrs(slog.String("method", "GET"))
	err := grouped.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "requests.method:")
	assert.Contains(t, output, "GET")
}

func TestOperatorHandler_Handle_WithGroupChained(t *testing.T) {
	var buf bytes.Buffer
	handler := newOperatorHandler(&buf, slog.LevelInfo)
	grouped := handler.WithGroup("svc").WithGroup("http")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "req", 0)
	record.AddAttrs(slog.Int("status", 200))
	err := grouped.Handle(context.Background(), record)

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "svc.http.status:")
	assert.Contains(t, output, "200")
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("disk full")
}

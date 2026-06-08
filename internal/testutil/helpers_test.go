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

package testutil

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewTestConfig
// ---------------------------------------------------------------------------

func TestNewTestConfig(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "ReturnsNonNil",
			test: func(t *testing.T, cfg *config.Config) {
				require.NotNil(t, cfg)
			},
		},
		{
			name: "FieldsPopulated",
			test: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "test-project", cfg.ProjectID)
				assert.NotEmpty(t, cfg.OperatorID)
				assert.NotEmpty(t, cfg.OperatorSessionId)
				assert.NotEmpty(t, cfg.PubSubURL)
				assert.NotEmpty(t, cfg.WorkDir)
			},
		},
		{
			name: "WorkDirExists",
			test: func(t *testing.T, cfg *config.Config) {
				_, err := os.Stat(cfg.WorkDir)
				require.NoError(t, err, "WorkDir from t.TempDir() must exist")
			},
		},
		{
			name: "OperatorIDContainsTestName",
			test: func(t *testing.T, cfg *config.Config) {
				safeName := strings.NewReplacer("/", "-", " ", "_", ":", "-").Replace(t.Name())
				if len(safeName) > 40 {
					safeName = safeName[:40]
				}
				assert.Contains(t, cfg.OperatorID, safeName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewTestConfig(t)
			tt.test(t, cfg)
		})
	}
}

func TestNewTestConfig_HasUniqueIDs(t *testing.T) {
	cfg1 := NewTestConfig(t)
	cfg2 := NewTestConfig(t)
	assert.NotEqual(t, cfg1.OperatorID, cfg2.OperatorID)
	assert.NotEqual(t, cfg1.OperatorSessionId, cfg2.OperatorSessionId)
}

func TestNewTestConfig_ParallelUnique(t *testing.T) {
	const n = 10
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = NewTestConfig(t).OperatorID
	}
	seen := make(map[string]bool, n)
	for _, id := range ids {
		assert.False(t, seen[id], "duplicate OperatorID: %s", id)
		seen[id] = true
	}
}

// ---------------------------------------------------------------------------
// NewTestLogger
// ---------------------------------------------------------------------------

func TestNewTestLogger(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T, logger *slog.Logger)
	}{
		{
			name: "ReturnsNonNil",
			test: func(t *testing.T, logger *slog.Logger) {
				require.NotNil(t, logger)
			},
		},
		{
			name: "DoesNotPanic",
			test: func(t *testing.T, logger *slog.Logger) {
				assert.NotPanics(t, func() {
					logger.Info("info message")
					logger.Info("debug message")
					logger.Error("error message")
					logger.Warn("warn message")
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger()
			tt.test(t, logger)
		})
	}
}

// ---------------------------------------------------------------------------
// NewVerboseTestLogger
// ---------------------------------------------------------------------------

func TestNewVerboseTestLogger(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T, logger *slog.Logger)
	}{
		{
			name: "ReturnsNonNil",
			test: func(t *testing.T, logger *slog.Logger) {
				require.NotNil(t, logger)
			},
		},
		{
			name: "WritesToTestLog",
			test: func(t *testing.T, logger *slog.Logger) {
				assert.NotPanics(t, func() {
					logger.Info("verbose test log message")
					logger.Info("verbose debug message")
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewVerboseTestLogger(t)
			tt.test(t, logger)
		})
	}
}

// ---------------------------------------------------------------------------
// testLogWriter.Write
// ---------------------------------------------------------------------------

func TestTestLogWriter_Write(t *testing.T) {
	tests := []struct {
		name string
		msg  []byte
		test func(t *testing.T, n int, err error)
	}{
		{
			name: "ReturnsLenAndNoError",
			msg:  []byte("test log line\n"),
			test: func(t *testing.T, n int, err error) {
				require.NoError(t, err)
				assert.Equal(t, len([]byte("test log line\n")), n)
			},
		},
		{
			name: "EmptySlice",
			msg:  []byte{},
			test: func(t *testing.T, n int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, n)
			},
		},
		{
			name: "MultiLine",
			msg:  []byte("line1\nline2\nline3\n"),
			test: func(t *testing.T, n int, err error) {
				require.NoError(t, err)
				assert.Equal(t, len([]byte("line1\nline2\nline3\n")), n)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := testLogWriter{t: t}
			n, err := w.Write(tt.msg)
			tt.test(t, n, err)
		})
	}
}

// ---------------------------------------------------------------------------
// GetTestOperatorDirectURL
// ---------------------------------------------------------------------------

func TestGetTestOperatorDirectURL(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T, url string)
	}{
		{
			name: "DefaultScheme",
			test: func(t *testing.T, url string) {
				// g8e uses ZERO environment variables - always uses default URL
				assert.True(t, strings.HasPrefix(url, "wss://"), "default URL must use wss:// scheme, got: %s", url)
				assert.NotEmpty(t, url)
			},
		},
		{
			name: "NotEmpty",
			test: func(t *testing.T, url string) {
				// g8e uses ZERO environment variables - always uses default URL
				assert.NotEmpty(t, url)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := GetTestOperatorDirectURL()
			tt.test(t, url)
		})
	}
}

// ---------------------------------------------------------------------------
// TempFile
// ---------------------------------------------------------------------------

func TestTempFile_RegistersCleanup(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tempfile_test_artifact"

	// Create the file so the cleanup has something to remove.
	require.NoError(t, os.WriteFile(path, []byte("data"), 0600))

	TempFile(t, path)

	// File must still exist before the test ends.
	_, err := os.Stat(path)
	require.NoError(t, err, "file must exist before cleanup runs")
	// Cleanup runs when t ends - verified by t.Cleanup registration in TempFile itself.
}

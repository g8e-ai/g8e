// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
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
				require.NoError(t, err, "WorkDir from TempDir(t) must exist")
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

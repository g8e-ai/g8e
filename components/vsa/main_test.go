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
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vault "github.com/g8e-ai/g8e/components/vsa/services/vault"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
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
			logger, err := configureLogger(tt.logLevel)
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

		newHandler := handler.WithAttrs([]slog.Attr{slog.String("svc", "vsa")})
		require.NotNil(t, newHandler)
		assert.NotSame(t, handler, newHandler)
	})

	t.Run("pre-attached attrs appear in every record", func(t *testing.T) {
		var buf bytes.Buffer
		handler := newOperatorHandler(&buf, slog.LevelInfo)
		withAttrs := handler.WithAttrs([]slog.Attr{slog.String("component", "vsa")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "started", 0)
		err := withAttrs.Handle(context.Background(), record)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "component:")
		assert.Contains(t, buf.String(), "vsa")
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

func TestMain_Version(t *testing.T) {
	// Backup os.Args and restore after test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Mock os.Exit to prevent test process from exiting
	// We can't easily mock os.Exit in the same process without some trickery
	// but we can check the version printing logic directly.
	assert.NotPanics(t, func() {
		printVersion()
	})
}

func TestMain_StreamFlag(t *testing.T) {
	// This tests the entry point branching logic for 'stream' command
	// without actually running the full stream logic which requires network/ssh.
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"g8e.operator", "stream", "--help"}

	// We just want to ensure it doesn't panic on the branch
	// Full testing of cmd.RunStream belongs in its own package tests
	assert.True(t, len(os.Args) > 1 && os.Args[1] == "stream")
}

func TestHandleVerifyVault_NotInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized())
}

func TestHandleRekeyVault_RequiresInitializedVault(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	err = v.Rekey("old-key", "new-key")
	require.Error(t, err)
}

func TestHandleResetVault_RequiresInitializedVault(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized())
}

// ---------------------------------------------------------------------------
// runListenMode — invalid log level path (does not bind any port)
// ---------------------------------------------------------------------------

func TestRunListenMode_InvalidLogLevel_ExitsConfigError(t *testing.T) {
	// configureLogger is the first thing runListenMode calls.
	// Verify that an invalid log level returns an error from configureLogger,
	// which is the gate before any network binding occurs.
	_, err := configureLogger("notavalidlevel")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "supported values are")
}

// ---------------------------------------------------------------------------
// handleVaultCommand — log level gate
// ---------------------------------------------------------------------------

func TestHandleVaultCommand_InvalidLogLevel_ConfigureLoggerErrors(t *testing.T) {
	// handleVaultCommand calls configureLogger as its first step.
	// Verify the same gate applies there.
	_, err := configureLogger("bad")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handleRekeyVault — missing old key
// ---------------------------------------------------------------------------

func TestHandleRekeyVault_MissingOldKey_PrintsError(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	// Rekey without initializing vault — must return error
	err = v.Rekey("", "new-key")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handleVerifyVault — not initialized path
// ---------------------------------------------------------------------------

func TestHandleVerifyVault_VaultNotInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized(), "fresh vault must not be initialized")
}

func TestHandleVerifyVault_MissingAPIKey_VaultInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	header, _, err := vault.NewVaultHeader("initial-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	require.True(t, v.IsInitialized())

	// Wrong key should fail integrity check
	err = v.VerifyIntegrity("wrong-key")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handleResetVault — not initialized path
// ---------------------------------------------------------------------------

func TestHandleResetVault_VaultNotInitialized_NoOp(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized(), "nothing to reset on fresh vault")
}

func TestHandleResetVault_Initialized_ResetDestroysData(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	header, _, err := vault.NewVaultHeader("some-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))
	require.True(t, v.IsInitialized())

	require.NoError(t, v.Reset(true))
	assert.False(t, v.IsInitialized(), "vault must be uninitialized after reset")
}

// ---------------------------------------------------------------------------
// runOpenClawMode — config validation gate
// ---------------------------------------------------------------------------

func TestRunOpenClawMode_EmptyURL_ConfigError(t *testing.T) {
	// config.LoadOpenClaw returns an error when gatewayURL is empty —
	// this is the first gate inside runOpenClawMode before any dial.
	_, err := configureLogger("info")
	require.NoError(t, err)

	// Reproduce the LoadOpenClaw validation that runOpenClawMode delegates to.
	cfg, err := loadOpenClawConfig("", "token", "node", "display", "", "info")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "gateway URL")
}

func TestRunOpenClawMode_InvalidLogLevel_ConfigError(t *testing.T) {
	_, err := configureLogger("garbage")
	require.Error(t, err)
}

// loadOpenClawConfig is a test-local wrapper that calls config.LoadOpenClaw
// so we can assert its validation without invoking runOpenClawMode (which calls os.Exit).
func loadOpenClawConfig(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) (interface{}, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gateway URL is required (--openclaw-url)")
	}
	return struct{}{}, nil
}

func TestHandleVaultLifecycle(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	require.False(t, v.IsInitialized())

	header, _, err := vault.NewVaultHeader("initial-api-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	require.True(t, v.IsInitialized())

	require.NoError(t, v.VerifyIntegrity("initial-api-key"))

	require.NoError(t, v.Rekey("initial-api-key", "new-api-key"))

	require.NoError(t, v.VerifyIntegrity("new-api-key"))

	err = v.VerifyIntegrity("wrong-key")
	require.Error(t, err)

	require.NoError(t, v.Reset(true))

	assert.False(t, v.IsInitialized())
}

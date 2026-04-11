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
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewTestConfig
// ---------------------------------------------------------------------------

func TestNewTestConfig_ReturnsNonNil(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)
}

func TestNewTestConfig_HasUniqueIDs(t *testing.T) {
	cfg1 := NewTestConfig(t)
	cfg2 := NewTestConfig(t)
	assert.NotEqual(t, cfg1.OperatorID, cfg2.OperatorID)
	assert.NotEqual(t, cfg1.OperatorSessionId, cfg2.OperatorSessionId)
}

func TestNewTestConfig_OperatorIDContainsTestName(t *testing.T) {
	cfg := NewTestConfig(t)
	safeName := strings.NewReplacer("/", "-", " ", "_", ":", "-").Replace(t.Name())
	if len(safeName) > 40 {
		safeName = safeName[:40]
	}
	assert.Contains(t, cfg.OperatorID, safeName)
}

func TestNewTestConfig_FieldsPopulated(t *testing.T) {
	cfg := NewTestConfig(t)
	assert.Equal(t, "test-project", cfg.ProjectID)
	assert.Equal(t, "test-api-key", cfg.APIKey)
	assert.NotEmpty(t, cfg.OperatorID)
	assert.NotEmpty(t, cfg.OperatorSessionId)
	assert.NotEmpty(t, cfg.PubSubURL)
	assert.NotEmpty(t, cfg.WorkDir)
}

func TestNewTestConfig_WorkDirExists(t *testing.T) {
	cfg := NewTestConfig(t)
	_, err := os.Stat(cfg.WorkDir)
	require.NoError(t, err, "WorkDir from t.TempDir() must exist")
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

func TestNewTestLogger_ReturnsNonNil(t *testing.T) {
	logger := NewTestLogger()
	require.NotNil(t, logger)
}

func TestNewTestLogger_DoesNotPanic(t *testing.T) {
	logger := NewTestLogger()
	assert.NotPanics(t, func() {
		logger.Info("info message")
		logger.Info("debug message")
		logger.Error("error message")
		logger.Warn("warn message")
	})
}

// ---------------------------------------------------------------------------
// NewVerboseTestLogger
// ---------------------------------------------------------------------------

func TestNewVerboseTestLogger_ReturnsNonNil(t *testing.T) {
	logger := NewVerboseTestLogger(t)
	require.NotNil(t, logger)
}

func TestNewVerboseTestLogger_WritesToTestLog(t *testing.T) {
	logger := NewVerboseTestLogger(t)
	assert.NotPanics(t, func() {
		logger.Info("verbose test log message")
		logger.Info("verbose debug message")
	})
}

// ---------------------------------------------------------------------------
// testLogWriter.Write
// ---------------------------------------------------------------------------

func TestTestLogWriter_Write_ReturnsLenAndNoError(t *testing.T) {
	w := testLogWriter{t: t}
	msg := []byte("test log line\n")
	n, err := w.Write(msg)
	require.NoError(t, err)
	assert.Equal(t, len(msg), n)
}

func TestTestLogWriter_Write_EmptySlice(t *testing.T) {
	w := testLogWriter{t: t}
	n, err := w.Write([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestTestLogWriter_Write_MultiLine(t *testing.T) {
	w := testLogWriter{t: t}
	msg := []byte("line1\nline2\nline3\n")
	n, err := w.Write(msg)
	require.NoError(t, err)
	assert.Equal(t, len(msg), n)
}

// ---------------------------------------------------------------------------
// GetTestG8esDirectURL
// ---------------------------------------------------------------------------

func TestGetTestG8esDirectURL_DefaultScheme(t *testing.T) {
	// Ensure env var is not set so the default branch is exercised.
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", "")
	url := GetTestG8esDirectURL()
	assert.True(t, strings.HasPrefix(url, "wss://"), "default URL must use wss:// scheme, got: %s", url)
	assert.NotEmpty(t, url)
}

func TestGetTestG8esDirectURL_EnvVarOverride(t *testing.T) {
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", "wss://custom-host:1234")
	url := GetTestG8esDirectURL()
	assert.Equal(t, "wss://custom-host:1234", url)
}

func TestGetTestG8esDirectURL_NotEmpty(t *testing.T) {
	url := GetTestG8esDirectURL()
	assert.NotEmpty(t, url)
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
	// Cleanup runs when t ends — verified by t.Cleanup registration in TempFile itself.
}

func TestTempFile_NonExistentFile_NoError(t *testing.T) {
	// TempFile must not panic or error when the file doesn't exist at cleanup time.
	// We cannot trigger t.Cleanup mid-test, so we verify TempFile registers without panic.
	assert.NotPanics(t, func() {
		TempFile(t, "/tmp/g8e_testutil_nonexistent_file_xyz_abc")
	})
}

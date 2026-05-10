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

package pubsub

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	sentinel "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLocalStore creates a real enabled LocalStoreService backed by a temp DB.
func newTestLocalStore(t *testing.T) *storage.LocalStoreService {
	t.Helper()
	cfg := storage.DefaultLocalStoreConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "scrubbed.db")
	svc, err := storage.NewLocalStoreService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, svc)
	t.Cleanup(func() { svc.Close() })
	return svc
}

// newTestRawVault creates a real enabled RawVaultService backed by a temp DB.
func newTestRawVault(t *testing.T) *storage.RawVaultService {
	t.Helper()
	cfg := &storage.RawVaultConfig{
		DBPath:               filepath.Join(t.TempDir(), "raw.db"),
		MaxDBSizeMB:          100,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
	svc, err := storage.NewRawVaultService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, svc)
	t.Cleanup(func() { svc.Close() })
	return svc
}

// ---------------------------------------------------------------------------
// NewVaultWriter
// ---------------------------------------------------------------------------

func TestNewVaultWriter_NilServicesAreAllowed(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	vw := NewVaultWriter(cfg, logger, nil, nil, nil)
	require.NotNil(t, vw)
	assert.Nil(t, vw.sentinel)
	assert.Nil(t, vw.rawVault)
	assert.Nil(t, vw.localStore)
}

func TestNewVaultWriter_StoresAllDependencies(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	ls := newTestLocalStore(t)
	rv := newTestRawVault(t)

	sentinelCfg := sentinel.DefaultSentinelConfig()
	sentinelCfg.Enabled = true
	s := sentinel.NewSentinel(sentinelCfg, logger)

	vw := NewVaultWriter(cfg, logger, s, rv, ls)
	require.NotNil(t, vw)
	assert.Equal(t, s, vw.sentinel)
	assert.Equal(t, rv, vw.rawVault)
	assert.Equal(t, ls, vw.localStore)
}

// ---------------------------------------------------------------------------
// WriteExecution — nil services (no-op paths)
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteExecution_NilServices_NoOp(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, nil, nil)

	// Must not panic with no services wired.
	vw.WriteExecution(executionWriteParams{
		id:      "exec-noop",
		command: "ls",
		stdout:  "output",
		stderr:  "",
	})
}

// ---------------------------------------------------------------------------
// WriteExecution — scrubbed vault path
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteExecution_WritesToScrubbedVault(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	ls := newTestLocalStore(t)

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, nil, ls)

	exitCode := 0
	vw.WriteExecution(executionWriteParams{
		id:         "exec-scrubbed-001",
		command:    "echo hello",
		exitCode:   &exitCode,
		durationMs: 50,
		stdout:     "hello",
		stderr:     "",
		stdoutSize: 5,
		stderrSize: 0,
		caseID:     "case-1",
		vaultMode:  constants.Status.VaultMode.Raw,
	})

	record, err := ls.GetExecution("exec-scrubbed-001")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "exec-scrubbed-001", record.ID)
	assert.Equal(t, "echo hello", record.Command)
	assert.Equal(t, []byte("hello"), record.StdoutCompressed)
}

// ---------------------------------------------------------------------------
// WriteExecution — raw vault skipped in scrubbed mode
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteExecution_RawVaultSkippedInScrubbedMode(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	rv := newTestRawVault(t)

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, rv, nil)

	vw.WriteExecution(executionWriteParams{
		id:        "exec-scrub-skip",
		command:   "secret-cmd",
		stdout:    "data",
		vaultMode: constants.Status.VaultMode.Scrubbed,
	})

	record, err := rv.GetRawExecution("exec-scrub-skip")
	require.NoError(t, err)
	assert.Nil(t, record, "raw vault must not receive writes in scrubbed mode")
}

// ---------------------------------------------------------------------------
// WriteExecution — raw vault written in raw mode
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteExecution_RawVaultWrittenInRawMode(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	rv := newTestRawVault(t)

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, rv, nil)

	exitCode := 0
	vw.WriteExecution(executionWriteParams{
		id:         "exec-raw-001",
		command:    "cat /etc/secret",
		exitCode:   &exitCode,
		durationMs: 10,
		stdout:     "supersecret",
		stderr:     "",
		stdoutSize: 11,
		caseID:     "case-raw",
		vaultMode:  constants.Status.VaultMode.Raw,
	})

	record, err := rv.GetRawExecution("exec-raw-001")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "exec-raw-001", record.ID)
	assert.Equal(t, []byte("supersecret"), record.StdoutCompressed)
}

// ---------------------------------------------------------------------------
// WriteExecution — sentinel scrubs local store output
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteExecution_SentinelScrubsScrubbedVault(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	ls := newTestLocalStore(t)

	sentinelCfg := sentinel.DefaultSentinelConfig()
	sentinelCfg.Enabled = true
	s := sentinel.NewSentinel(sentinelCfg, testutil.NewTestLogger())

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), s, nil, ls)

	vw.WriteExecution(executionWriteParams{
		id:         "exec-sentinel-001",
		command:    "env",
		stdout:     "API_KEY=sk-secret-abc123",
		stdoutSize: 22,
		vaultMode:  constants.Status.VaultMode.Raw,
	})

	record, err := ls.GetExecution("exec-sentinel-001")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.NotContains(t, string(record.StdoutCompressed), "sk-secret-abc123",
		"sentinel must scrub secrets from scrubbed vault")
}

// ---------------------------------------------------------------------------
// WriteFileDiff — nil services (no-op paths)
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteFileDiff_NilServices_NoOp(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, nil, nil)

	vw.WriteFileDiff(fileDiffWriteParams{
		diffID:    "diff-noop",
		timestamp: time.Now().UTC(),
		filePath:  "/tmp/test.txt",
		operation: "write",
	})
}

// ---------------------------------------------------------------------------
// WriteFileDiff — scrubbed vault path
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteFileDiff_WritesToScrubbedVault(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	ls := newTestLocalStore(t)

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, nil, ls)

	now := time.Now().UTC()
	vw.WriteFileDiff(fileDiffWriteParams{
		diffID:            "diff-001",
		timestamp:         now,
		filePath:          "/etc/hosts",
		operation:         "write",
		ledgerHashBefore:  "abc123",
		ledgerHashAfter:   "def456",
		diffStat:          "1 file changed",
		diffContent:       "+127.0.0.1 newhost",
		caseID:            "case-diff",
		operatorSessionID: cfg.OperatorSessionId,
	})

	record, err := ls.GetFileDiff("diff-001")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "diff-001", record.ID)
	assert.Equal(t, "/etc/hosts", record.FilePath)
	assert.Equal(t, []byte("+127.0.0.1 newhost"), record.DiffCompressed)
}

// ---------------------------------------------------------------------------
// WriteFileDiff — raw vault path
// ---------------------------------------------------------------------------

func TestVaultWriter_WriteFileDiff_WritesToRawVault(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	rv := newTestRawVault(t)

	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, rv, nil)

	now := time.Now().UTC()
	vw.WriteFileDiff(fileDiffWriteParams{
		diffID:           "diff-raw-001",
		timestamp:        now,
		filePath:         "/etc/secret-conf",
		operation:        "write",
		ledgerHashBefore: "h1",
		ledgerHashAfter:  "h2",
		diffContent:      "password=hunter2",
	})

	record, err := rv.GetRawFileDiff("diff-raw-001")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "diff-raw-001", record.ID)
	assert.Equal(t, []byte("password=hunter2"), record.DiffCompressed)
}

// ---------------------------------------------------------------------------
// StoreFileDiffFromLedger — nil ledger is a no-op
// ---------------------------------------------------------------------------

func TestVaultWriter_StoreFileDiffFromLedger_NilLedger_NoOp(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	vw := NewVaultWriter(cfg, testutil.NewTestLogger(), nil, nil, nil)

	vw.StoreFileDiffFromLedger("/tmp/f.txt", "write", "evt-1", cfg.OperatorSessionId, "case-1", nil)
}

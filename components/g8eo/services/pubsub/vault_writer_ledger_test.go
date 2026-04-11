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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultWriter_StoreFileDiffFromLedger_Success(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewVerboseTestLogger(t)
	ls := newTestLocalStore(t)
	rv := newTestRawVault(t)
	vw := NewVaultWriter(cfg, logger, nil, rv, ls)

	// Setup audit vault with git
	avCfg := storage.DefaultAuditVaultConfig()
	avCfg.DataDir = t.TempDir()
	avCfg.GitPath = "/usr/bin/git" // Assume git is in common location or in path
	av, err := storage.NewAuditVaultService(avCfg, logger)
	require.NoError(t, err)
	if !av.IsGitAvailable() {
		t.Skip("git not available for ledger testing")
	}

	ledger := storage.NewLedgerService(av, av.GetEncryptionVault(), logger)

	filePath := filepath.Join(t.TempDir(), "test.txt")

	// First commit - New file creation
	res, err := ledger.LedgerFileWrite("sess-1", filePath)
	require.NoError(t, err)

	err = os.WriteFile(filePath, []byte("line 1\n"), 0644)
	require.NoError(t, err)

	err = ledger.CompleteMirrorWrite(res, "sess-1")
	require.NoError(t, err)

	// Second commit - Update existing file
	res, err = ledger.LedgerFileWrite("sess-1", filePath)
	require.NoError(t, err)

	err = os.WriteFile(filePath, []byte("line 1\nline 2\n"), 0644)
	require.NoError(t, err)

	err = ledger.CompleteMirrorWrite(res, "sess-1")
	require.NoError(t, err)

	vw.StoreFileDiffFromLedger(filePath, "WRITE", "evt-123", "sess-1", "case-1", ledger)

	// Verify it was stored in scrubbed vault
	diffs, err := ls.GetFileDiffsBySession("sess-1", 10)
	require.NoError(t, err)
	require.NotEmpty(t, diffs)

	found := false
	for _, d := range diffs {
		if d.FilePath == filePath {
			// GetFileDiffsBySession does not include the content, so we fetch the full record
			fullRecord, err := ls.GetFileDiff(d.ID)
			require.NoError(t, err)
			assert.Contains(t, string(fullRecord.DiffCompressed), "+line 2")
			found = true
			break
		}
	}
	assert.True(t, found, "diff for file should be found in local store")
}

func TestVaultWriter_StoreFileDiffFromLedger_InsufficientHistory(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	ls := newTestLocalStore(t)
	vw := NewVaultWriter(cfg, logger, nil, nil, ls)

	avCfg := storage.DefaultAuditVaultConfig()
	avCfg.DataDir = t.TempDir()
	avCfg.GitPath = "/usr/bin/git"
	av, _ := storage.NewAuditVaultService(avCfg, logger)
	ledger := storage.NewLedgerService(av, av.GetEncryptionVault(), logger)

	filePath := filepath.Join(t.TempDir(), "missing.txt")
	vw.StoreFileDiffFromLedger(filePath, "WRITE", "evt-1", "sess-2", "case-1", ledger)

	diffs, _ := ls.GetFileDiffsBySession("sess-2", 10)
	assert.Len(t, diffs, 0)
}

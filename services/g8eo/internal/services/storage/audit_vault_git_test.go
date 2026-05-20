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

package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditVaultService_GitGetCurrentHash_ReturnsHash(t *testing.T) {
	tempDir := t.TempDir()
	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   "embedded",
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	hash, err := avs.gitGetCurrentHash()
	require.NoError(t, err)
	assert.Len(t, hash, 40, "Commit hash should be 40 hex characters")
}

func TestAuditVaultService_GitGetCurrentHash_HashChangesAfterCommit(t *testing.T) {
	tempDir := t.TempDir()
	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   "embedded",
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	hash1, err := avs.gitGetCurrentHash()
	require.NoError(t, err)
	assert.Len(t, hash1, 40)

	// Make a native commit to verify hash changes
	repo, err := git.PlainOpen(avs.ledgerPath)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)

	// Create a dummy file and add it
	dummyFile := filepath.Join(avs.ledgerPath, "dummy.txt")
	err = os.WriteFile(dummyFile, []byte("change"), 0644)
	require.NoError(t, err)

	_, err = w.Add("dummy.txt")
	require.NoError(t, err)

	_, err = w.Commit("Second commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	hash2, err := avs.gitGetCurrentHash()
	require.NoError(t, err)
	assert.Len(t, hash2, 40)

	assert.NotEqual(t, hash1, hash2, "hash should change after a new commit")
}

func TestAuditVaultService_GitGetCurrentHash_ErrorWhenGitUnavailable(t *testing.T) {
	tempDir := t.TempDir()
	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   "",
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	_, err = avs.gitGetCurrentHash()
	require.Error(t, err)
}

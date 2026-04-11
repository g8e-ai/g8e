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
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// gitOutput — runs git command and returns stdout
// ---------------------------------------------------------------------------

func TestAuditVaultService_GitOutput_ReturnsOutput(t *testing.T) {
	gitPath := testGitPath(t)

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
		GitPath:                   gitPath,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	output, err := avs.gitOutput("rev-parse", "--git-dir")
	require.NoError(t, err)
	assert.Equal(t, ".git", output)
}

func TestAuditVaultService_GitOutput_ReturnsErrorOnBadCommand(t *testing.T) {
	gitPath := testGitPath(t)

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
		GitPath:                   gitPath,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	_, err = avs.gitOutput("not-a-real-git-subcommand-xyz")
	assert.Error(t, err)
}

func TestAuditVaultService_GitOutput_ErrorWhenGitPathEmpty(t *testing.T) {
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
	require.NotNil(t, avs, "enabled service with empty GitPath must not be nil")
	defer avs.Close()

	_, err = avs.gitOutput("rev-parse", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git not available")
}

// ---------------------------------------------------------------------------
// gitGetCurrentHash — wraps gitOutput("rev-parse", "HEAD")
// ---------------------------------------------------------------------------

func TestAuditVaultService_GitGetCurrentHash_ReturnsHash(t *testing.T) {
	gitPath := testGitPath(t)

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
		GitPath:                   gitPath,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Seed at least one commit so HEAD resolves
	_, err = avs.gitOutput("commit", "--allow-empty", "-m", "init")
	require.NoError(t, err)

	hash, err := avs.gitGetCurrentHash()
	require.NoError(t, err)
	assert.Len(t, hash, 40, "SHA-1 commit hash should be 40 hex characters")
}

func TestAuditVaultService_GitGetCurrentHash_HashChangesAfterCommit(t *testing.T) {
	gitPath := testGitPath(t)

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
		GitPath:                   gitPath,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	_, err = avs.gitOutput("commit", "--allow-empty", "-m", "first")
	require.NoError(t, err)

	hash1, err := avs.gitGetCurrentHash()
	require.NoError(t, err)
	assert.Len(t, hash1, 40)

	_, err = avs.gitOutput("commit", "--allow-empty", "-m", "second")
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
	require.NotNil(t, avs, "enabled service with empty GitPath must not be nil")
	defer avs.Close()

	_, err = avs.gitGetCurrentHash()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git not available")
}

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
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestLedgerForDiffStat creates a test environment for GitLedgerService
// specifically for diff stat testing, using real infrastructure (Git, encryption vault).
func setupTestLedgerForDiffStat(t *testing.T) (*GitLedgerService, string) {
	t.Helper()
	gitPath := testGitPath(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

	// Create vault but do NOT unlock it (encryption disabled)
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	ledgerConfig := &LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}
	lms, err := NewGitLedgerService(ledgerConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, lms)

	return lms, tempDir
}

// TestLedgerService_GetDiffStat_EmptyHashesReturnsEmpty verifies that GetDiffStat
// returns an empty string when both hash parameters are empty, demonstrating
// fail-closed behavior for invalid input.
func TestLedgerService_GetDiffStat_EmptyHashesReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedgerForDiffStat(t)

	result := lms.GetDiffStat("", "", "operator-session")
	assert.Empty(t, result)
}

// TestLedgerService_GetDiffStat_BetweenTwoCommits verifies that GetDiffStat
// correctly calculates the diff statistics between two valid Git commits,
// ensuring the Git diff-numstat command is executed properly.
func TestLedgerService_GetDiffStat_BetweenTwoCommits(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedgerForDiffStat(t)

	testFile := filepath.Join(tempDir, "diffstat_test.txt")
	operatorSessionID := "sess-diffstat"

	// First commit: write initial file content
	result1, err := lms.LedgerFileWrite(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result1, operatorSessionID))

	hashBefore := result1.LedgerHashAfter
	require.NotEmpty(t, hashBefore)

	// Second commit: overwrite with different content
	result2, err := lms.LedgerFileWrite(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\nline three\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result2, operatorSessionID))

	hashAfter := result2.LedgerHashAfter
	require.NotEmpty(t, hashAfter)

	stat := lms.GetDiffStat(hashBefore, hashAfter, operatorSessionID)
	assert.NotEmpty(t, stat)
}

// TestLedgerService_GetDiffStat_SameHashReturnsEmpty verifies that GetDiffStat
// returns an empty string when the same hash is provided for both parameters,
// as there is no diff to calculate.
func TestLedgerService_GetDiffStat_SameHashReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedgerForDiffStat(t)

	testFile := filepath.Join(tempDir, "same_hash.txt")
	operatorSessionID := "sess-same"

	result, err := lms.LedgerFileWrite(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("content\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result, operatorSessionID))

	hash := result.LedgerHashAfter
	require.NotEmpty(t, hash)

	stat := lms.GetDiffStat(hash, hash, operatorSessionID)
	assert.Empty(t, stat)
}

// TestLedgerService_GetDiffStat_InvalidHashesReturnsEmpty verifies fail-closed
// behavior when invalid/non-existent Git hashes are provided, ensuring the
// function returns empty instead of panicking or returning errors.
func TestLedgerService_GetDiffStat_InvalidHashesReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedgerForDiffStat(t)

	stat := lms.GetDiffStat("deadbeef", "cafebabe", "session")
	assert.Empty(t, stat)
}

// TestLedgerService_GetDiffStat_GitDisabledReturnsEmpty verifies that GetDiffStat
// returns an empty string when the Git ledger service is disabled (nil config),
// demonstrating fail-closed behavior for disabled infrastructure.
func TestLedgerService_GetDiffStat_GitDisabledReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	stat := lms.GetDiffStat("abc123", "def456", "operator-session")
	assert.Empty(t, stat)
}

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GetDiffStat — wraps calculateDiffStat
// ---------------------------------------------------------------------------

func TestLedgerService_GetDiffStat_EmptyHashesReturnsEmpty(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	result := lms.GetDiffStat("", "")
	assert.Empty(t, result)
}

func TestLedgerService_GetDiffStat_BetweenTwoCommits(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	testFile := filepath.Join(tempDir, "diffstat_test.txt")

	// First commit: write initial file content
	result1, err := lms.LedgerFileWrite("sess-diffstat", testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result1, "sess-diffstat"))

	hashBefore := result1.LedgerHashAfter
	require.NotEmpty(t, hashBefore)

	// Second commit: overwrite with different content
	result2, err := lms.LedgerFileWrite("sess-diffstat", testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\nline three\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result2, "sess-diffstat"))

	hashAfter := result2.LedgerHashAfter
	require.NotEmpty(t, hashAfter)

	stat := lms.GetDiffStat(hashBefore, hashAfter)
	assert.NotEmpty(t, stat)
}

func TestLedgerService_GetDiffStat_SameHashReturnsEmpty(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	testFile := filepath.Join(tempDir, "same_hash.txt")

	result, err := lms.LedgerFileWrite("sess-same", testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("content\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result, "sess-same"))

	hash := result.LedgerHashAfter
	require.NotEmpty(t, hash)

	stat := lms.GetDiffStat(hash, hash)
	assert.Empty(t, stat)
}

func TestLedgerService_GetDiffStat_InvalidHashesReturnsEmpty(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	stat := lms.GetDiffStat("deadbeef", "cafebabe")
	assert.Empty(t, stat)
}

func TestLedgerService_GetDiffStat_GitDisabledReturnsEmpty(t *testing.T) {
	lms := NewLedgerService(nil, nil, nil)

	stat := lms.GetDiffStat("abc123", "def456")
	assert.Empty(t, stat)
}

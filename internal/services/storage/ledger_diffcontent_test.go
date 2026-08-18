// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GetDiffContent - wraps git diff between two commits
// ---------------------------------------------------------------------------

func TestLedgerService_GetDiffContent_EmptyHashesReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedger(t)

	result := lms.GetDiffContent("", "", "operator-session")
	assert.Empty(t, result)
}

func TestLedgerService_GetDiffContent_BetweenTwoCommits(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFile := filepath.Join(tempDir, "diffcontent_test.txt")
	operatorSessionID := "sess-diffcontent"

	// First commit: write initial file content
	result1, err := lms.MirrorFileCreate(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("Line 1\n"), 0644))
	require.NoError(t, lms.CompleteMirrorCreate(result1, operatorSessionID))

	hashBefore := result1.LedgerHashAfter
	require.NotEmpty(t, hashBefore)

	// Second commit: modify file content
	result2, err := lms.LedgerFileWrite(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("Line 1\nLine 2\n"), 0644))
	require.NoError(t, lms.CompleteMirrorWrite(result2, operatorSessionID))

	hashAfter := result2.LedgerHashAfter
	require.NotEmpty(t, hashAfter)

	// Verify diff content shows the added line
	diff := lms.GetDiffContent(hashBefore, hashAfter, operatorSessionID)
	assert.NotEmpty(t, diff)
	assert.Contains(t, diff, "+Line 2")
}

func TestLedgerService_GetDiffContent_SameHashReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFile := filepath.Join(tempDir, "same_hash.txt")
	operatorSessionID := "sess-same"

	result, err := lms.MirrorFileCreate(operatorSessionID, testFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, []byte("content\n"), 0644))
	require.NoError(t, lms.CompleteMirrorCreate(result, operatorSessionID))

	hash := result.LedgerHashAfter
	require.NotEmpty(t, hash)

	diff := lms.GetDiffContent(hash, hash, operatorSessionID)
	assert.Empty(t, diff)
}

func TestLedgerService_GetDiffContent_InvalidHashesReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedger(t)

	diff := lms.GetDiffContent("deadbeef", "cafebabe", "session")
	assert.Empty(t, diff)
}

func TestLedgerService_GetDiffContent_GitDisabledReturnsEmpty(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, nil, nil)

	diff := lms.GetDiffContent("abc123", "def456", "operator-session")
	assert.Empty(t, diff)
}

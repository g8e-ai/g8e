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

func TestLedgerService_GetDiffContent(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	testFilePath := filepath.Join(tempDir, "diff_content_test.txt")
	operatorSessionID := "test-session-diff"

	// 1. Create file (initial commit)
	result1, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)
	err = os.WriteFile(testFilePath, []byte("Line 1\n"), 0644)
	require.NoError(t, err)
	err = lms.CompleteMirrorCreate(result1, operatorSessionID)
	require.NoError(t, err)
	hash1 := result1.LedgerHashAfter

	// 2. Modify file (second commit)
	result2, err := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	require.NoError(t, err)
	err = os.WriteFile(testFilePath, []byte("Line 1\nLine 2\n"), 0644)
	require.NoError(t, err)
	err = lms.CompleteMirrorWrite(result2, operatorSessionID)
	require.NoError(t, err)
	hash2 := result2.LedgerHashAfter

	// 3. Verify diff content via GetDiffContent
	diff := lms.GetDiffContent(hash1, hash2)
	assert.NotEmpty(t, diff)
	assert.Contains(t, diff, "+Line 2")

	// 4. Verify empty hashes return empty string
	assert.Empty(t, lms.GetDiffContent("", hash2))
	assert.Empty(t, lms.GetDiffContent(hash1, ""))
}

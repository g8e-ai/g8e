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

package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestResolveGitNodeBinary_SystemGit(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	resolved := ResolveGitBinary(logger)
	assert.Equal(t, GitEmbedded, resolved)
}

func TestValidateGitNodeBinary_Valid(t *testing.T) {
	t.Parallel()
	version, err := ValidateGitBinary(GitEmbedded)
	require.NoError(t, err)
	assert.Contains(t, version, "go-git v5")
}

func TestValidateGitNodeBinary_Empty(t *testing.T) {
	t.Parallel()
	_, err := ValidateGitBinary("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no git binary path provided")
}

func TestIsExecutable_NotExist(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	assert.False(t, isExecutable(filepath.Join(tmpDir, "nonexistent")))
}

func TestIsExecutable_Directory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	assert.False(t, isExecutable(tmpDir))
}

func TestIsExecutable_NotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not have Unix-style execution bits")
	}
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
	assert.False(t, isExecutable(path))
}

func TestIsExecutable_Executable(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0755))
	assert.True(t, isExecutable(path))
}

func TestTruncateHash(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "abcdef012345", truncateHash("abcdef0123456789"))
	assert.Equal(t, "abcdef012345", truncateHash("abcdef012345"))
	assert.Equal(t, "short", truncateHash("short"))
	assert.Empty(t, truncateHash(""))
}

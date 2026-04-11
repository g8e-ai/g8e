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
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/vsa/testutil"
)

func testGitPath(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available — skipping git-dependent test")
	}
	return gitPath
}

func TestResolveGitBinary_SystemGit(t *testing.T) {
	_ = testGitPath(t)

	logger := testutil.NewTestLogger()
	resolved := ResolveGitBinary(logger)
	require.NotEmpty(t, resolved)
}

func TestResolveGitBinary_ReturnsAbsolutePath(t *testing.T) {
	_ = testGitPath(t)

	logger := testutil.NewTestLogger()
	resolved := ResolveGitBinary(logger)
	require.NotEmpty(t, resolved)
	assert.True(t, filepath.IsAbs(resolved), "expected absolute path, got %q", resolved)
}

func TestValidateGitBinary_Valid(t *testing.T) {
	gitPath := testGitPath(t)

	version, err := ValidateGitBinary(gitPath)
	require.NoError(t, err)
	assert.Contains(t, version, "git version")
}

func TestValidateGitBinary_Empty(t *testing.T) {
	_, err := ValidateGitBinary("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no git binary path provided")
}

func TestValidateGitBinary_Invalid(t *testing.T) {
	_, err := ValidateGitBinary("/nonexistent/git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not functional")
}

func TestIsExecutable_NotExist(t *testing.T) {
	assert.False(t, isExecutable(filepath.Join(t.TempDir(), "nonexistent")))
}

func TestIsExecutable_Directory(t *testing.T) {
	assert.False(t, isExecutable(t.TempDir()))
}

func TestIsExecutable_NotExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
	assert.False(t, isExecutable(path))
}

func TestIsExecutable_Executable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0755))
	assert.True(t, isExecutable(path))
}

func TestTruncateHash(t *testing.T) {
	assert.Equal(t, "abcdef012345", truncateHash("abcdef0123456789"))
	assert.Equal(t, "abcdef012345", truncateHash("abcdef012345"))
	assert.Equal(t, "short", truncateHash("short"))
	assert.Equal(t, "", truncateHash(""))
}

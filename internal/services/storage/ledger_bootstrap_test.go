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
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ---------------------------------------------------------------------------
// Bootstrap helpers
// ---------------------------------------------------------------------------

// setupBootstrapLedger creates a minimal ledger service for bootstrap testing.
// It uses a fresh temp directory and does NOT perform any mutations, so the
// only directory structure present is what bootstrap() created.
func setupBootstrapLedger(t *testing.T) (*GitLedgerService, string) {
	t.Helper()
	gitPath := testGitPath(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

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

	config := &LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}
	svc, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, svc)

	return svc, ledgerDir
}

// ---------------------------------------------------------------------------
// Directory structure tests
// ---------------------------------------------------------------------------

// TestBootstrap_CreatesBaseDir verifies that BaseDir exists immediately after
// construction, even when it did not exist before.
func TestBootstrap_CreatesBaseDir(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	info, err := os.Stat(ledgerDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestBootstrap_CreatesFilesDir verifies that the files/ subdirectory exists
// immediately after construction.
func TestBootstrap_CreatesFilesDir(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	info, err := os.Stat(filesDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestBootstrap_CreatesSessionsDir verifies that the sessions/ subdirectory
// exists immediately after construction.
func TestBootstrap_CreatesSessionsDir(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	sessionsDir := filepath.Join(ledgerDir, constants.SessionsDirname)
	info, err := os.Stat(sessionsDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestBootstrap_FilesDirHasGitRepo verifies that the files/ directory has a
// valid .git directory immediately after construction.
func TestBootstrap_FilesDirHasGitRepo(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	gitDir := filepath.Join(ledgerDir, constants.FilesDirname, constants.GitDirname)
	info, err := os.Stat(gitDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), ".git directory must exist in files/")
}

// TestBootstrap_GitignoreExistsInFilesDir verifies that the .gitignore file
// was created in the files/ directory during bootstrap.
func TestBootstrap_GitignoreExistsInFilesDir(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	gitignorePath := filepath.Join(ledgerDir, constants.FilesDirname, constants.GitignoreFilename)
	info, err := os.Stat(gitignorePath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

// TestBootstrap_AllPathsExist is a comprehensive single-check test that
// verifies the entire expected directory tree in one assertion block.
func TestBootstrap_AllPathsExist(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	expectedPaths := []string{
		ledgerDir,
		filepath.Join(ledgerDir, constants.FilesDirname),
		filepath.Join(ledgerDir, constants.FilesDirname, constants.GitDirname),
		filepath.Join(ledgerDir, constants.FilesDirname, constants.GitignoreFilename),
		filepath.Join(ledgerDir, constants.SessionsDirname),
	}

	for _, p := range expectedPaths {
		_, err := os.Stat(p)
		require.NoError(t, err, "expected path to exist: %s", p)
	}
}

// ---------------------------------------------------------------------------
// Git repo state tests (no mutations performed)
// ---------------------------------------------------------------------------

// TestBootstrap_FilesDirHasInitialCommit verifies that the files/ git repo
// has at least one commit (the initial commit) immediately after construction.
// This is the core regression test: before the fix, no git repo existed at all
// until a mutation triggered lazy init.
func TestBootstrap_FilesDirHasInitialCommit(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	repo, err := git.PlainOpen(filesDir)
	require.NoError(t, err)

	ref, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)
	assert.Equal(t, "Initial ledger commit", strings.TrimSpace(commit.Message))
}

// TestBootstrap_InitialCommitAuthor verifies the initial commit has the
// expected g8e-operator author identity.
func TestBootstrap_InitialCommitAuthor(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	repo, err := git.PlainOpen(filesDir)
	require.NoError(t, err)

	ref, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)
	assert.Equal(t, "g8e-operator", commit.Author.Name)
	assert.Equal(t, "g8e-operator@system", commit.Author.Email)
}

// TestBootstrap_GetStateMerkleRootImmediately verifies that GetStateMerkleRoot
// returns a non-empty hash immediately after construction, without any prior
// file mutations. This was the exact scenario that failed before the fix:
// scenarios that never trigger a file write (e.g. HTTP-only operations) would
// get an error because the files/ git repo didn't exist.
func TestBootstrap_GetStateMerkleRootImmediately(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	root, err := svc.GetStateMerkleRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
	assert.Len(t, root, 40, "expected a 40-char SHA-1 hash")
}

// TestBootstrap_ListCommitsImmediately verifies that ListCommits returns at
// least the initial commit immediately after construction, with no mutations.
func TestBootstrap_ListCommitsImmediately(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	commits, err := svc.ListCommits("", 100)
	require.NoError(t, err)
	require.NotEmpty(t, commits, "expected at least the initial commit")

	first := commits[0]
	assert.NotEmpty(t, first.CommitHash)
	assert.Equal(t, "Initial ledger commit", strings.TrimSpace(first.Message))
	assert.False(t, first.TimestampUTC.IsZero())
}

// TestBootstrap_ListCommitsEmptySessionUsesFilesDir verifies that ListCommits
// with an empty session ID queries the default files/ repo, not a session repo.
func TestBootstrap_ListCommitsEmptySessionUsesFilesDir(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	commits, err := svc.ListCommits("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, commits)
}

// ---------------------------------------------------------------------------
// Idempotency tests
// ---------------------------------------------------------------------------

// TestBootstrap_InitGitRepoIdempotent verifies that calling initGitRepo on a
// directory that already has a .git directory is a no-op and does not error.
func TestBootstrap_InitGitRepoIdempotent(t *testing.T) {
	t.Parallel()
	svc, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)

	// The repo was already initialized during bootstrap. Calling again must not fail.
	err := svc.initGitRepo(filesDir)
	require.NoError(t, err)

	// Verify the repo is still valid
	repo, err := git.PlainOpen(filesDir)
	require.NoError(t, err)
	ref, err := repo.Head()
	require.NoError(t, err)
	assert.NotEmpty(t, ref.Hash().String())
}

// TestBootstrap_ReconstructionOverExistingDir verifies that constructing a
// second GitLedgerService over an already-bootstrapped directory succeeds
// (bootstrap must be safe to run on an existing structure).
func TestBootstrap_ReconstructionOverExistingDir(t *testing.T) {
	t.Parallel()
	gitPath := testGitPath(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

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

	config := &LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}

	// First construction bootstraps the directory
	svc1, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// Capture the initial commit hash
	root1, err := svc1.GetStateMerkleRoot()
	require.NoError(t, err)

	// Second construction over the same directory must succeed
	svc2, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// The initial commit should still be the HEAD (initGitRepo is idempotent)
	root2, err := svc2.GetStateMerkleRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "reconstruction must not alter the existing git history")
}

// ---------------------------------------------------------------------------
// Constructor validation tests
// ---------------------------------------------------------------------------

// TestBootstrap_NilConfigReturnsError verifies that a nil config is rejected.
func TestBootstrap_NilConfigReturnsError(t *testing.T) {
	t.Parallel()
	svc, err := NewGitLedgerService(nil, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "config")
}

// TestBootstrap_NilVaultReturnsError verifies that a nil vault is rejected.
func TestBootstrap_NilVaultReturnsError(t *testing.T) {
	t.Parallel()
	config := &LedgerConfig{
		BaseDir: t.TempDir(),
		GitPath: "/usr/bin/git",
	}
	svc, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "vault")
}

// TestBootstrap_BootstrapFailureOnUnwritableBaseDir verifies that bootstrap
// returns an error when the base directory cannot be created (e.g. under a
// read-only parent).
func TestBootstrap_BootstrapFailureOnUnwritableBaseDir(t *testing.T) {
	t.Parallel()

	// Create a file to block directory creation — os.MkdirAll will fail because
	// the path component is a file, not a directory. This is cross-platform,
	// unlike chmod 0555 which doesn't prevent writes on Windows.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(t.TempDir(), "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { testVault.Close() })

	config := &LedgerConfig{
		BaseDir:         filepath.Join(blocker, "subdir", "ledger"),
		GitPath:         testGitPath(t),
		EncryptionVault: testVault,
	}
	svc, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, svc)
}

// ---------------------------------------------------------------------------
// Session management tests (lazy init still works alongside bootstrap)
// ---------------------------------------------------------------------------

// TestBootstrap_GetSessionLedgerPathEmptyReturnsFilesDir verifies that an
// empty session ID returns the default files/ directory (not a session repo).
func TestBootstrap_GetSessionLedgerPathEmptyReturnsFilesDir(t *testing.T) {
	t.Parallel()
	svc, ledgerDir := setupBootstrapLedger(t)

	path, err := svc.GetSessionLedgerPath("")
	require.NoError(t, err)

	expectedFilesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	assert.Equal(t, expectedFilesDir, path)
}

// TestBootstrap_SessionRepoCreatedLazily verifies that a per-session git repo
// is created on first access (not during bootstrap) and has a valid initial commit.
func TestBootstrap_SessionRepoCreatedLazily(t *testing.T) {
	t.Parallel()
	svc, ledgerDir := setupBootstrapLedger(t)

	sessionID := "bootstrap-test-session"
	sessionPath := filepath.Join(ledgerDir, constants.SessionsDirname, sessionID)

	// Before calling GetSessionLedgerPath, the session directory should not exist
	_, err := os.Stat(filepath.Join(sessionPath, constants.GitDirname))
	assert.True(t, os.IsNotExist(err), "session .git should not exist before first access")

	// After calling, it should exist and be a valid git repo
	path, err := svc.GetSessionLedgerPath(sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionPath, path)

	_, err = os.Stat(filepath.Join(sessionPath, constants.GitDirname))
	require.NoError(t, err, "session .git must exist after GetSessionLedgerPath")

	// Verify the session repo has an initial commit
	repo, err := git.PlainOpen(sessionPath)
	require.NoError(t, err)
	ref, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)
	assert.Equal(t, "Initial ledger commit", strings.TrimSpace(commit.Message))
}

// TestBootstrap_SessionRepoIdempotent verifies that calling GetSessionLedgerPath
// twice for the same session ID returns the same path and does not reinitialize.
func TestBootstrap_SessionRepoIdempotent(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	sessionID := "idempotent-session"

	path1, err := svc.GetSessionLedgerPath(sessionID)
	require.NoError(t, err)

	path2, err := svc.GetSessionLedgerPath(sessionID)
	require.NoError(t, err)

	assert.Equal(t, path1, path2)
}

// ---------------------------------------------------------------------------
// End-to-end: git operations work immediately after construction
// ---------------------------------------------------------------------------

// TestBootstrap_MutationWorksImmediatelyAfterConstruction verifies that a
// full write-mirror cycle works immediately after construction without any
// prior "warm-up" mutations. This simulates the dow-cross-cue scenario where
// the operator starts, performs HTTP-only actions, and then later needs to
// write a file.
func TestBootstrap_MutationWorksImmediatelyAfterConstruction(t *testing.T) {
	t.Parallel()
	svc, tempDir := setupBootstrapLedger(t)

	// No mutations have been performed yet. Directly do a file write.
	testFile := filepath.Join(tempDir, "immediate_write.txt")
	result, err := svc.MirrorFileCreate("sess-immediate", testFile)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NoError(t, os.WriteFile(testFile, []byte("immediate content"), 0644))
	require.NoError(t, svc.CompleteMirrorCreate(result, "sess-immediate"))

	assert.NotEmpty(t, result.LedgerHashAfter)

	// Verify the file was mirrored
	mirrored, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, "immediate content", string(mirrored))
}

// TestBootstrap_DiffStatWorksImmediatelyAfterConstruction verifies that
// diff operations work on the bootstrapped repo without prior mutations.
// This uses the public two-phase mirror API to ensure the full pipeline
// works from a freshly bootstrapped state.
func TestBootstrap_DiffStatWorksImmediatelyAfterConstruction(t *testing.T) {
	t.Parallel()
	svc, tempDir := setupBootstrapLedger(t)

	// First mutation: create a file
	testFile := filepath.Join(tempDir, "diffstat_immediate.txt")
	result1, err := svc.MirrorFileCreate("sess-diffstat-boot", testFile)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\n"), 0644))
	require.NoError(t, svc.CompleteMirrorCreate(result1, "sess-diffstat-boot"))

	hashBefore := result1.LedgerHashAfter
	require.NotEmpty(t, hashBefore)

	// Second mutation: modify the file
	result2, err := svc.LedgerFileWrite("sess-diffstat-boot", testFile)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(testFile, []byte("line one\nline two\nline three\n"), 0644))
	require.NoError(t, svc.CompleteMirrorWrite(result2, "sess-diffstat-boot"))

	hashAfter := result2.LedgerHashAfter
	require.NotEmpty(t, hashAfter)
	assert.NotEqual(t, hashBefore, hashAfter)

	// Diff stat should work on the bootstrapped repo
	stat := svc.GetDiffStat(hashBefore, hashAfter, "sess-diffstat-boot")
	assert.NotEmpty(t, stat)
}

// TestBootstrap_GetFileHistoryEmptyRepo verifies that GetFileHistory on the
// freshly bootstrapped repo returns no error and empty results.
func TestBootstrap_GetFileHistoryEmptyRepo(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	history, err := svc.GetFileHistory("/some/nonexistent/file.txt", 10, "")
	require.NoError(t, err)
	assert.Empty(t, history)
}

// ---------------------------------------------------------------------------
// Concurrent bootstrap safety
// ---------------------------------------------------------------------------

// TestBootstrap_ConcurrentReadersAfterConstruction verifies that multiple
// goroutines can read from the bootstrapped repo concurrently without issues.
func TestBootstrap_ConcurrentReadersAfterConstruction(t *testing.T) {
	t.Parallel()
	svc, _ := setupBootstrapLedger(t)

	const goroutines = 10
	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := svc.GetStateMerkleRoot()
			done <- err
		}()
	}

	for i := 0; i < goroutines; i++ {
		err := <-done
		assert.NoError(t, err)
	}
}

// ---------------------------------------------------------------------------
// Gitignore content test
// ---------------------------------------------------------------------------

// TestBootstrap_GitignoreContent verifies the .gitignore file in the files/
// directory has the expected content.
func TestBootstrap_GitignoreContent(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	gitignorePath := filepath.Join(ledgerDir, constants.FilesDirname, constants.GitignoreFilename)
	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "g8e Ledger")
}

// ---------------------------------------------------------------------------
// Files dir git repo is a proper worktree (not bare)
// ---------------------------------------------------------------------------

// TestBootstrap_FilesDirIsNotBareRepo verifies the files/ repo is a working
// tree (not bare), so worktree operations like Add and Commit work correctly.
func TestBootstrap_FilesDirIsNotBareRepo(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	repo, err := git.PlainOpen(filesDir)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)
	assert.NotNil(t, w)
}

// TestBootstrap_FilesDirWorktreeStatusClean verifies that after bootstrap, the
// files/ worktree has a clean status (the initial commit captured everything).
func TestBootstrap_FilesDirWorktreeStatusClean(t *testing.T) {
	t.Parallel()
	_, ledgerDir := setupBootstrapLedger(t)

	filesDir := filepath.Join(ledgerDir, constants.FilesDirname)
	repo, err := git.PlainOpen(filesDir)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)

	status, err := w.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean(), "worktree should be clean after bootstrap")
}

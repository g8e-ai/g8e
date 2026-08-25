// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Phase 3.10 CredentialStore tests (§4.1 deferred items) ---
//
// These exercise CredentialStore directly (not through the command layer):
//   - Staged-commit retry after an interrupted commit.
//   - Rollback releases staged state without writing canonical files.
//   - Permissions: committed files are created with PermFilePrivate (0600).
//   - Enrollment-lock: concurrent Stage calls are serialized by the mutex.

// assertNoOrphanedTmpFiles walks the runtime directory and fails if any
// `.g8e-tmp-*` file remains from an interrupted atomic write.
func assertNoOrphanedTmpFiles(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".g8e-tmp-") {
			t.Errorf("orphaned temp file remains: %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}

// TestCredentialStore_InterruptedCommitRetry verifies that after a Commit
// fails (simulated via a cancelled context), a second Stage+Commit succeeds
// and leaves no orphaned tmp files. This is the §4.1 staged-commit retry
// scenario.
func TestCredentialStore_InterruptedCommitRetry(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)

	// Stage succeeds — it only validates in memory.
	staged, err := store.Stage(context.Background(), artifacts)
	require.NoError(t, err)
	require.NotNil(t, staged)

	// Commit with a cancelled context fails before writing canonical files.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.Commit(cancelledCtx, staged)
	require.Error(t, err)

	// No canonical credential files should exist yet.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Nil(t, creds, "no credentials should be committed after failed commit")

	// Retry: a fresh Stage+Commit must succeed.
	staged2, err := store.Stage(context.Background(), artifacts)
	require.NoError(t, err)
	require.NoError(t, store.Commit(context.Background(), staged2))

	// Credentials are now committed.
	creds, err = LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, artifacts.CLISessionID, creds.CLISessionID)
	assert.Equal(t, artifacts.UserID, creds.UserID)

	// No orphaned tmp files remain in the runtime tree.
	assertNoOrphanedTmpFiles(t, fileSvc.Resolve(""))
}

// TestCredentialStore_RollbackWritesNoCanonicalFiles verifies that Rollback
// releases the staged identity without writing any canonical managed files.
// The previously committed identity (if any) remains in place.
func TestCredentialStore_RollbackWritesNoCanonicalFiles(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)

	// Stage, then Rollback without committing.
	staged, err := store.Stage(context.Background(), artifacts)
	require.NoError(t, err)
	store.Rollback(staged)

	// No canonical files should exist.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Nil(t, creds, "Rollback must not write credentials JSON")

	certExists, err := fileSvc.FileExists(context.Background(), cfg.CLICertFile())
	require.NoError(t, err)
	assert.False(t, certExists, "Rollback must not write CLI cert")

	keyExists, err := fileSvc.FileExists(context.Background(), cfg.CLIKeyFile())
	require.NoError(t, err)
	assert.False(t, keyExists, "Rollback must not write CLI key")

	// Rollback(nil) is a safe no-op.
	store.Rollback(nil)
}

// TestCredentialStore_CommittedFilePermissions verifies that committed CLI
// credential files are created with PermFilePrivate (0600). The trust bundle
// is written with PermFilePublic (0644) since it must be readable by
// subprocesses. This is the §4.1 permissions assertion.
func TestCredentialStore_CommittedFilePermissions(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)

	staged, err := store.Stage(context.Background(), artifacts)
	require.NoError(t, err)
	require.NoError(t, store.Commit(context.Background(), staged))

	// CLI cert, CLI key, and credentials JSON must be 0600.
	for _, abs := range []string{cfg.CLICertFile(), cfg.CLIKeyFile(), cfg.CredentialsFile()} {
		rel, err := fileSvc.RelFromAbs(abs)
		require.NoError(t, err)
		info, err := fileSvc.Stat(context.Background(), rel)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(),
			"file must have private permissions (0600): %s", abs)
	}
}

// TestCredentialStore_ConcurrentStageCommitNoTornState verifies that two
// concurrent Stage+Commit sequences do not produce torn state. The mutex
// serializes individual Stage and Commit operations, and the atomic
// tmp+rename write pattern ensures the final state is one of the two
// committed identities — never a mix of files from both.
func TestCredentialStore_ConcurrentStageCommitNoTornState(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)

	artifactsA := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	artifactsA.CLISessionID = "cli-session-A"
	artifactsA.UserID = "user-A"

	artifactsB := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	artifactsB.CLISessionID = "cli-session-B"
	artifactsB.UserID = "user-B"

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	for i, a := range []EnrollmentArtifacts{artifactsA, artifactsB} {
		idx := i
		art := a
		go func() {
			defer wg.Done()
			staged, err := store.Stage(context.Background(), art)
			if err != nil {
				errs[idx] = err
				return
			}
			errs[idx] = store.Commit(context.Background(), staged)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d Stage+Commit failed", i)
	}

	// The final committed state must be one of the two identities — not a
	// mix. Verify the credentials JSON, CLI cert, and CLI key are mutually
	// consistent by loading and inspecting.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, creds)

	// The session ID must be one of A or B (no torn/partial value).
	assert.Contains(t, []string{"cli-session-A", "cli-session-B"}, creds.CLISessionID)
	assert.Contains(t, []string{"user-A", "user-B"}, creds.UserID)

	// Inspect must report a complete, consistent state (not partial/corrupt).
	inspection, err := store.Inspect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, LocalStateComplete, inspection.State,
		"concurrent commits must leave a complete, consistent identity")
	assert.True(t, inspection.KeyMatchesCert,
		"CLI cert and key must match after concurrent commits")

	// No orphaned tmp files.
	assertNoOrphanedTmpFiles(t, fileSvc.Resolve(""))
}

// TestCredentialStore_ClearRetainsTrustBundle verifies that Clear (logout)
// removes the local CLI credential material (credentials JSON, CLI cert,
// CLI key) but does NOT remove the runtime trust bundle. This is the §4.3
// ownership policy: the OS root CA is shared and must survive logout.
func TestCredentialStore_ClearRetainsTrustBundle(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)

	staged, err := store.Stage(context.Background(), artifacts)
	require.NoError(t, err)
	require.NoError(t, store.Commit(context.Background(), staged))

	// Trust bundle exists after commit.
	bundleExists, err := fileSvc.FileExists(context.Background(), cfg.DefaultTrustBundleRelPath())
	require.NoError(t, err)
	assert.True(t, bundleExists, "trust bundle must exist after commit")

	// Clear removes local credentials but retains the trust bundle.
	require.NoError(t, store.Clear(context.Background()))

	for _, abs := range []string{cfg.CredentialsFile(), cfg.CLICertFile(), cfg.CLIKeyFile()} {
		rel, err := fileSvc.RelFromAbs(abs)
		require.NoError(t, err)
		exists, err := fileSvc.FileExists(context.Background(), rel)
		require.NoError(t, err)
		assert.False(t, exists, "Clear must remove local credential file: %s", abs)
	}

	bundleExists, err = fileSvc.FileExists(context.Background(), cfg.DefaultTrustBundleRelPath())
	require.NoError(t, err)
	assert.True(t, bundleExists, "Clear must NOT remove the runtime trust bundle (shared OS root CA)")

	// Calling Clear a second time when files are already gone is idempotent and succeeds.
	require.NoError(t, store.Clear(context.Background()))
}

// TestCredentialStore_InspectBundleOnlyIsAbsent verifies that a trust
// bundle written by `gw start` (with no CLI identity artifacts present)
// does not promote the local identity state from Absent to Partial. The
// trust bundle is a shared gateway artifact (§4.3) and is not part of the
// CLI identity set. Without this, `auth enroll user` on a freshly-started but
// unbootstrapped gateway would route to the recovery endpoint and receive
// a 403 ("CLI recovery only available after bootstrap").
func TestCredentialStore_InspectBundleOnlyIsAbsent(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)

	// Write only the trust bundle — no credentials, cert, or key.
	bundlePEM := testutil.GenerateTestCA(t, "test-root-ca")
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, []byte(bundlePEM), constants.PermFilePublic))

	inspection, err := store.Inspect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, LocalStateAbsent, inspection.State,
		"trust bundle alone must not promote absent identity to partial")
}

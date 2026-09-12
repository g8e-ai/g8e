// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package buildinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// goldenFixtureDigest is the digest produced by the Python reference
// implementation (g8e_evals.preflight.compute_source_tree_state_hash) for
// the fixture built by writeParityFixture. The same constant is asserted
// in ensemble/evals/tests/test_source_tree_hash_parity.py, pinning the
// canonical digest format across both implementations.
const goldenFixtureDigest = "36ee160f6da7ad1edd2ffebe9086a825c7361dacbb77238f33d432964b0e634f"

// writeParityFixture builds the cross-language parity fixture. The layout
// deliberately exercises path-component ordering: "a" is a directory whose
// entries must sort before "a.b" and "a.txt" even though '.' < '/' in
// byte order. Mirrored byte-for-byte by the Python parity test.
func writeParityFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"a.txt":         "alpha",
		"a/b.txt":       "beta",
		"a/c.txt":       "chi",
		"a.b/d.txt":     "delta",
		".hidden":       "hidden",
		"dir/sub/c.txt": "gamma",
	}
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	}
}

func TestComputeSourceTreeHash_MatchesPythonReferenceDigest(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)

	digest, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	assert.Equal(t, goldenFixtureDigest, digest)
}

func TestComputeSourceTreeHash_DeterministicAcrossRuns(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)

	first, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	second, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestComputeSourceTreeHash_ContentChangeChangesDigest(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)

	before, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha-changed"), 0o644))
	after, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}

func TestComputeSourceTreeHash_RenamedFileChangesDigest(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)

	before, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	require.NoError(t, os.Rename(filepath.Join(root, "a.txt"), filepath.Join(root, "renamed.txt")))
	after, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}

func TestComputeSourceTreeHash_RejectsSymlink(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)
	require.NoError(t, os.Symlink("a.txt", filepath.Join(root, "link.txt")))

	_, err := ComputeSourceTreeHash(root)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeSymlink)
}

func TestComputeSourceTreeHash_RejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)
	require.NoError(t, os.Symlink("a", filepath.Join(root, "linkdir")))

	_, err := ComputeSourceTreeHash(root)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeSymlink)
}

func TestComputeSourceTreeHash_RejectsNonDirectoryRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	_, err := ComputeSourceTreeHash(file)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeNotDir)
}

func TestComputeSourceTreeHash_ExcludesMatchingComponents(t *testing.T) {
	root := t.TempDir()
	writeParityFixture(t, root)
	cache := filepath.Join(root, "dir", "__pycache__")
	require.NoError(t, os.MkdirAll(cache, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cache, "c.pyc"), []byte("junk"), 0o644))

	withJunk, err := ComputeSourceTreeHash(root)
	require.NoError(t, err)
	excluded, err := ComputeSourceTreeHash(root, "__pycache__")
	require.NoError(t, err)

	// Excluding the cache dir leaves exactly the parity fixture.
	assert.NotEqual(t, withJunk, excluded)
	assert.Equal(t, goldenFixtureDigest, excluded)
}

func TestComputeSourceManifestHash_KeysRetainEntryPrefix(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)

	// Manifest mode keys files by their path relative to base, so
	// "dir/sub/c.txt" contributes under its full repo-relative name. The
	// digest therefore differs from the single-root digest of base.
	manifest, err := ComputeSourceManifestHash(base, []string{"dir", "a.txt"}, nil)
	require.NoError(t, err)
	whole, err := ComputeSourceTreeHash(base)
	require.NoError(t, err)
	assert.NotEqual(t, manifest, whole)
	assert.Len(t, manifest, 64)
}

func TestComputeSourceManifestHash_RejectsMissingEntry(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)

	_, err := ComputeSourceManifestHash(base, []string{"does-not-exist"}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeEntryNotFound)
}

func TestComputeSourceManifestHash_RejectsEscapingEntry(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)

	for _, entry := range []string{"../outside", "/absolute/path", "a/../..", ""} {
		_, err := ComputeSourceManifestHash(base, []string{entry}, nil)
		require.Error(t, err, "entry %q must be rejected", entry)
		assert.ErrorIs(t, err, constants.ErrSourceTreeEntryInvalid)
	}
}

func TestComputeSourceManifestHash_RejectsSymlinkEntry(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "ext")))

	_, err := ComputeSourceManifestHash(base, []string{"ext"}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeSymlink)
}

func TestComputeSourceManifestHash_ExcludeSkipsNestedGeneratedDirs(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)
	venv := filepath.Join(base, "pkg", ".venv", "bin")
	require.NoError(t, os.MkdirAll(venv, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(venv, "python"), []byte("x"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "pkg", "real.py"), []byte("y"), 0o644))

	_, err := ComputeSourceManifestHash(base, []string{"pkg"}, nil)
	// No excludes: the .venv content is hashed in (and a symlink inside it
	// would be rejected). With excludes, only real.py remains.
	require.NoError(t, err)
	excluded, err := ComputeSourceManifestHash(base, []string{"pkg"}, []string{".venv"})
	require.NoError(t, err)

	// Excluded run must equal hashing the manifest without .venv at all.
	require.NoError(t, os.RemoveAll(filepath.Join(base, "pkg", ".venv")))
	clean, err := ComputeSourceManifestHash(base, []string{"pkg"}, nil)
	require.NoError(t, err)
	assert.Equal(t, clean, excluded)
}

func TestSourceTreeHash_ManifestModeWithoutGit(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)

	digest, err := SourceTreeHash(base, []string{"dir"}, nil)
	require.NoError(t, err)
	assert.Len(t, digest, 64)
}

func TestSourceTreeHash_ManifestModeRejectsEmptyEntries(t *testing.T) {
	base := t.TempDir()
	writeParityFixture(t, base)

	_, err := SourceTreeHash(base, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSourceTreeEntryNotFound)
}

func TestComputeTrackedSourceHash_GitWorktree(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	if _, err := git(repoRoot, "rev-parse", "--git-dir"); err != nil {
		t.Skip("not inside a git work tree")
	}

	digest, err := ComputeTrackedSourceHash(repoRoot)
	require.NoError(t, err)
	assert.Len(t, digest, 64)

	// Deterministic on unchanged source.
	again, err := ComputeTrackedSourceHash(repoRoot)
	require.NoError(t, err)
	assert.Equal(t, digest, again)
}

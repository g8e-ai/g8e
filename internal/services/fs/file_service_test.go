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

package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFS creates a RuntimeFileService backed by a temp directory.
// It calls paths.InitWithBase so paths.Infra is populated for CreateRuntimeTree.
func setupTestFS(t *testing.T) RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	require.NoError(t, paths.InitWithBase(baseDir))
	svc, err := NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	return svc
}

func TestNewRuntimeFileService_EmptyBaseDirUsesCWD(t *testing.T) {
	baseDir := testutil.TempDir(t)
	require.NoError(t, paths.InitWithBase(baseDir))

	svc, err := NewRuntimeFileService("", testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestResolve_EmptyPathReturnsRuntimeDir(t *testing.T) {
	svc := setupTestFS(t)
	abs := svc.Resolve("")
	assert.Contains(t, abs, constants.RuntimeDirname)
}

func TestResolve_RelativePathJoinsRuntimeDir(t *testing.T) {
	svc := setupTestFS(t)
	abs := svc.Resolve("pki/root_ca.crt")
	assert.Contains(t, abs, constants.RuntimeDirname)
	assert.Contains(t, abs, "pki")
	assert.Contains(t, abs, "root_ca.crt")
}

func TestWriteFile_CreatesFileWithCorrectContent(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	data := []byte("hello world")
	err := svc.WriteFile(ctx, "test.txt", data, constants.PermFilePrivate)
	require.NoError(t, err)

	got, err := svc.ReadFile(ctx, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestWriteFile_CreatesParentDirectories(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.WriteFile(ctx, "nested/deep/dir/file.txt", []byte("data"), constants.PermFilePrivate)
	require.NoError(t, err)

	got, err := svc.ReadFile(ctx, "nested/deep/dir/file.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)
}

func TestWriteFile_EnforcesPermissions(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.WriteFile(ctx, "secret.txt", []byte("secret"), constants.PermFilePrivate)
	require.NoError(t, err)

	info, err := svc.Stat(ctx, "secret.txt")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
}

func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "overwrite.txt", []byte("old"), constants.PermFilePrivate))
	require.NoError(t, svc.WriteFile(ctx, "overwrite.txt", []byte("new"), constants.PermFilePrivate))

	got, err := svc.ReadFile(ctx, "overwrite.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)
}

func TestWriteFile_NoTempFilesRemain(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "clean.txt", []byte("data"), constants.PermFilePrivate))

	absDir := svc.Resolve("")
	entries, err := os.ReadDir(absDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".g8e-tmp-"),
			"temp file remains: %s", e.Name())
	}
}

func TestReadFile_NonexistentReturnsErrNotFound(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	_, err := svc.ReadFile(ctx, "does-not-exist.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestFileExists_ExistingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "exists.txt", []byte("data"), constants.PermFilePrivate))

	exists, err := svc.FileExists(ctx, "exists.txt")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestFileExists_NonexistentFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	exists, err := svc.FileExists(ctx, "nope.txt")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStat_ReturnsFileInfo(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "stat.txt", []byte("data"), constants.PermFilePrivate))

	info, err := svc.Stat(ctx, "stat.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(4), info.Size())
	assert.False(t, info.IsDir())
}

func TestStat_NonexistentReturnsErrNotFound(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	_, err := svc.Stat(ctx, "nope.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestRemove_ExistingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "remove.txt", []byte("data"), constants.PermFilePrivate))
	require.NoError(t, svc.Remove(ctx, "remove.txt"))

	exists, err := svc.FileExists(ctx, "remove.txt")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemove_NonexistentIsNoOp(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.Remove(ctx, "never-existed.txt")
	assert.NoError(t, err)
}

func TestRemoveAll_ExistingDirectory(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.MkdirAll(ctx, "rmdir", constants.PermDirStandard))
	require.NoError(t, svc.WriteFile(ctx, "rmdir/file.txt", []byte("data"), constants.PermFilePrivate))
	require.NoError(t, svc.RemoveAll(ctx, "rmdir"))

	exists, err := svc.FileExists(ctx, "rmdir")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemoveAll_NonexistentIsNoOp(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.RemoveAll(ctx, "never-existed")
	assert.NoError(t, err)
}

func TestReadDir_ListsEntries(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "dir/a.txt", []byte("a"), constants.PermFilePrivate))
	require.NoError(t, svc.WriteFile(ctx, "dir/b.txt", []byte("b"), constants.PermFilePrivate))

	entries, err := svc.ReadDir(ctx, "dir")
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	names := []string{entries[0].Name(), entries[1].Name()}
	assert.Contains(t, names, "a.txt")
	assert.Contains(t, names, "b.txt")
}

func TestReadDir_NonexistentReturnsErrNotFound(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	_, err := svc.ReadDir(ctx, "no-dir")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestRename_RenamesFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "old.txt", []byte("data"), constants.PermFilePrivate))
	require.NoError(t, svc.Rename(ctx, "old.txt", "new.txt"))

	got, err := svc.ReadFile(ctx, "new.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)

	exists, _ := svc.FileExists(ctx, "old.txt")
	assert.False(t, exists)
}

func TestMkdirAll_CreatesNestedDirectories(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.MkdirAll(ctx, "a/b/c", constants.PermDirStandard)
	require.NoError(t, err)

	info, err := svc.Stat(ctx, "a/b/c")
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCreateRuntimeTree_CreatesAllDirectories(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.CreateRuntimeTree(ctx)
	require.NoError(t, err)

	dirs := []string{
		paths.Infra.DataDir,
		paths.Infra.PkiDir,
		paths.Infra.PkiRootDir,
		paths.Infra.PkiAuthoritiesDir,
		paths.Infra.PkiIssuedDir,
		paths.Infra.PkiIssuedHubDir,
		paths.Infra.PkiIssuedGatewayPeerDir,
		paths.Infra.AppCertDir,
		paths.Infra.PkiTrustDir,
		paths.Infra.PkiRevocationDir,
		paths.Infra.PkiBinariesDir,
		paths.Infra.TrustedSignersDir,
		paths.Infra.ClientPkiDir,
		paths.Infra.SecretsDir,
		paths.Infra.VaultDir,
		paths.Infra.LedgerDir,
		paths.Infra.LedgerFilesDir,
		paths.Infra.LogDir,
		paths.Infra.PidDir,
		paths.Infra.BinDir,
		paths.Infra.ProtocolDir,
		paths.Infra.DocsDir,
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		require.NoError(t, err, "directory not created: %s", dir)
		assert.True(t, info.IsDir(), "not a directory: %s", dir)
	}
}

func TestCreateRuntimeTree_SecretsDirIsPrivate(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.SecretsDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirPrivate), info.Mode().Perm())
}

func TestCreateRuntimeTree_VaultDirIsPrivate(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.VaultDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirPrivate), info.Mode().Perm())
}

func TestCreateRuntimeTree_PkiDirsAreStandard(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	pkiDirs := []string{
		paths.Infra.PkiDir,
		paths.Infra.PkiRootDir,
		paths.Infra.PkiAuthoritiesDir,
		paths.Infra.PkiIssuedDir,
		paths.Infra.PkiTrustDir,
	}

	for _, dir := range pkiDirs {
		info, err := os.Stat(dir)
		require.NoError(t, err, "failed to stat: %s", dir)
		assert.Equal(t, os.FileMode(constants.PermDirStandard), info.Mode().Perm(),
			"wrong permissions for: %s", dir)
	}
}

func TestCreateRuntimeTree_Idempotent(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))
	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.PkiDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnforceFilePermissions_SetsMode(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "perm.txt", []byte("data"), constants.PermFilePublic))
	require.NoError(t, svc.EnforceFilePermissions(ctx, "perm.txt", constants.PermFilePrivate))

	info, err := svc.Stat(ctx, "perm.txt")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
}

func TestEnforceDirPermissions_Recursive(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.MkdirAll(ctx, "enforce/sub", constants.PermDirStandard))
	require.NoError(t, svc.WriteFile(ctx, "enforce/sub/file.txt", []byte("data"), constants.PermFilePublic))
	require.NoError(t, svc.EnforceDirPermissions(ctx, "enforce", constants.PermDirPrivate))

	info, err := svc.Stat(ctx, "enforce/sub/file.txt")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirPrivate), info.Mode().Perm())

	info, err = svc.Stat(ctx, "enforce/sub")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirPrivate), info.Mode().Perm())
}

func TestWriteFile_CancelledContextReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.WriteFile(ctx, "cancelled.txt", []byte("data"), constants.PermFilePrivate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestReadFile_CancelledContextReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ReadFile(ctx, "anything.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestRelFromAbs_Success(t *testing.T) {
	svc := setupTestFS(t)
	abs := svc.Resolve("pki/root_ca.crt")
	rel, err := svc.RelFromAbs(abs)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("pki", "root_ca.crt"), rel)
}

func TestRelFromAbs_OutsidePathReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	outside := filepath.Join(os.TempDir(), "outside-g8e", "cert.pem")
	_, err := svc.RelFromAbs(outside)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrPathValidation))
}

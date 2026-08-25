// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	vault "github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

// testGitPath returns the system git binary path, skipping the test if git is unavailable.
func testGitPath(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available - skipping git-dependent test")
	}
	return gitPath
}

// newTestFileSvc creates a RuntimeFileService backed by baseDir with the full
// .g8e runtime tree created. Returns the file service and the data directory
// path (fileSvc.Resolve(constants.DataDirname)).
func newTestFileSvc(t *testing.T, baseDir string) (fs.RuntimeFileService, string) {
	t.Helper()
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc, svc.Resolve(constants.DataDirname)
}

// CreateTestVault creates a new unlocked Vault in the given directory using the provided private key.
// The vault header is initialized and the vault is unlocked. Cleanup closes it via t.Cleanup.
func CreateTestVault(t testing.TB, dataDir string, privateKey []byte) *vault.Vault {
	t.Helper()

	require.NoError(t, os.MkdirAll(dataDir, 0700))

	logger := testutil.NewTestLogger()

	header, _, err := vault.NewVaultHeader(privateKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(dataDir))

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dataDir,
		Logger:  logger,
	})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(privateKey))

	t.Cleanup(func() { v.Close() })
	return v
}

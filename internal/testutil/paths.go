// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestPaths holds test-specific filesystem paths.
// This allows tests to have isolated path structures without mutating
// the global constants.Paths package-level state, which causes race
// conditions in parallel test execution.
type TestPaths struct {
	BaseDir           string
	RuntimeDir        string
	DataDir           string
	PKIDir            string
	SecretsDir        string
	VaultDir          string
	VaultKeyPath      string
	TestVaultDir      string
	ProtocolDir       string
	DocsDir           string
	SshConfigPath     string
	DbPath            string
	LocalStateDBPath  string
	AuditVaultDBPath  string
	SuspendedTxDBPath string
}

// NewTestPaths creates a TestPaths instance with all paths calculated
// relative to the provided base directory. Does NOT mutate global state.
func NewTestPaths(baseDir string) *TestPaths {
	runtimeDir := filepath.Join(baseDir, constants.RuntimeDirname)
	dataDir := filepath.Join(runtimeDir, constants.DataDirname)
	pkiDir := filepath.Join(runtimeDir, constants.PkiDirname)
	secretsDir := filepath.Join(runtimeDir, constants.SecretsDirname)
	vaultDir := filepath.Join(runtimeDir, constants.VaultDirname)
	testVaultDir := filepath.Join(runtimeDir, constants.TestVaultDirname)
	protocolDir := filepath.Join(runtimeDir, constants.ProtocolDirname)
	docsDir := filepath.Join(runtimeDir, constants.DocsDirname)

	return &TestPaths{
		BaseDir:           baseDir,
		RuntimeDir:        runtimeDir,
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		VaultDir:          vaultDir,
		VaultKeyPath:      filepath.Join(vaultDir, constants.VaultKeyFilename),
		TestVaultDir:      testVaultDir,
		ProtocolDir:       protocolDir,
		DocsDir:           docsDir,
		SshConfigPath:     filepath.Join(runtimeDir, constants.SshConfigFilename),
		DbPath:            filepath.Join(dataDir, constants.DbFilename),
		LocalStateDBPath:  filepath.Join(runtimeDir, constants.LocalStateDBFilename),
		AuditVaultDBPath:  filepath.Join(dataDir, constants.AuditVaultDBFilename),
		SuspendedTxDBPath: filepath.Join(dataDir, constants.SuspendedTxFilename),
	}
}

// TempDir returns an absolute, test-owned directory in the system temporary
// directory. Keeping runtime files outside the source tree avoids triggering
// editor and source watchers. testing owns cleanup independently of CWD.
func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("failed to resolve absolute path for %s: %v", dir, err)
	}
	return absDir
}

// NewTestPathsFromTemp creates a TestPaths instance using TempDir(t) as the base.
// This is the recommended way to get isolated test paths.
func NewTestPathsFromTemp(t *testing.T) *TestPaths {
	t.Helper()
	return NewTestPaths(TempDir(t))
}

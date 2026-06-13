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

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestPaths holds test-specific filesystem paths.
// This allows tests to have isolated path structures without mutating
// the global constants.Paths package-level state, which causes race
// conditions in parallel test execution.
type TestPaths struct {
	BaseDir          string
	RuntimeDir       string
	DataDir          string
	PKIDir           string
	SecretsDir       string
	VaultDir         string
	VaultKeyPath     string
	TestVaultDir     string
	ProtocolDir      string
	DocsDir          string
	SshConfigPath    string
	DbPath           string
	LocalStateDBPath string
	AuditVaultDBPath string
	SuspendedTxDBPath string
}

// NewTestPaths creates a TestPaths instance with all paths calculated
// relative to the provided base directory. Does NOT mutate global state.
func NewTestPaths(baseDir string) *TestPaths {
	runtimeDir := filepath.Join(baseDir, constants.RuntimeDirname)
	dataDir := filepath.Join(runtimeDir, "data")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	vaultDir := filepath.Join(runtimeDir, "vault")
	testVaultDir := filepath.Join(runtimeDir, "test-vault")
	protocolDir := filepath.Join(runtimeDir, "protocol")
	docsDir := filepath.Join(runtimeDir, "docs")

	return &TestPaths{
		BaseDir:          baseDir,
		RuntimeDir:       runtimeDir,
		DataDir:          dataDir,
		PKIDir:           pkiDir,
		SecretsDir:       secretsDir,
		VaultDir:         vaultDir,
		VaultKeyPath:     filepath.Join(vaultDir, "key"),
		TestVaultDir:     testVaultDir,
		ProtocolDir:      protocolDir,
		DocsDir:          docsDir,
		SshConfigPath:    filepath.Join(runtimeDir, constants.SshConfigFilename),
		DbPath:           filepath.Join(dataDir, "g8e.db"),
		LocalStateDBPath: filepath.Join(runtimeDir, "local_state.db"),
		AuditVaultDBPath: filepath.Join(dataDir, "audit_vault.db"),
		SuspendedTxDBPath: filepath.Join(dataDir, "suspended_transactions.db"),
	}
}

// NewTestPathsFromTemp creates a TestPaths instance using t.TempDir() as the base.
// This is the recommended way to get isolated test paths.
func NewTestPathsFromTemp(t *testing.T) *TestPaths {
	t.Helper()
	return NewTestPaths(t.TempDir())
}

// EnsureDirs creates all directories required by the TestPaths.
// Returns an error if any directory creation fails.
func (tp *TestPaths) EnsureDirs() error {
	dirs := []string{
		tp.RuntimeDir,
		tp.DataDir,
		tp.PKIDir,
		tp.SecretsDir,
		tp.VaultDir,
		tp.TestVaultDir,
		tp.ProtocolDir,
		tp.DocsDir,
		filepath.Join(tp.PKIDir, "root"),
		filepath.Join(tp.PKIDir, "authorities"),
		filepath.Join(tp.PKIDir, "issued"),
		filepath.Join(tp.PKIDir, "trust"),
		filepath.Join(tp.PKIDir, "revocation"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("testpaths: failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// EnsureDirsWithPerms creates all directories with specified permissions.
// Useful for directories that need stricter permissions (e.g., secrets).
func (tp *TestPaths) EnsureDirsWithPerms(perms map[string]os.FileMode) error {
	dirs := []string{
		tp.RuntimeDir,
		tp.DataDir,
		tp.PKIDir,
		tp.SecretsDir,
		tp.VaultDir,
		tp.TestVaultDir,
		tp.ProtocolDir,
		tp.DocsDir,
		filepath.Join(tp.PKIDir, "root"),
		filepath.Join(tp.PKIDir, "authorities"),
		filepath.Join(tp.PKIDir, "issued"),
		filepath.Join(tp.PKIDir, "trust"),
		filepath.Join(tp.PKIDir, "revocation"),
	}

	for _, dir := range dirs {
		perm := os.FileMode(0755)
		if p, ok := perms[dir]; ok {
			perm = p
		}
		if err := os.MkdirAll(dir, perm); err != nil {
			return fmt.Errorf("testpaths: failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Cleanup removes all directories created by EnsureDirs.
// This is typically called via t.Cleanup().
func (tp *TestPaths) Cleanup() error {
	if tp.BaseDir == "" {
		return nil
	}
	return os.RemoveAll(tp.BaseDir)
}

// RegisterCleanup registers a cleanup function with the test to remove
// the test directories when the test completes.
func (tp *TestPaths) RegisterCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := tp.Cleanup(); err != nil {
			t.Logf("TestPaths cleanup failed: %v", err)
		}
	})
}

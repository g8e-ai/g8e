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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
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

// TempDir creates a unique temp directory under CWD (./.g8e-test-tmp/) and
// registers a t.Cleanup to remove it. This replaces t.TempDir() to keep all
// test artifacts relative to the project root instead of the system TEMP dir.
func TempDir(t *testing.T) string {
	t.Helper()
	base := filepath.Join(constants.PathCurrentDir, constants.TestTempDirname)
	if err := os.MkdirAll(base, constants.PermDirStandard); err != nil {
		t.Fatalf("failed to create test temp base dir %s: %v", base, err)
	}
	safeName := strings.NewReplacer("/", "_", "\\", "_").Replace(t.Name())
	dir, err := os.MkdirTemp(base, safeName+"-*")
	if err != nil {
		t.Fatalf("failed to create test temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("TempDir cleanup: failed to remove %s: %v", dir, err)
		}
	})
	return dir
}

// NewTestPathsFromTemp creates a TestPaths instance using TempDir(t) as the base.
// This is the recommended way to get isolated test paths.
func NewTestPathsFromTemp(t *testing.T) *TestPaths {
	t.Helper()
	return NewTestPaths(TempDir(t))
}

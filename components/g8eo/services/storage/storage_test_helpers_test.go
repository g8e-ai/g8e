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
	"os"
	"os/exec"
	"testing"

	vault "github.com/g8e-ai/g8e/components/g8eo/services/vault"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/require"
)

// testGitPath returns the system git binary path, skipping the test if git is unavailable.
func testGitPath(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available — skipping git-dependent test")
	}
	return gitPath
}

// newTestVault creates a new unlocked Vault in the given directory using the provided API key.
// The vault header is initialized and the vault is unlocked. Cleanup closes it via t.Cleanup.
func newTestVault(t *testing.T, dataDir string, apiKey string) *vault.Vault {
	t.Helper()

	require.NoError(t, os.MkdirAll(dataDir, 0700))

	logger := testutil.NewTestLogger()

	header, _, err := vault.NewVaultHeader(apiKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(dataDir))

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dataDir,
		Logger:  logger,
	})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(apiKey))

	t.Cleanup(func() { v.Close() })
	return v
}

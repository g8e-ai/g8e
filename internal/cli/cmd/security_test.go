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

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityCmd(t *testing.T) {
	t.Run("security command has correct use and description", func(t *testing.T) {
		cmd := securityCmd()
		assert.Equal(t, "security", cmd.Use)
		assert.Contains(t, cmd.Short, "Security validation")
		assert.Contains(t, cmd.Long, "PKI verification")
	})
}

func TestSecurityValidateCmd(t *testing.T) {
	t.Run("validate command has correct use", func(t *testing.T) {
		cmd := securityValidateCmd()
		assert.Equal(t, "validate", cmd.Use)
		assert.Contains(t, cmd.Short, "Run security validation")
	})

	t.Run("validate has pki-dir flag", func(t *testing.T) {
		cmd := securityValidateCmd()
		flag := cmd.Flags().Lookup("pki-dir")
		assert.NotNil(t, flag)
	})

	t.Run("validate has secrets-dir flag", func(t *testing.T) {
		cmd := securityValidateCmd()
		flag := cmd.Flags().Lookup("secrets-dir")
		assert.NotNil(t, flag)
	})
}

func TestSecurityValidateWithTestEnvironment(t *testing.T) {
	t.Run("validate checks PKI directory structure", func(t *testing.T) {
		tmpDir := t.TempDir()
		pkiDir := filepath.Join(tmpDir, "pki")
		secretsDir := filepath.Join(tmpDir, "secrets")

		// Create required PKI structure
		require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
		require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))

		// Create dummy files
		require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), []byte("dummy cert"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8e-gw-ca-bundle.pem"), []byte("dummy bundle"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

		// Create secrets
		require.NoError(t, os.MkdirAll(secretsDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

		cmd := securityValidateCmd()
		cmd.Flags().Set("pki-dir", pkiDir)
		cmd.Flags().Set("secrets-dir", secretsDir)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		// Will fail on certificate validation (dummy certs), but structure checks should pass
		assert.Error(t, err)
	})

	t.Run("validate fails with missing PKI directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		pkiDir := filepath.Join(tmpDir, "pki")
		secretsDir := filepath.Join(tmpDir, "secrets")

		// Don't create PKI directory
		require.NoError(t, os.MkdirAll(secretsDir, 0700))

		cmd := securityValidateCmd()
		cmd.Flags().Set("pki-dir", pkiDir)
		cmd.Flags().Set("secrets-dir", secretsDir)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		// Should fail due to missing PKI structure
		assert.Error(t, err)
	})

	t.Run("validate fails with missing secrets directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		pkiDir := filepath.Join(tmpDir, "pki")
		secretsDir := filepath.Join(tmpDir, "secrets")

		// Create PKI but not secrets
		require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))

		cmd := securityValidateCmd()
		cmd.Flags().Set("pki-dir", pkiDir)
		cmd.Flags().Set("secrets-dir", secretsDir)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		// Should fail due to missing secrets
		assert.Error(t, err)
	})
}

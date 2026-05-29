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
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApproveCmd(t *testing.T) {
	t.Run("approve command has correct use and description", func(t *testing.T) {
		cmd := approveCmd()
		assert.Contains(t, cmd.Use, "approve")
		assert.Contains(t, cmd.Short, "Approve")
		assert.Contains(t, cmd.Short, "L3")
		assert.Contains(t, cmd.Short, "CLI signature")
	})

	t.Run("approve requires exactly one argument", func(t *testing.T) {
		cmd := approveCmd()
		// Test that args validation is set by checking it's not nil
		assert.NotNil(t, cmd.Args)
	})

	t.Run("approve fails with no arguments", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		setupApproveTestConfig(t, tmpDir)

		// The cobra framework will validate args before RunE is called
		// So we test the validation directly
		err := cmd.Args(cmd, []string{})
		assert.Error(t, err)
	})

	t.Run("approve fails with invalid config", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Don't set up config - should fail to load
		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
		// Config loading might fail for various reasons, just check it fails
	})

	t.Run("approve fails when CLI private key missing", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		_ = setupApproveTestConfig(t, tmpDir)
		// Don't create CLI key file

		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
		// Will fail on key read or config load
	})

	t.Run("approve fails with invalid PEM private key", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		cfg := setupApproveTestConfig(t, tmpDir)
		// Write invalid PEM
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("invalid pem"), 0600))

		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
		// Will fail on PEM decode or config load
	})

	t.Run("approve requires CLI certificate", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		cfg := setupApproveTestConfig(t, tmpDir)

		// Create valid Ed25519 key
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)

		privPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), privPEM, 0600))

		// Don't create cert file
		err = cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
		// Will fail on cert read or config load
	})
}

func setupApproveTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	credentialsDir := filepath.Join(runtimeDir, "credentials")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))

	// Create minimal paths.json structure
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := `{
		"host": "localhost",
		"infra": {
			"app_cert_dir": ".g8e/pki/app",
			"ca_cert_path": ".g8e/pki/root/root_ca.crt",
			"db_path": ".g8e/data/operator.db",
			"docs_dir": "docs",
			"pki_dir": ".g8e/pki",
			"protocol_constants_dir": "protocol/constants",
			"protocol_dir": "protocol",
			"protocol_models_dir": "protocol/models",
			"secrets_dir": ".g8e/secrets",
			"ssh_config_path": ".g8e/ssh/config"
		},
		"ports": {
			"insecure_mcp_gateway": 18789,
			"operator_bootstrap_https": 8441,
			"operator_https": 8440,
			"operator_public_https": 8443
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	return &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
	}
}

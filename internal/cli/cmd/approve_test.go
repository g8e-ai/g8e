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
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApproveCmdStructure(t *testing.T) {
	t.Run("approve command has correct use and description", func(t *testing.T) {
		cmd := approveCmd()
		assert.Contains(t, cmd.Use, "approve")
		assert.Contains(t, cmd.Short, "Approve")
		assert.Contains(t, cmd.Long, "suspended")
	})

	t.Run("approve command requires exactly one argument", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.Error(t, err)
	})
}

func TestApproveCmdWithConfig(t *testing.T) {
	t.Run("approve fails when config load fails", func(t *testing.T) {
		failLoader := func(string) (*config.Config, error) {
			return nil, errors.New("config load error")
		}
		cmd := approveCmdWithConfig(failLoader, defaultAPIClientFactory)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
	})

	t.Run("approve fails when CLI key file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		loader := func(string) (*config.Config, error) {
			return cfg, nil
		}
		cmd := approveCmdWithConfig(loader, defaultAPIClientFactory)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
	})

	t.Run("approve fails with invalid PEM key", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("not a pem"), 0o600))

		loader := func(string) (*config.Config, error) {
			return cfg, nil
		}
		cmd := approveCmdWithConfig(loader, defaultAPIClientFactory)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
	})

	t.Run("approve fails with extra data after PEM", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		keyData := append([]byte("-----BEGIN PRIVATE KEY-----\naGVsbG8=\n-----END PRIVATE KEY-----\n"), []byte("extra data")...)
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), keyData, 0o600))

		loader := func(string) (*config.Config, error) {
			return cfg, nil
		}
		cmd := approveCmdWithConfig(loader, defaultAPIClientFactory)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
	})
}

func TestApproveCmdWithValidKeyFile(t *testing.T) {
	t.Run("approve fails when cert file does not exist after key is read", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		_, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keyDER, err := x509.MarshalPKCS8PrivateKey(privKey)
		require.NoError(t, err)
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), keyPEM, 0o600))

		loader := func(string) (*config.Config, error) {
			return cfg, nil
		}
		cmd := approveCmdWithConfig(loader, defaultAPIClientFactory)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err = cmd.RunE(cmd, []string{"abc123"})
		assert.Error(t, err)
	})
}

func TestEnrollCmdWithConfigErrorPaths(t *testing.T) {
	t.Run("enroll fails when config load fails", func(t *testing.T) {
		failLoader := func(string) (*config.Config, error) {
			return nil, errors.New("config load error")
		}
		cmd := enrollCmdWithConfig(failLoader)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
	})

	t.Run("enroll fails when operator not running", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		config.SetEndpointOverride("127.0.0.1:65535")
		t.Cleanup(func() { config.SetEndpointOverride("") })

		loader := func(string) (*config.Config, error) {
			return cfg, nil
		}
		cmd := enrollCmdWithConfig(loader)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
	})
}

func TestEnrollCmdStructure(t *testing.T) {
	t.Run("enroll command has correct use and description", func(t *testing.T) {
		cmd := enrollCmdWithConfig(func(string) (*config.Config, error) { return nil, nil })
		assert.Equal(t, "enroll", cmd.Use)
		assert.Contains(t, cmd.Short, "Enroll")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("enroll has tpm flag on Windows only", func(t *testing.T) {
		cmd := enrollCmdWithConfig(func(string) (*config.Config, error) { return nil, nil })
		flag := cmd.Flags().Lookup("tpm")
		if runtime.GOOS == "windows" {
			assert.NotNil(t, flag)
			assert.Equal(t, "false", flag.DefValue)
		} else {
			assert.Nil(t, flag)
		}
	})
}

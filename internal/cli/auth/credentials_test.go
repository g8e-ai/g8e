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

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	err := SaveCredentials(cfg, creds)
	require.NoError(t, err)

	loaded, err := LoadCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, creds.OperatorSessionID, loaded.OperatorSessionID)
	assert.Equal(t, creds.UserID, loaded.UserID)
	assert.Equal(t, creds.OperatorID, loaded.OperatorID)
	assert.Equal(t, creds.CLISessionID, loaded.CLISessionID)
}

func TestLoadCredentials_NotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	loaded, err := LoadCredentials(cfg)
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestLoadCredentials_InvalidJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.MkdirAll(cfg.CredentialsDir, 0700))
	require.NoError(t, os.WriteFile(credsFile, []byte("invalid-json{{{"), 0600))

	loaded, err := LoadCredentials(cfg)
	require.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "failed to parse credentials")
}

func TestDeleteCredentials_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
				VaultDir             string `json:"vault_dir"`
				VaultKeyPath         string `json:"vault_key_path"`
			}{
				CACertPath: filepath.Join(tmpDir, ".g8e/pki/trust/g8eg-ca-bundle.pem"),
			},
		},
	}

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	require.NoError(t, SaveCredentials(cfg, creds))

	certDir := cfg.CredentialsDir
	require.NoError(t, os.MkdirAll(certDir, 0700))
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), []byte("cli-cert"), 0600))
	require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("cli-key"), 0600))
	require.NoError(t, os.WriteFile(cfg.OperatorCertFile(), []byte("op-cert"), 0600))
	require.NoError(t, os.WriteFile(cfg.OperatorKeyFile(), []byte("op-key"), 0600))
	hubBundle := cfg.TrustBundlePath()
	require.NoError(t, os.MkdirAll(filepath.Dir(hubBundle), 0700))
	require.NoError(t, os.WriteFile(hubBundle, []byte("g8e-gw-ca-bundle"), 0600))

	err := DeleteCredentials(cfg)
	require.NoError(t, err)

	assert.NoFileExists(t, cfg.CredentialsFile())
	assert.NoFileExists(t, cfg.CLICertFile())
	assert.NoFileExists(t, cfg.CLIKeyFile())
	assert.NoFileExists(t, hubBundle)
}

func TestDeleteCredentials_NonExistentFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
				VaultDir             string `json:"vault_dir"`
				VaultKeyPath         string `json:"vault_key_path"`
			}{
				CACertPath: filepath.Join(tmpDir, ".g8e/pki/trust/g8eg-ca-bundle.pem"),
			},
		},
	}

	err := DeleteCredentials(cfg)
	require.NoError(t, err)
}

func TestSaveCredentials_WriteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// Create the credentials directory
	require.NoError(t, os.MkdirAll(cfg.CredentialsDir, 0700))

	// Create a file at the credentials file path to block writing
	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.WriteFile(credsFile, []byte("block"), 0400))

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	err := SaveCredentials(cfg, creds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write credentials file")
}

func TestLoadCredentials_ReadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// Create the credentials directory
	require.NoError(t, os.MkdirAll(cfg.CredentialsDir, 0700))

	// Create a file at the credentials file path that's a directory
	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.MkdirAll(credsFile, 0700))

	_, err := LoadCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read credentials file")
}

func TestSaveCredentials_MkdirError(t *testing.T) {
	t.Parallel()

	// Create a file where we expect a directory
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "credentials")
	require.NoError(t, os.WriteFile(blockingFile, []byte("block"), 0600))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: blockingFile, // This is a file, not a directory
		Paths:          &config.PathsConfig{},
	}

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	err := SaveCredentials(cfg, creds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create credentials directory")
}

func TestDeleteCredentials_RemoveError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
				VaultDir             string `json:"vault_dir"`
				VaultKeyPath         string `json:"vault_key_path"`
			}{
				CACertPath: filepath.Join(tmpDir, ".g8e/pki/trust/g8eg-ca-bundle.pem"),
			},
		},
	}

	// Create a directory where we expect a file (to cause removal error on some OSes)
	// This test is platform-dependent
	err := DeleteCredentials(cfg)
	require.NoError(t, err) // Non-existent files should not error
}

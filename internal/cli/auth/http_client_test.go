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
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecureHTTPClient_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	trustBundlePath := filepath.Join(tmpDir, "trust-bundle.pem")

	// Generate a test CA certificate
	caPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	require.NoError(t, os.WriteFile(trustBundlePath, []byte(caPEM), 0600))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = trustBundlePath

	client, err := NewSecureHTTPClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify TLS config is set correctly
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
}

func TestNewSecureHTTPClient_MissingTrustBundlePath(t *testing.T) {
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

	client, err := NewSecureHTTPClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "trust bundle path not configured")
}

func TestNewSecureHTTPClient_InvalidPEM(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	trustBundlePath := filepath.Join(tmpDir, "trust-bundle.pem")

	require.NoError(t, os.WriteFile(trustBundlePath, []byte("invalid-pem-data"), 0600))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = trustBundlePath

	client, err := NewSecureHTTPClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "failed to parse CA certificates")
}

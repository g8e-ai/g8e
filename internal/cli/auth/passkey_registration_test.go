//go:build integration

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
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyPasskeyRegistration_NetworkError(t *testing.T) {

	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.RuntimeDirname),
		PKIDir:         filepath.Join(tmpDir, constants.RuntimeDirname, constants.PkiDirname),
		SecretsDir:     filepath.Join(tmpDir, constants.RuntimeDirname, constants.SecretsDirname),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// VerifyPasskeyRegistration now uses mTLS: supply CLI cert and a CA bundle
	// so the test reaches the network dial (and fails there, as expected).
	writeTestCLICert(t, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath, err := filepath.Abs(filepath.Join(tmpDir, "test-ca.pem"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath

	hasPasskey, err := VerifyPasskeyRegistration(cfg, "test-user", "test-cli-session")

	require.Error(t, err)
	assert.False(t, hasPasskey)
}

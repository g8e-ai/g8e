//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyPasskeyRegistration_NetworkError(t *testing.T) {

	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{
		ProjectRoot: tmpDir,
		RuntimeDir:  filepath.Join(tmpDir, constants.RuntimeDirname),
		PKIDir:      filepath.Join(tmpDir, constants.RuntimeDirname, constants.PkiDirname),
		SecretsDir:  filepath.Join(tmpDir, constants.RuntimeDirname, constants.SecretsDirname),
		Paths:       &config.PathsConfig{},
	}

	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	// VerifyPasskeyRegistration now uses mTLS: supply CLI cert and a CA bundle
	// so the test reaches the network dial (and fails there, as expected).
	writeTestCLICert(t, fileSvc, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath := filepath.Join(tmpDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-cli-session")

	require.Error(t, err)
	assert.False(t, hasPasskey)
}

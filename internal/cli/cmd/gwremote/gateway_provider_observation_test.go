//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(wd)
		require.NotEqual(t, parent, wd)
		wd = parent
	}
	original, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(wd))
	t.Cleanup(func() { _ = os.Chdir(original) })
}

func TestProviderObservationGatewayRead_LiveNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live gateway integration in short mode")
	}
	chdirRepoRoot(t)
	if !IsGatewayHealthy() {
		t.Skip("gateway not healthy on localhost:8080")
	}

	cfg, err := config.Load("")
	require.NoError(t, err)
	fileSvc, err := fs.NewRuntimeFileService("", testutil.NewTestLogger())
	require.NoError(t, err)

	remote, err := NewProviderObservationRemote(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, remote)

	_, _, err = remote.Load(t.Context(), "missing-attempt-integration-test")
	require.Error(t, err)
	if strings.Contains(err.Error(), "certificate signed by unknown authority") {
		t.Skip("skipping live gateway test: running gateway TLS certificate not trusted by local PKI")
	}
	require.ErrorIs(t, err, constants.ErrHTTPStatusError)
	require.Contains(t, err.Error(), "status 404")
}

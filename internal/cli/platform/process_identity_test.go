// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestWriteNetworkIdentityFile_ErrorOnInvalidRuntimeDir(t *testing.T) {
	t.Parallel()

	dir := testutil.TempDir(t)

	// Create a file where the .g8e/ runtime directory would be expected,
	// causing WriteFile to fail because the path is not a directory.
	runtimeBlockingFile := filepath.Join(dir, constants.RuntimeDirname)
	require.NoError(t, os.WriteFile(runtimeBlockingFile, []byte("blocking"), constants.PermFilePrivate))

	fileSvc := newPlatformTestFileSvc(t, dir)
	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	_, err = pm.WriteNetworkIdentityFile([]byte(`{"IPs":[]}`))
	require.Error(t, err)
}

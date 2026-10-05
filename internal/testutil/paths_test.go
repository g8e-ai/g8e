// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestTempDir_CleanupSurvivesWorkingDirectoryChange(t *testing.T) {
	original, err := os.Getwd()
	require.NoError(t, err)
	discoveryDir := t.TempDir()
	var root string
	t.Run("directory discovery changes after allocation", func(t *testing.T) {
		// Exercise TempDir's working-directory discovery and cleanup ownership.
		// The directory change is the behavior under test, not runtime alignment.
		t.Chdir(discoveryDir)
		root = TempDir(t)
		require.True(t, filepath.IsAbs(root))
		require.NoError(t, os.WriteFile(filepath.Join(root, "owned.txt"), []byte("fixture"), constants.PermFilePrivate))
		require.NoError(t, os.Chdir(original))
	})
	_, err = os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist, "the owning test must remove its root even after directory discovery changes")
}

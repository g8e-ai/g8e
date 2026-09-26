// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// ChdirTemp creates a temp dir, chdirs into it, and restores the original
// working directory on cleanup.
//
// WARNING: This function mutates process-global state via os.Chdir.
// Tests using ChdirTemp must NOT call t.Parallel(), and no other test in
// the same process should chdir concurrently. The functions under test rely
// on os.Getwd(), so chdir is unavoidable.
func ChdirTemp(t *testing.T) string {
	t.Helper()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := testutil.TempDir(t)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { os.Chdir(originalWd) })
	return tmpDir
}

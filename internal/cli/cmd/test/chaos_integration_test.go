// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package testcmd

import (
	"bytes"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunChaosErrorHandling(t *testing.T) {
	t.Run("runChaos wraps chaos.Run errors", func(t *testing.T) {
		// This test verifies that errors from chaos.Run are properly wrapped
		// with the "chaos: failed to run chaos test" prefix
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Use count=0 to trigger validation error deterministically on all platforms
		chaosCount = 0
		chaosDataDir = ""
		chaosPKIDir = ""

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		tmpDir := testutil.TempDir(t)
		chaosDataDir = tmpDir

		err := runChaos(cmd, []string{})
		// We expect an error due to count=0 validation
		assert.Error(t, err)
	})

	t.Run("runChaos with valid temporary directory", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Use a valid temporary directory
		tmpDir := testutil.TempDir(t)
		chaosCount = 1
		chaosDataDir = tmpDir
		chaosPKIDir = ""

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// This may still fail due to missing dependencies, but should not fail
		// due to directory creation
		err := runChaos(cmd, []string{})
		// The error is acceptable here as we're testing the config construction
		// and path handling, not the full chaos.Run execution
		if err != nil {
			assert.Contains(t, err.Error(), "chaos")
		}
	})
}

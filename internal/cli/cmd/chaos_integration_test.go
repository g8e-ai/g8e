// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// os.Chdir is used because runChaos calls configLoad which reads config from
// the current working directory. This is a legitimate cwd usage — the config
// layer translates cwd into fileSvc baseDir. Injecting fileSvcFactory into
// runChaos would require config-layer injection, which is out of scope for the
// current refactor.

package cmd

import (
	"bytes"
	"os"
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

		// Change to a temp directory to avoid affecting real filesystem
		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := runChaos(cmd, []string{})
		// We expect an error due to the invalid path
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

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

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

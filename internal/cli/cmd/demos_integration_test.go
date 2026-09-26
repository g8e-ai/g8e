// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

// Demo commands discover ./demos/ from a root directory. These tests pass
// testutil.TempDir through cliSourceRoot. They do not call os.Chdir.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunDemosList(t *testing.T) {
	t.Run("returns error when demos directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		cmd := &cobra.Command{}
		err = runDemosList(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read directory")
	})

	t.Run("succeeds when demos directory exists", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		// Create a demo directory with compose.yml
		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		err = runDemosList(cmd, []string{})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Available demo environments:")
		assert.Contains(t, output, "healthcare")
	})

	t.Run("lists directories that contain compose.yml", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		// A directory named bin is listed when it has compose.yml. Listing is
		// based on the compose file, not the directory name.
		binDir := filepath.Join(demosDir, constants.BinDirname)
		err = os.Mkdir(binDir, 0755)
		require.NoError(t, err)
		binCompose := filepath.Join(binDir, constants.DemosComposeFile)
		err = os.WriteFile(binCompose, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		// Create a valid demo directory
		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		err = runDemosList(cmd, []string{})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "healthcare")
		assert.Contains(t, output, "bin")
	})

	t.Run("only lists directories with compose.yml", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		// Create a demo directory without compose.yml (should not be listed)
		noComposeDir := filepath.Join(demosDir, "no-compose")
		err = os.Mkdir(noComposeDir, 0755)
		require.NoError(t, err)

		// Create a valid demo directory
		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		err = runDemosList(cmd, []string{})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "healthcare")
		assert.NotContains(t, output, "no-compose")
	})
}

func TestRunDemosStart(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStart(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent'")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStart(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml in demo directory 'healthcare'")
	})
}

func TestRunDemosStop(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStop(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent'")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStop(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml in demo directory 'healthcare'")
	})
}

func TestRunDemosStatus(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStatus(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent'")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStatus(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml in demo directory 'healthcare'")
	})
}

func TestRunDemosClean(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"nonexistent"}, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent'")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"healthcare"}, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml in demo directory 'healthcare'")
	})
}

func TestRunDemosRun(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"nonexistent"}, false, fileSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent'")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare"}, false, fileSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml in demo directory 'healthcare'")
	})

	t.Run("calls runScenario when scenario argument provided", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare", "1"}, false, fileSvc)
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})

	t.Run("calls runAllScenarios when no scenario argument", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		useCLISourceRoot(t, tmpDir)
		var err error
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare"}, false, fileSvc)
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})
}

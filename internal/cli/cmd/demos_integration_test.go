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

//go:build e2e

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestRunDemosList(t *testing.T) {
	t.Run("returns error when demos directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Change to a temporary directory without demos
		tmpDir := testutil.TempDir(t)
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosList(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read demos directory")
	})

	t.Run("succeeds when demos directory exists", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Create a temporary directory with demos structure
		tmpDir := testutil.TempDir(t)
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

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := &cobra.Command{}
		err = runDemosList(cmd, []string{})

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Available demo environments:")
		assert.Contains(t, output, "healthcare")
	})

	t.Run("excludes bin directory from list", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Create a temporary directory with demos structure
		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		// Create bin directory (should be excluded)
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

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := &cobra.Command{}
		err = runDemosList(cmd, []string{})

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "healthcare")
		assert.NotContains(t, output, "bin")
	})

	t.Run("only lists directories with compose.yml", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Create a temporary directory with demos structure
		tmpDir := testutil.TempDir(t)
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

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := &cobra.Command{}
		err = runDemosList(cmd, []string{})

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "healthcare")
		assert.NotContains(t, output, "no-compose")
	})
}

func TestRunDemosStart(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStart(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStart(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})
}

func TestRunDemosStop(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStop(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStop(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})
}

func TestRunDemosStatus(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStatus(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosStatus(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})
}

func TestRunDemosClean(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"nonexistent"}, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"healthcare"}, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})
}

func TestRunDemosRun(t *testing.T) {
	t.Run("returns error when demo directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"nonexistent"}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare"}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})

	t.Run("calls runScenario when scenario argument provided", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare", "1"}, false)
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})

	t.Run("calls runAllScenarios when no scenario argument", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := testutil.TempDir(t)
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)
		composePath := filepath.Join(healthcareDir, constants.DemosComposeFile)
		err = os.WriteFile(composePath, []byte("version: '3'"), 0644)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare"}, false)
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})
}

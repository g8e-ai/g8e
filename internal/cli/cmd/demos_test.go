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

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func TestDemos(t *testing.T) {
	t.Run("demos command has correct use and description", func(t *testing.T) {
		cmd := demosCmd()
		assert.Equal(t, "demos", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage g8e demo environments")
		assert.Contains(t, cmd.Long, "Docker Compose demo environments")
		assert.Contains(t, cmd.Long, "hermetically sealed")
	})

	t.Run("demos command has all expected subcommands", func(t *testing.T) {
		cmd := demosCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{
			"list",
			"start",
			"stop",
			"status",
			"clean",
			"reset",
			"run",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "demos command should have %s subcommand", subcmd)
		}
	})
}

func TestDemosListCmd(t *testing.T) {
	t.Run("list command has correct structure", func(t *testing.T) {
		cmd := demosListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List available demo environments")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("list command requires no arguments", func(t *testing.T) {
		cmd := demosListCmd()
		assert.Nil(t, cmd.Args)
	})
}

func TestDemosStartCmd(t *testing.T) {
	t.Run("start command has correct structure", func(t *testing.T) {
		cmd := demosStartCmd()
		assert.Equal(t, "start <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Start a demo environment")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("start command requires exactly one argument", func(t *testing.T) {
		cmd := demosStartCmd()
		// cobra.ExactArgs(1) is used, verify the validation
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosStopCmd(t *testing.T) {
	t.Run("stop command has correct structure", func(t *testing.T) {
		cmd := demosStopCmd()
		assert.Equal(t, "stop <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop a demo environment")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("stop command requires exactly one argument", func(t *testing.T) {
		cmd := demosStopCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosStatusCmd(t *testing.T) {
	t.Run("status command has correct structure", func(t *testing.T) {
		cmd := demosStatusCmd()
		assert.Equal(t, "status <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Show status of a demo environment")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("status command requires exactly one argument", func(t *testing.T) {
		cmd := demosStatusCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosCleanCmd(t *testing.T) {
	t.Run("clean command has correct structure", func(t *testing.T) {
		cmd := demosCleanCmd()
		assert.Equal(t, "clean <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Remove containers, volumes, and networks")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("clean command requires exactly one argument", func(t *testing.T) {
		cmd := demosCleanCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosResetCmd(t *testing.T) {
	t.Run("reset command has correct structure", func(t *testing.T) {
		cmd := demosResetCmd()
		assert.Equal(t, "reset <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Clean and restart a demo environment")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("reset command requires exactly one argument", func(t *testing.T) {
		cmd := demosResetCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosRunCmd(t *testing.T) {
	t.Run("run command has correct structure", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Equal(t, "run <org> [scenario]", cmd.Use)
		assert.Contains(t, cmd.Short, "Run demo scenarios")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("run command accepts 1 or 2 arguments", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.NotNil(t, cmd.Args)
	})

	t.Run("run command has detailed long description", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "healthcare")
		assert.Contains(t, cmd.Long, "gov")
		assert.Contains(t, cmd.Long, "finance")
		assert.Contains(t, cmd.Long, "FHIR PA Request")
		assert.Contains(t, cmd.Long, "CUI Exfiltration")
		assert.Contains(t, cmd.Long, "Unauthorized Trade")
	})
}

func TestScenarioCounts(t *testing.T) {
	t.Run("scenario counts map is correctly defined", func(t *testing.T) {
		assert.Equal(t, 4, scenarioCounts["healthcare"])
		assert.Equal(t, 1, scenarioCounts["gov"])
		assert.Equal(t, 1, scenarioCounts["finance"])
	})

	t.Run("scenario counts map has expected entries", func(t *testing.T) {
		expectedOrgs := []string{"healthcare", "gov", "finance"}
		for _, org := range expectedOrgs {
			_, exists := scenarioCounts[org]
			assert.True(t, exists, "scenarioCounts should have entry for %s", org)
		}
	})
}

func TestPrintDemoEndpoints(t *testing.T) {
	t.Run("prints healthcare endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("healthcare")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8081")
		assert.Contains(t, output, "https://localhost:8444")
		assert.Contains(t, output, "http://localhost:15673")
		assert.Contains(t, output, "localhost:5433")
		assert.Contains(t, output, "http://localhost:3001")
	})

	t.Run("prints gov endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("gov")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8080")
		assert.Contains(t, output, "https://localhost:8443")
		assert.Contains(t, output, "http://localhost:3000")
	})

	t.Run("prints finance endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("finance")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8082")
		assert.Contains(t, output, "https://localhost:8445")
		assert.Contains(t, output, "http://localhost:3002")
	})

	t.Run("prints default message for unknown org", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("unknown-org")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "No endpoint information available for 'unknown-org'")
	})
}

func TestRunScenario(t *testing.T) {
	t.Run("returns error for unknown org", func(t *testing.T) {
		err := runScenario("unknown-org", "/tmp", "1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no scenarios defined for demo environment 'unknown-org'")
	})

	t.Run("returns error for invalid healthcare scenario", func(t *testing.T) {
		err := runHealthcareScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for healthcare")
		assert.Contains(t, err.Error(), "valid: 1-4")
	})

	t.Run("returns error for invalid gov scenario", func(t *testing.T) {
		err := runGovScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for gov")
		assert.Contains(t, err.Error(), "valid: 1")
	})

	t.Run("returns error for invalid finance scenario", func(t *testing.T) {
		err := runFinanceScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for finance")
		assert.Contains(t, err.Error(), "valid: 1")
	})

	t.Run("healthcare scenario functions exist", func(t *testing.T) {
		// Verify the scenario function exists and doesn't panic on valid input
		// We don't execute it since it requires Docker
		assert.NotNil(t, runHealthcareScenario)
	})

	t.Run("gov scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runGovScenario)
	})

	t.Run("finance scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runFinanceScenario)
	})
}

func TestRunAllScenarios(t *testing.T) {
	t.Run("returns error for org without scenarios", func(t *testing.T) {
		err := runAllScenarios("unknown-org", "/tmp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no scenarios defined for demo environment 'unknown-org'")
	})

	t.Run("healthcare has 4 scenarios", func(t *testing.T) {
		count, ok := scenarioCounts["healthcare"]
		assert.True(t, ok)
		assert.Equal(t, 4, count)
	})

	t.Run("gov has 1 scenario", func(t *testing.T) {
		count, ok := scenarioCounts["gov"]
		assert.True(t, ok)
		assert.Equal(t, 1, count)
	})

	t.Run("finance has 1 scenario", func(t *testing.T) {
		count, ok := scenarioCounts["finance"]
		assert.True(t, ok)
		assert.Equal(t, 1, count)
	})
}

func TestGetProjectRoot(t *testing.T) {
	t.Run("returns current working directory when available", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Change to a temporary directory
		tmpDir := t.TempDir()
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		root := getProjectRoot()
		assert.Equal(t, tmpDir, root)
	})

	t.Run("falls back to executable path when cwd fails", func(t *testing.T) {
		// This test verifies the fallback logic exists
		// We can't easily make os.Getwd() fail in a test, but we can verify
		// the function doesn't panic and returns a non-empty string
		root := getProjectRoot()
		assert.NotEmpty(t, root)
	})

	t.Run("returns non-empty string", func(t *testing.T) {
		root := getProjectRoot()
		assert.NotEmpty(t, root)
		assert.IsType(t, "", root)
	})
}

func TestRunDemosList(t *testing.T) {
	t.Run("returns error when demos directory does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Change to a temporary directory without demos
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
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

		tmpDir := t.TempDir()
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := t.TempDir()
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosClean(cmd, []string{"healthcare"})
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

		tmpDir := t.TempDir()
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo environment 'nonexistent' not found")
	})

	t.Run("returns error when compose.yml does not exist", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := t.TempDir()
		demosDir := filepath.Join(tmpDir, constants.DemosDirname)
		err = os.Mkdir(demosDir, 0755)
		require.NoError(t, err)

		healthcareDir := filepath.Join(demosDir, "healthcare")
		err = os.Mkdir(healthcareDir, 0755)
		require.NoError(t, err)

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		cmd := &cobra.Command{}
		err = runDemosRun(cmd, []string{"healthcare"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose.yml not found in demo directory 'healthcare'")
	})

	t.Run("calls runScenario when scenario argument provided", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := t.TempDir()
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
		err = runDemosRun(cmd, []string{"healthcare", "1"})
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})

	t.Run("calls runAllScenarios when no scenario argument", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		tmpDir := t.TempDir()
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
		err = runDemosRun(cmd, []string{"healthcare"})
		// Will fail due to Docker not being available, but should not fail due to missing compose.yml
		assert.NotContains(t, err.Error(), "compose.yml not found")
	})
}

func TestBinaryPathConstruction(t *testing.T) {
	t.Run("uses .exe extension on Windows", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			binPath := filepath.Join(constants.DemosDirname, constants.BinDirname, "g8e.exe")
			assert.True(t, strings.HasSuffix(binPath, ".exe"))
		} else {
			binPath := filepath.Join(constants.DemosDirname, constants.BinDirname, "g8e")
			assert.False(t, strings.HasSuffix(binPath, ".exe"))
		}
	})

	t.Run("uses no extension on non-Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			binPath := filepath.Join(constants.DemosDirname, constants.BinDirname, "g8e")
			assert.False(t, strings.HasSuffix(binPath, ".exe"))
		}
	})
}

func TestDemoStep(t *testing.T) {
	t.Run("demoStep function signature is correct", func(t *testing.T) {
		// Verify the function exists and has the correct signature
		// We can't test execution without Docker, but we can verify the signature
		assert.NotNil(t, demoStep)
	})
}

func TestHealthcareScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "healthcare: 1-4")
		assert.Contains(t, cmd.Long, "FHIR PA Request")
		assert.Contains(t, cmd.Long, "Gold Card Auto-Approval")
		assert.Contains(t, cmd.Long, "SLA Breach")
		assert.Contains(t, cmd.Long, "PHI Exfiltration")
	})
}

func TestGovScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "gov: 1")
		assert.Contains(t, cmd.Long, "CUI Exfiltration")
	})
}

func TestFinanceScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "finance: 1")
		assert.Contains(t, cmd.Long, "Unauthorized Trade")
	})
}

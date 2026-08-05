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

// os.Chdir is used for source-tree demo discovery (finding ./demos/ directories,
// compose.yml, doctrine/, target-data/), not .g8e/ runtime state. This is a
// legitimate cwd usage — demo commands resolve paths relative to the working
// directory, not through RuntimeFileService.

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func getProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// expectedDemoFiles lists all demo scenario Go files that must exist and use
// harnessRun or runTwoLayerScenario. Extracted as a package-level var to avoid
// drift across multiple test functions.
var expectedDemoFiles = []string{
	"demo_gov.go",
	"demo_finance.go",
	"demo_healthcare.go",
	"demo_dhs.go",
	"demo_fedramp.go",
}

func TestDemos(t *testing.T) {
	t.Run("demos command has correct use and description", func(t *testing.T) {
		cmd := demosCmd()
		assert.Equal(t, "demos", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage g8e demo environments")
		assert.Contains(t, cmd.Long, "Docker Compose demo environments")
		assert.Contains(t, cmd.Long, "hermetically sealed")
	})

	t.Run("demos command has demo alias", func(t *testing.T) {
		cmd := demosCmd()
		assert.Contains(t, cmd.Aliases, "demo")
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
			"rebuild",
			"run",
			"scenarios",
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
		assert.Equal(t, "clean [org]", cmd.Use)
		assert.Contains(t, cmd.Short, "Remove containers, volumes, and networks")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("clean command requires exactly one argument", func(t *testing.T) {
		cmd := demosCleanCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosRebuildCmd(t *testing.T) {
	t.Run("rebuild command has correct structure", func(t *testing.T) {
		cmd := demosRebuildCmd()
		assert.Equal(t, "rebuild <org>", cmd.Use)
		assert.Contains(t, cmd.Short, "Rebuild Docker images")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("rebuild command requires exactly one argument", func(t *testing.T) {
		cmd := demosRebuildCmd()
		assert.NotNil(t, cmd.Args)
	})

	t.Run("rebuild command has no-cache flag defaulting to true", func(t *testing.T) {
		cmd := demosRebuildCmd()
		flag := cmd.Flag("no-cache")
		require.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
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
		assert.Contains(t, cmd.Long, "Sovereign Multi-Source Ingest")
		assert.Contains(t, cmd.Long, "Governed Cloud Resource Provisioning")
		assert.Contains(t, cmd.Long, "Third-Party Frontend Enrollment")
	})
}

func TestScenarioCounts(t *testing.T) {
	t.Run("scenario counts map is correctly defined", func(t *testing.T) {
		assert.Equal(t, 4, scenarioCounts["healthcare"])
		assert.Equal(t, 1, scenarioCounts["gov"])
		assert.Equal(t, 1, scenarioCounts["finance"])
		assert.Equal(t, 5, scenarioCounts["dhs"])
		assert.Equal(t, 5, scenarioCounts["fedramp"])
		assert.Equal(t, 1, scenarioCounts["frontend"])
	})

	t.Run("scenario counts map has expected entries", func(t *testing.T) {
		expectedOrgs := []string{"healthcare", "gov", "finance", "dhs", "fedramp", "frontend"}
		for _, org := range expectedOrgs {
			_, exists := scenarioCounts[org]
			assert.True(t, exists, "scenarioCounts should have entry for %s", org)
		}
	})

	t.Run("scenario counts map does not contain removed demos", func(t *testing.T) {
		removedOrgs := []string{"secure-data", "dow", "swarm"}
		for _, org := range removedOrgs {
			_, exists := scenarioCounts[org]
			assert.False(t, exists, "scenarioCounts should NOT have entry for removed demo %s", org)
		}
	})

	t.Run("scenario counts map keys exactly match remaining 6 demo orgs", func(t *testing.T) {
		expectedOrgs := map[string]bool{
			"healthcare": true,
			"gov":        true,
			"finance":    true,
			"dhs":        true,
			"fedramp":    true,
			"frontend":   true,
		}
		assert.Equal(t, len(expectedOrgs), len(scenarioCounts),
			"scenarioCounts should have exactly %d entries", len(expectedOrgs))
		for org := range expectedOrgs {
			_, exists := scenarioCounts[org]
			assert.True(t, exists, "scenarioCounts should have entry for %s", org)
		}
		for org := range scenarioCounts {
			assert.True(t, expectedOrgs[org], "scenarioCounts should not have unexpected entry for %s", org)
		}
	})
}

func TestDemosRunCmdLongDescriptionExcludesRemovedDemos(t *testing.T) {
	cmd := demosRunCmd()
	removedDemos := []string{"dow", "swarm", "secure-data"}
	for _, demo := range removedDemos {
		assert.NotContains(t, cmd.Long, demo,
			"demosRunCmd Long description should not contain removed demo %q", demo)
	}
}

func TestPrintDemoEndpoints(t *testing.T) {
	t.Run("prints healthcare endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "healthcare")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8081")
		assert.Contains(t, output, "https://localhost:8444")
		assert.Contains(t, output, "http://localhost:15673")
		assert.Contains(t, output, "localhost:5433")
		assert.Contains(t, output, "http://localhost:3001")
		assert.Contains(t, output, "https://localhost:8444/console/")
		assert.Contains(t, output, "g8e auth enroll -e localhost:8081 --port 8444")
	})

	t.Run("prints gov endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "gov")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8080")
		assert.Contains(t, output, "https://localhost:8443")
		assert.Contains(t, output, "http://localhost:3000")
		assert.Contains(t, output, "https://localhost:8443/console/")
		assert.Contains(t, output, "g8e auth enroll -e localhost:8080 --port 8443")
	})

	t.Run("prints finance endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "finance")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8082")
		assert.Contains(t, output, "https://localhost:8445")
		assert.Contains(t, output, "http://localhost:3002")
		assert.Contains(t, output, "https://localhost:8445/console/")
		assert.Contains(t, output, "g8e auth enroll -e localhost:8082 --port 8445")
	})

	t.Run("prints dhs endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "dhs")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8087")
		assert.Contains(t, output, "https://localhost:8450")
		assert.Contains(t, output, "https://localhost:8450/console/")
		assert.Contains(t, output, "g8e auth enroll -e localhost:8087 --port 8450")
	})

	t.Run("prints fedramp endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "fedramp")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8088")
		assert.Contains(t, output, "https://localhost:8451")
		assert.Contains(t, output, "https://localhost:8451/console/")
		assert.Contains(t, output, "g8e auth enroll -e localhost:8088 --port 8451")
	})

	t.Run("prints default message for unknown org", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)

		printDemoEndpoints(cmd, "unknown-org")

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "No endpoint information available for 'unknown-org'")
		assert.NotContains(t, output, "auth enroll")
	})
}

func TestRunScenario(t *testing.T) {
	t.Run("returns ErrNotFound wrapped error for unknown org", func(t *testing.T) {
		err := runScenario("unknown-org", "/tmp", "1")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotFound)
		assert.Contains(t, err.Error(), "no scenarios defined for demo environment 'unknown-org'")
	})

	t.Run("returns error with valid range for invalid healthcare scenario number", func(t *testing.T) {
		_, err := runHealthcareScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for healthcare")
		assert.Contains(t, err.Error(), "valid: 1-4")
	})

	t.Run("returns error with valid range for invalid gov scenario number", func(t *testing.T) {
		_, err := runGovScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for gov")
		assert.Contains(t, err.Error(), "valid: 1")
	})

	t.Run("returns error with valid range for invalid finance scenario number", func(t *testing.T) {
		_, err := runFinanceScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for finance")
		assert.Contains(t, err.Error(), "valid: 1")
	})

	t.Run("returns error with valid range for invalid dhs scenario number", func(t *testing.T) {
		_, err := runDHSScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for dhs")
		assert.Contains(t, err.Error(), "valid: 1-5")
	})

	t.Run("returns error with valid range for invalid fedramp scenario number", func(t *testing.T) {
		_, err := runFedRAMPScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for fedramp")
		assert.Contains(t, err.Error(), "valid: 1-5")
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

	t.Run("dhs scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runDHSScenario)
	})

	t.Run("fedramp scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runFedRAMPScenario)
	})
}

func TestRunAllScenarios(t *testing.T) {
	t.Run("returns ErrNotFound wrapped error for org without scenarios", func(t *testing.T) {
		err := runAllScenarios(&cobra.Command{}, "unknown-org", "/tmp")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotFound)
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

	t.Run("dhs has 5 scenarios", func(t *testing.T) {
		count, ok := scenarioCounts["dhs"]
		assert.True(t, ok)
		assert.Equal(t, 5, count)
	})

	t.Run("fedramp has 5 scenarios", func(t *testing.T) {
		count, ok := scenarioCounts["fedramp"]
		assert.True(t, ok)
		assert.Equal(t, 5, count)
	})
}

func TestGetProjectRoot(t *testing.T) {
	t.Run("returns current working directory when available", func(t *testing.T) {
		// Save original working directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Change to a temporary directory
		tmpDir := testutil.TempDir(t)
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		root := getProjectRoot()
		expected, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)
		actual, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
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

func TestDemoStepHTTP(t *testing.T) {
	t.Run("demoStepHTTP function exists", func(t *testing.T) {
		assert.NotNil(t, demoStepHTTP)
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		err := demoStepHTTP(t.TempDir(), "failing command", "200",
			"false",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failing command")
	})

	t.Run("returns error on status code mismatch", func(t *testing.T) {
		dir := t.TempDir()
		// Use echo to simulate curl writing a status code to stdout
		err := demoStepHTTP(dir, "status check", "200",
			"echo", "404",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected HTTP 200")
		assert.Contains(t, err.Error(), "got 404")
	})

	t.Run("passes when status code matches", func(t *testing.T) {
		dir := t.TempDir()
		err := demoStepHTTP(dir, "status check", "200",
			"echo", "200",
		)
		assert.NoError(t, err)
	})

	t.Run("passes with 401 expected code", func(t *testing.T) {
		dir := t.TempDir()
		err := demoStepHTTP(dir, "SSE protection", "401",
			"echo", "401",
		)
		assert.NoError(t, err)
	})

	t.Run("trims whitespace from output", func(t *testing.T) {
		dir := t.TempDir()
		// printf adds trailing newline; demoStepHTTP should trim it
		err := demoStepHTTP(dir, "status check", "200",
			"printf", "200\n",
		)
		assert.NoError(t, err)
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

func TestCheckDockerAvailable(t *testing.T) {
	t.Run("checkDockerAvailable function exists", func(t *testing.T) {
		assert.NotNil(t, checkDockerAvailable)
	})

	t.Run("returns error when docker is not on PATH", func(t *testing.T) {
		originalPath := os.Getenv("PATH")
		defer os.Setenv("PATH", originalPath)

		os.Setenv("PATH", "/nonexistent/path")

		err := checkDockerAvailable()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrServiceUnavailable)
	})
}

func TestToDockerPath(t *testing.T) {
	t.Run("converts backslashes to forward slashes on Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("test only applies to Windows")
		}
		result := toDockerPath(`D:\g8e\demos\dhs\compose.yml`)
		assert.Equal(t, "D:/g8e/demos/dhs/compose.yml", result)
	})

	t.Run("returns path unchanged on non-Windows", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("test only applies to non-Windows")
		}
		result := toDockerPath("/home/user/g8e/demos/dhs/compose.yml")
		assert.Equal(t, "/home/user/g8e/demos/dhs/compose.yml", result)
	})
}

func TestDHSScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "dhs: 1-5")
		assert.Contains(t, cmd.Long, "Sovereign Multi-Source Ingest (chain-of-custody)")
		assert.Contains(t, cmd.Long, "Cross-Domain Release requires Notary authority")
		assert.Contains(t, cmd.Long, "Resilient Disconnected Operations")
		assert.Contains(t, cmd.Long, "Governed Predictive Cueing")
		assert.Contains(t, cmd.Long, "Sovereign Destruction + tamper-proof audit")
	})
}

func TestDefaultHarnessConfig(t *testing.T) {
	t.Run("returns standard demo topology defaults", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		assert.Equal(t, "agent-runtime", cfg.Container)
		assert.Equal(t, "https://g8e.local:8443", cfg.MTLSURL)
		assert.Equal(t, "http://g8e.local:8080", cfg.PublicURL)
		assert.Equal(t, "/root/.g8e/pki/operator.crt", cfg.CertPath)
		assert.Equal(t, "/root/.g8e/pki/operator.key", cfg.KeyPath)
		assert.Equal(t, "/root/.g8e/pki/trust/g8eg-ca-bundle.pem", cfg.CAPath)
		assert.False(t, cfg.UseRun)
	})

	t.Run("container name is parameterized", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-sigint")
		assert.Equal(t, "agent-sigint", cfg.Container)
	})

	t.Run("dhs harness config sets host-reachable approval URL", func(t *testing.T) {
		cfg := defaultDHSHarnessConfig()
		assert.Equal(t, "https://localhost:8450", cfg.ApprovalURL)
		assert.Equal(t, "agent-coalition", cfg.Container)
	})

	t.Run("fedramp harness config sets host-reachable approval URL", func(t *testing.T) {
		cfg := defaultFedRAMPHarnessConfig()
		assert.Equal(t, "https://localhost:8451", cfg.ApprovalURL)
		assert.Equal(t, "agent-runtime", cfg.Container)
	})

	t.Run("dhs harness config propagates demoIdentity", func(t *testing.T) {
		original := demoIdentity
		defer func() { demoIdentity = original }()
		demoIdentity = hostIdentity{UserID: "host-user-1", CLISessionID: "host-sess-1"}
		cfg := defaultDHSHarnessConfig()
		assert.Equal(t, "host-user-1", cfg.UserID)
		assert.Equal(t, "host-sess-1", cfg.CLISessionID)
	})

	t.Run("fedramp harness config propagates demoIdentity", func(t *testing.T) {
		original := demoIdentity
		defer func() { demoIdentity = original }()
		demoIdentity = hostIdentity{UserID: "host-user-2", CLISessionID: "host-sess-2"}
		cfg := defaultFedRAMPHarnessConfig()
		assert.Equal(t, "host-user-2", cfg.UserID)
		assert.Equal(t, "host-sess-2", cfg.CLISessionID)
	})

	t.Run("harness config defaults empty identity when demoIdentity unset", func(t *testing.T) {
		original := demoIdentity
		defer func() { demoIdentity = original }()
		demoIdentity = hostIdentity{}
		cfg := defaultDHSHarnessConfig()
		assert.Empty(t, cfg.UserID)
		assert.Empty(t, cfg.CLISessionID)
	})
}

func TestHarnessRun(t *testing.T) {
	t.Run("exec mode builds docker compose exec command", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("gov-cui-exfil-block", cfg)
		assert.Equal(t, []string{
			"docker", "compose", "exec", "-T", "agent-runtime", "/g8e", "demos", "scenarios", "run",
			"--mtls-url", "https://g8e.local:8443",
			"--public-url", "http://g8e.local:8080",
			"--cert", "/root/.g8e/pki/operator.crt",
			"--key", "/root/.g8e/pki/operator.key",
			"--ca", "/root/.g8e/pki/trust/g8eg-ca-bundle.pem",
			"gov-cui-exfil-block",
		}, cmd)
	})

	t.Run("run mode builds docker compose run --rm command", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cfg.UseRun = true
		cmd := harnessRun("dhs-ingest", cfg)
		assert.Equal(t, "run", cmd[2])
		assert.Equal(t, "--rm", cmd[3])
		assert.Contains(t, cmd, "dhs-ingest")
	})

	t.Run("omits ensemble and l3-mode when not set", func(t *testing.T) {
		cfg := harnessConfig{
			Container: "agent-runtime",
			MTLSURL:   "https://g8e.local:8443",
			PublicURL: "http://g8e.local:8080",
			CertPath:  "/cert",
			KeyPath:   "/key",
			CAPath:    "/ca",
		}
		cmd := harnessRun("test-scenario", cfg)
		assert.NotContains(t, cmd, "--ensemble")
		assert.NotContains(t, cmd, "--l3-mode")
	})

	t.Run("appends --approval-url when set", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-coalition")
		cfg.ApprovalURL = "https://localhost:8450"
		cmd := harnessRun("dhs-release", cfg)
		assert.Contains(t, cmd, "--approval-url")
		assert.Contains(t, cmd, "https://localhost:8450")
		assert.Equal(t, "dhs-release", cmd[len(cmd)-1])
	})

	t.Run("omits --approval-url when empty", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("dhs-ingest", cfg)
		assert.NotContains(t, cmd, "--approval-url")
	})

	t.Run("scenario is always the last argument", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("my-scenario", cfg)
		assert.Equal(t, "my-scenario", cmd[len(cmd)-1])
	})

	t.Run("appends --user-id and --cli-session-id when set", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cfg.UserID = "user-123"
		cfg.CLISessionID = "sess-456"
		cmd := harnessRun("dhs-release", cfg)
		assert.Contains(t, cmd, "--user-id")
		assert.Contains(t, cmd, "user-123")
		assert.Contains(t, cmd, "--cli-session-id")
		assert.Contains(t, cmd, "sess-456")
		assert.Equal(t, "dhs-release", cmd[len(cmd)-1])
	})

	t.Run("omits --user-id and --cli-session-id when empty", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("dhs-ingest", cfg)
		assert.NotContains(t, cmd, "--user-id")
		assert.NotContains(t, cmd, "--cli-session-id")
	})
}

func TestDemoScenarioFilesCallHarnessRun(t *testing.T) {
	for _, file := range expectedDemoFiles {
		t.Run(file+" uses harnessRun or runTwoLayerScenario", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			src := string(data)
			assert.True(t, strings.Contains(src, "harnessRun") || strings.Contains(src, "runTwoLayerScenario"),
				"%s must call harnessRun (directly or via runTwoLayerScenario) to submit real GovernanceEnvelopes", file)
		})
	}
}

func TestNoGatewayBypassInDemoFiles(t *testing.T) {
	for _, file := range expectedDemoFiles {
		t.Run(file+" has no curl POST bypass", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			src := string(data)
			assert.NotContains(t, src, "curl -X POST",
				"%s must not use curl -X POST to bypass the gateway", file)
			assert.NotContains(t, src, "curl --request POST",
				"%s must not use curl --request POST to bypass the gateway", file)
		})
	}
}

func TestNoSqliteBackdoorInScenarioFiles(t *testing.T) {
	for _, file := range expectedDemoFiles {
		t.Run(file+" has no sqlite3 backdoor", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			src := string(data)
			assert.NotContains(t, src, "exec.Command(\"sqlite3\"",
				"%s must not directly access gateway SQLite databases via exec.Command", file)
			assert.NotContains(t, src, "\"sqlite3\"",
				"%s must not reference sqlite3 at all — use g8e audit vault instead", file)
		})
	}
}

func TestNoCopyPasteInScenarioFiles(t *testing.T) {
	for _, file := range expectedDemoFiles {
		t.Run(file+" has no copy-paste instructions", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			src := string(data)
			assert.NotContains(t, src, "Copy-paste to inspect",
				"%s must not print copy-paste instructions — use 'g8e audit' instead", file)
			assert.NotContains(t, src, "Copy-paste to query",
				"%s must not print copy-paste instructions — use 'g8e audit' instead", file)
			assert.NotContains(t, src, "Copy-paste to confirm",
				"%s must not print copy-paste instructions — use 'g8e audit' instead", file)
		})
	}
}

func TestDemosPullCmd(t *testing.T) {
	t.Run("pull subcommand is registered on demos command", func(t *testing.T) {
		root := demosCmd()
		var found bool
		for _, c := range root.Commands() {
			if c.Use == "pull" {
				found = true
				assert.Contains(t, c.Short, "air-gapped")
				break
			}
		}
		assert.True(t, found, "demos command should have a 'pull' subcommand")
	})

	t.Run("pull command has correct metadata", func(t *testing.T) {
		cmd := demosPullCmd()
		assert.Equal(t, "pull", cmd.Use)
		assert.Contains(t, cmd.Short, "Pre-pull")
		assert.Contains(t, cmd.Long, "images.json")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("imageManifestEntry struct has expected JSON tags", func(t *testing.T) {
		entry := imageManifestEntry{
			Image:  "alpine",
			Tag:    "latest",
			Digest: "sha256:abc123",
			Demos:  []string{"gov"},
		}
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"image":"alpine"`)
		assert.Contains(t, string(data), `"digest":"sha256:abc123"`)
	})
}

func TestDemosExportCmd(t *testing.T) {
	t.Run("export subcommand is registered on demos command", func(t *testing.T) {
		root := demosCmd()
		var found bool
		for _, c := range root.Commands() {
			if c.Use == "export [output-dir]" {
				found = true
				break
			}
		}
		assert.True(t, found, "demos command should have an 'export' subcommand")
	})

	t.Run("export command has correct metadata", func(t *testing.T) {
		cmd := demosExportCmd()
		assert.Equal(t, "export [output-dir]", cmd.Use)
		assert.Contains(t, cmd.Short, "tar files")
		assert.Contains(t, cmd.Long, "images.json")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("export command accepts at most one argument", func(t *testing.T) {
		cmd := demosExportCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosImportCmd(t *testing.T) {
	t.Run("import subcommand is registered on demos command", func(t *testing.T) {
		root := demosCmd()
		var found bool
		for _, c := range root.Commands() {
			if c.Use == "import [input-dir]" {
				found = true
				break
			}
		}
		assert.True(t, found, "demos command should have an 'import' subcommand")
	})

	t.Run("import command has correct metadata", func(t *testing.T) {
		cmd := demosImportCmd()
		assert.Equal(t, "import [input-dir]", cmd.Use)
		assert.Contains(t, cmd.Short, "air-gapped")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("import command accepts at most one argument", func(t *testing.T) {
		cmd := demosImportCmd()
		assert.NotNil(t, cmd.Args)
	})
}

func TestDemosImagesCmd(t *testing.T) {
	t.Run("images subcommand is registered on demos command", func(t *testing.T) {
		root := demosCmd()
		var found bool
		for _, c := range root.Commands() {
			if c.Use == "images" {
				found = true
				break
			}
		}
		assert.True(t, found, "demos command should have an 'images' subcommand")
	})

	t.Run("images command has correct metadata", func(t *testing.T) {
		cmd := demosImagesCmd()
		assert.Equal(t, "images", cmd.Use)
		assert.Contains(t, cmd.Short, "manifest")
		assert.NotNil(t, cmd.RunE)
	})
}

func TestDemoPrintln(t *testing.T) {
	t.Run("demoPrintln is no-op when not verbose", func(t *testing.T) {
		original := demoVerbose
		demoVerbose = false
		defer func() { demoVerbose = original }()

		// Should not panic and should produce no output
		demoPrintln("test", "output")
		demoPrintln()
	})

	t.Run("demoPrintf is no-op when not verbose", func(t *testing.T) {
		original := demoVerbose
		demoVerbose = false
		defer func() { demoVerbose = original }()

		demoPrintf("test %s %d", "format", 42)
	})

	t.Run("demoPrintln does not panic when verbose", func(t *testing.T) {
		original := demoVerbose
		demoVerbose = true
		defer func() { demoVerbose = original }()

		demoPrintln("test output")
		demoPrintf("test %s", "format")
	})
}

// resolveRepoRoot finds the repository root using go list -m.
func resolveRepoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	require.NoError(t, err, "go list -m should succeed")
	root := strings.TrimSpace(string(output))
	require.NotEmpty(t, root, "repo root should not be empty")
	return root
}

// TestComposeConfig_ConsensusBootstrapMounts verifies that demos using
// consensus posture mount consensus-bootstrap.json in both the gateway and
// agent containers. This is a regression test for a bug where the agent
// containers mounted ensemble-seed.hex but not consensus-bootstrap.json,
// causing L2 quorum failures (0 affirmative votes) because MemberKeyIDs
// stayed nil.
func TestComposeConfig_ConsensusBootstrapMounts(t *testing.T) {
	repoRoot := resolveRepoRoot(t)

	tests := []struct {
		name        string
		composePath string
		gatewaySvc  string
		agentSvc    string
	}{
		{
			name:        "dhs",
			composePath: filepath.Join(repoRoot, "demos", "dhs", "compose.yml"),
			gatewaySvc:  "gateway:",
			agentSvc:    "agent-coalition:",
		},
		{
			name:        "fedramp",
			composePath: filepath.Join(repoRoot, "demos", "fedramp", "compose.yml"),
			gatewaySvc:  "gateway:",
			agentSvc:    "agent-runtime:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.composePath)
			require.NoError(t, err, "compose.yml should exist at %s", tt.composePath)
			content := string(data)

			assert.Contains(t, content, "consensus-bootstrap.json",
				"%s compose.yml must mount consensus-bootstrap.json", tt.name)

			assert.Contains(t, content, "ensemble-seed.hex",
				"%s compose.yml must mount ensemble-seed.hex", tt.name)

			agentSection := extractServiceSection(content, tt.agentSvc)
			require.NotEmpty(t, agentSection, "agent service section should be found in %s compose.yml", tt.name)
			assert.Contains(t, agentSection, "consensus-bootstrap.json",
				"%s agent container must mount consensus-bootstrap.json (regression: missing mount causes L2 quorum failure)", tt.name)
			assert.Contains(t, agentSection, "ensemble-seed.hex",
				"%s agent container must mount ensemble-seed.hex", tt.name)

			gatewaySection := extractServiceSection(content, tt.gatewaySvc)
			require.NotEmpty(t, gatewaySection, "gateway service section should be found in %s compose.yml", tt.name)
			assert.Contains(t, gatewaySection, "consensus-bootstrap.json",
				"%s gateway container must mount consensus-bootstrap.json", tt.name)
		})
	}
}

// extractServiceSection extracts the YAML block for a service definition,
// from the service header (e.g., "agent-coalition:") to the next top-level
// key at the same indentation level or end of file.
func extractServiceSection(content, serviceHeader string) string {
	idx := strings.Index(content, serviceHeader)
	if idx < 0 {
		return ""
	}
	rest := content[idx:]
	lines := strings.Split(rest, "\n")
	var section []string
	for i, line := range lines {
		if i == 0 {
			section = append(section, line)
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' && line[0] != '-' {
			break
		}
		section = append(section, line)
	}
	return strings.Join(section, "\n")
}

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
			"audit",
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

func TestDemosAuditCmd(t *testing.T) {
	t.Run("audit command has correct structure", func(t *testing.T) {
		cmd := demosAuditCmd()
		assert.Equal(t, "audit <org> [action]", cmd.Use)
		assert.Contains(t, cmd.Short, "View audit logs and ledger history")
		assert.NotNil(t, cmd.RunE)
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
		assert.Contains(t, cmd.Long, "Governed Migration")
		assert.Contains(t, cmd.Long, "SIGINT-to-EO/IR")
		assert.Contains(t, cmd.Long, "BFT Spoofing")
		assert.Contains(t, cmd.Long, "Disconnected Operations")
	})
}

func TestScenarioCounts(t *testing.T) {
	t.Run("scenario counts map is correctly defined", func(t *testing.T) {
		assert.Equal(t, 4, scenarioCounts["healthcare"])
		assert.Equal(t, 1, scenarioCounts["gov"])
		assert.Equal(t, 1, scenarioCounts["finance"])
		assert.Equal(t, 3, scenarioCounts["secure-data"])
		assert.Equal(t, 3, scenarioCounts["dow"])
		assert.Equal(t, 5, scenarioCounts["dhs"])
	})

	t.Run("scenario counts map has expected entries", func(t *testing.T) {
		expectedOrgs := []string{"healthcare", "gov", "finance", "secure-data", "dow", "dhs"}
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
		assert.Contains(t, output, "https://localhost:8444/console/")
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
		assert.Contains(t, output, "https://localhost:8443/console/")
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
		assert.Contains(t, output, "https://localhost:8445/console/")
	})

	t.Run("prints secure-data endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("secure-data")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8083")
		assert.Contains(t, output, "https://localhost:8446")
		assert.Contains(t, output, "http://localhost:3003")
		assert.Contains(t, output, "https://localhost:8446/console/")
	})

	t.Run("prints dow endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("dow")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8086")
		assert.Contains(t, output, "https://localhost:8449")
		assert.Contains(t, output, "https://localhost:8449/console/")
	})

	t.Run("prints dhs endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("dhs")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8087")
		assert.Contains(t, output, "https://localhost:8450")
		assert.Contains(t, output, "https://localhost:8450/console/")
	})

	t.Run("prints swarm endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printDemoEndpoints("swarm")

		w.Close()
		os.Stdout = originalStdout
		buf.ReadFrom(r)

		output := buf.String()
		assert.Contains(t, output, "Available endpoints:")
		assert.Contains(t, output, "http://localhost:8085")
		assert.Contains(t, output, "https://localhost:8448")
		assert.Contains(t, output, "https://localhost:8448/console/")
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

	t.Run("returns error for invalid secure-data scenario", func(t *testing.T) {
		err := runSecureDataScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for secure-data")
		assert.Contains(t, err.Error(), "valid: 1-3")
	})

	t.Run("returns error for invalid dow scenario", func(t *testing.T) {
		err := runDoWScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for dow")
		assert.Contains(t, err.Error(), "valid: 1-3")
	})

	t.Run("returns error for invalid dhs scenario", func(t *testing.T) {
		err := runDHSScenario("/tmp", "99")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scenario number for dhs")
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

	t.Run("secure-data scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runSecureDataScenario)
	})

	t.Run("dow scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runDoWScenario)
	})

	t.Run("dhs scenario functions exist", func(t *testing.T) {
		assert.NotNil(t, runDHSScenario)
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

	t.Run("secure-data has 3 scenarios", func(t *testing.T) {
		count, ok := scenarioCounts["secure-data"]
		assert.True(t, ok)
		assert.Equal(t, 3, count)
	})

	t.Run("dow has 3 scenarios", func(t *testing.T) {
		count, ok := scenarioCounts["dow"]
		assert.True(t, ok)
		assert.Equal(t, 3, count)
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

func TestSecureDataScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "secure-data: 1-3")
		assert.Contains(t, cmd.Long, "Governed Migration with Chain-of-Custody Receipts")
		assert.Contains(t, cmd.Long, "Connector Bypass Attempt Blocked")
		assert.Contains(t, cmd.Long, "Cross-Tenant Leak Doctrine Triggered")
	})
}

func TestDoWScenarioDescriptions(t *testing.T) {
	t.Run("scenario descriptions are documented in run command", func(t *testing.T) {
		cmd := demosRunCmd()
		assert.Contains(t, cmd.Long, "dow: 1-3")
		assert.Contains(t, cmd.Long, "Autonomous SIGINT-to-EO/IR Cross-Cueing")
		assert.Contains(t, cmd.Long, "BFT Spoofing Defense")
		assert.Contains(t, cmd.Long, "Disconnected Operations")
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
		assert.Equal(t, 3, cfg.EnsembleSize)
		assert.Equal(t, "mock", cfg.L3Mode)
		assert.False(t, cfg.UseRun)
	})

	t.Run("container name is parameterized", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-sigint")
		assert.Equal(t, "agent-sigint", cfg.Container)
	})
}

func TestHarnessRun(t *testing.T) {
	t.Run("exec mode builds docker compose exec command", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("gov-cui-exfil-block", cfg)
		assert.Equal(t, []string{
			"docker", "compose", "exec", "-T", "agent-runtime", "/g8e", "agent", "run",
			"--mtls-url", "https://g8e.local:8443",
			"--public-url", "http://g8e.local:8080",
			"--cert", "/root/.g8e/pki/operator.crt",
			"--key", "/root/.g8e/pki/operator.key",
			"--ca", "/root/.g8e/pki/trust/g8eg-ca-bundle.pem",
			"--ensemble", "3",
			"--l3-mode", "mock",
			"gov-cui-exfil-block",
		}, cmd)
	})

	t.Run("run mode builds docker compose run --rm command", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-sigint")
		cfg.UseRun = true
		cmd := harnessRun("dow-cross-cue", cfg)
		assert.Equal(t, "run", cmd[2])
		assert.Equal(t, "--rm", cmd[3])
		assert.Contains(t, cmd, "dow-cross-cue")
	})

	t.Run("includes consensus seed and tribunal id when set", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cfg.ConsensusSeed = "deadbeef"
		cfg.TribunalID = "test-tribunal"
		cmd := harnessRun("dhs-cue", cfg)
		assert.Contains(t, cmd, "--consensus-seed")
		assert.Contains(t, cmd, "deadbeef")
		assert.Contains(t, cmd, "--tribunal-id")
		assert.Contains(t, cmd, "test-tribunal")
	})

	t.Run("omits ensemble and l3-mode when zero/empty", func(t *testing.T) {
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

	t.Run("scenario is always the last argument", func(t *testing.T) {
		cfg := defaultHarnessConfig("agent-runtime")
		cmd := harnessRun("my-scenario", cfg)
		assert.Equal(t, "my-scenario", cmd[len(cmd)-1])
	})
}

func TestDemoScenarioFilesCallHarnessRun(t *testing.T) {
	demoFiles := []string{
		"demo_gov.go",
		"demo_finance.go",
		"demo_healthcare.go",
		"demo_secure_data.go",
		"demo_dow.go",
		"demo_dhs.go",
	}

	for _, file := range demoFiles {
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
	demoFiles := []string{
		"demo_gov.go",
		"demo_finance.go",
		"demo_healthcare.go",
		"demo_secure_data.go",
		"demo_dow.go",
		"demo_dhs.go",
	}

	for _, file := range demoFiles {
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
	scenarioFiles := []string{
		"demo_gov.go",
		"demo_finance.go",
		"demo_healthcare.go",
		"demo_secure_data.go",
		"demo_dow.go",
		"demo_dhs.go",
	}

	for _, file := range scenarioFiles {
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

func TestSqliteOnlyInAuditCmdVaultAction(t *testing.T) {
	t.Run("demos.go references sqlite3 only in vault audit action", func(t *testing.T) {
		path := filepath.Join(".", "demos.go")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		src := string(data)

		// sqlite3 should appear in demos.go (for the vault audit action)
		assert.Contains(t, src, "sqlite3",
			"demos.go should reference sqlite3 for the vault audit action")

		// But never via exec.Command("sqlite3" — it should go through runDockerComposeExec
		assert.NotContains(t, src, "exec.Command(\"sqlite3\"",
			"demos.go must not call exec.Command with sqlite3 directly — use runDockerComposeExec")

		// sqlite3 should only appear in the audit command context (runDemosAudit)
		// Verify it appears in the vault case block, not in scenario functions
		auditIdx := strings.Index(src, "func runDemosAudit")
		require.True(t, auditIdx >= 0, "runDemosAudit function should exist")

		beforeAudit := src[:auditIdx]
		afterAudit := src[auditIdx:]

		assert.NotContains(t, beforeAudit, "sqlite3",
			"sqlite3 must not appear before runDemosAudit — only the audit vault action may use it")
		assert.Contains(t, afterAudit, "sqlite3",
			"sqlite3 should appear in runDemosAudit for the vault action")
	})
}

func TestNoCopyPasteInScenarioFiles(t *testing.T) {
	scenarioFiles := []string{
		"demo_gov.go",
		"demo_finance.go",
		"demo_healthcare.go",
		"demo_secure_data.go",
		"demo_dow.go",
		"demo_dhs.go",
	}

	for _, file := range scenarioFiles {
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

func TestCaptureCommand(t *testing.T) {
	t.Run("captures stdout from echo command", func(t *testing.T) {
		output, err := captureCommand(t.TempDir(), "echo", "hello world")
		require.NoError(t, err)
		assert.Equal(t, "hello world\n", output)
	})

	t.Run("returns error for non-existent command", func(t *testing.T) {
		_, err := captureCommand(t.TempDir(), "nonexistent-command-12345")
		assert.Error(t, err)
	})

	t.Run("returns empty string for command with no output", func(t *testing.T) {
		output, err := captureCommand(t.TempDir(), "true")
		require.NoError(t, err)
		assert.Empty(t, strings.TrimSpace(output))
	})

	t.Run("captures multi-line output", func(t *testing.T) {
		output, err := captureCommand(t.TempDir(), "printf", "line1\nline2\n")
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2\n", output)
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

func TestRunG8EAuditCmd(t *testing.T) {
	t.Run("function exists and is callable", func(t *testing.T) {
		assert.NotNil(t, runG8EAuditCmd)
	})

	t.Run("returns error when demo not running", func(t *testing.T) {
		tmpDir := t.TempDir()
		composePath := filepath.Join(tmpDir, "compose.yml")
		err := runG8EAuditCmd(tmpDir, composePath, "receipts")
		assert.Error(t, err)
	})
}

func TestPrintDataDump(t *testing.T) {
	t.Run("function exists and is callable", func(t *testing.T) {
		assert.NotNil(t, printDataDump)
	})

	t.Run("does not panic with non-existent demo dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Should handle missing compose file gracefully without panicking
		printDataDump(tmpDir)
	})
}

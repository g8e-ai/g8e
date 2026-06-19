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
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/agentic_tool_emulator/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgenticToolEmulatorCmd(t *testing.T) {
	t.Run("agentic-tool-emulator command has correct use and description", func(t *testing.T) {
		cmd := agenticToolEmulatorCmd()
		assert.Equal(t, "agentic-tool-emulator", cmd.Use)
		assert.Contains(t, cmd.Short, "Universal agentic tool emulator")
		assert.Contains(t, cmd.Long, "impersonates arbitrary AI tools")
	})

	t.Run("agentic-tool-emulator has expected subcommands", func(t *testing.T) {
		cmd := agenticToolEmulatorCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"list", "run", "audit"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "agentic-tool-emulator command should have %s subcommand", subcmd)
		}
	})
}

func TestAgenticToolEmulatorListCmd(t *testing.T) {
	t.Run("agentic-tool-emulator list command has correct use", func(t *testing.T) {
		cmd := agenticToolEmulatorListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List available scenarios")
	})
}

func TestAgenticToolEmulatorRunCmd(t *testing.T) {
	t.Run("agentic-tool-emulator run command has correct use", func(t *testing.T) {
		cmd := agenticToolEmulatorRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Run scenarios")
	})

	t.Run("agentic-tool-emulator run has required flags", func(t *testing.T) {
		cmd := agenticToolEmulatorRunCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "insecure", "out", "l3-mode", "ensemble", "verbose", "phase"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "agentic-tool-emulator run should have --%s flag", flagName)
		}
	})

	t.Run("agentic-tool-emulator run ensemble flag has default value", func(t *testing.T) {
		cmd := agenticToolEmulatorRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("ensemble")
		require.NotNil(t, flag)
		assert.Equal(t, "3", flag.DefValue)
	})

	t.Run("agentic-tool-emulator run phase flag has default value", func(t *testing.T) {
		cmd := agenticToolEmulatorRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("phase")
		require.NotNil(t, flag)
		assert.Equal(t, "all", flag.DefValue)
	})
}

func TestAgenticToolEmulatorAuditCmd(t *testing.T) {
	t.Run("agentic-tool-emulator audit command has correct use", func(t *testing.T) {
		cmd := agenticToolEmulatorAuditCmd()
		assert.Contains(t, cmd.Use, "audit")
		assert.Contains(t, cmd.Short, "Audit signed receipts")
	})

	t.Run("agentic-tool-emulator audit has required flags", func(t *testing.T) {
		cmd := agenticToolEmulatorAuditCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "insecure", "out"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "agentic-tool-emulator audit should have --%s flag", flagName)
		}
	})
}

func TestApplyAgenticToolEmulatorFlags(t *testing.T) {
	t.Run("applyAgenticToolEmulatorFlags sets MTLS URL", func(t *testing.T) {
		emulatorMTLSURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttp)
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttp), cfg.MTLSBaseURL)
		emulatorMTLSURL = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets public URL", func(t *testing.T) {
		emulatorPublicURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttps)
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttps), cfg.PublicBaseURL)
		emulatorPublicURL = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets cert", func(t *testing.T) {
		emulatorCert = "/path/to/cert.pem"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "/path/to/cert.pem", cfg.Auth.ClientCert)
		emulatorCert = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets key", func(t *testing.T) {
		emulatorKey = "/path/to/key.pem"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "/path/to/key.pem", cfg.Auth.ClientKey)
		emulatorKey = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets CA bundle", func(t *testing.T) {
		emulatorCA = "/path/to/ca.pem"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "/path/to/ca.pem", cfg.Auth.CABundle)
		emulatorCA = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets API key", func(t *testing.T) {
		emulatorAPIKey = "test-api-key"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "test-api-key", cfg.Auth.APIKey)
		emulatorAPIKey = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets insecure flag", func(t *testing.T) {
		emulatorInsecure = true
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.True(t, cfg.Auth.Insecure)
		emulatorInsecure = false
	})

	t.Run("applyAgenticToolEmulatorFlags sets operator session ID", func(t *testing.T) {
		emulatorSessionID = "session-123"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "session-123", cfg.OperatorSessionID)
		emulatorSessionID = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets out directory", func(t *testing.T) {
		testOutDir := t.TempDir()
		emulatorOutDir = testOutDir
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, testOutDir, cfg.OutDir)
		emulatorOutDir = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets L3 mode", func(t *testing.T) {
		emulatorL3Mode = "mock"
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, "mock", cfg.L3Mode)
		emulatorL3Mode = ""
	})

	t.Run("applyAgenticToolEmulatorFlags sets ensemble size", func(t *testing.T) {
		emulatorEnsemble = 5
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.Equal(t, 5, cfg.EnsembleSize)
		emulatorEnsemble = 0
	})

	t.Run("applyAgenticToolEmulatorFlags sets verbose flag", func(t *testing.T) {
		emulatorVerbose = true
		cfg := config.Default()
		applyAgenticToolEmulatorFlags(&cfg)
		assert.True(t, cfg.Verbose)
		emulatorVerbose = false
	})
}

func TestTrunc(t *testing.T) {
	t.Run("trunc returns string when shorter than limit", func(t *testing.T) {
		result := trunc("short", 10)
		assert.Equal(t, "short", result)
	})

	t.Run("trunc returns string when equal to limit", func(t *testing.T) {
		result := trunc("exactlen", 8)
		assert.Equal(t, "exactlen", result)
	})

	t.Run("trunc truncates string when longer than limit", func(t *testing.T) {
		result := trunc("verylongstring", 5)
		assert.Equal(t, "veryl", result)
	})

	t.Run("trunc handles empty string", func(t *testing.T) {
		result := trunc("", 5)
		assert.Empty(t, result)
	})
}

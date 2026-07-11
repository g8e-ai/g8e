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
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentHarnessCmd(t *testing.T) {
	t.Run("agent command has correct Use and description", func(t *testing.T) {
		cmd := agentHarnessCmd()
		assert.Equal(t, "agent", cmd.Use)
		assert.Contains(t, cmd.Short, "Universal agent harness")
		assert.Contains(t, cmd.Long, "impersonates arbitrary AI tools")
	})

	t.Run("agent has agent-harness alias", func(t *testing.T) {
		cmd := agentHarnessCmd()
		assert.Contains(t, cmd.Aliases, "agent-harness")
	})

	t.Run("agent has expected subcommands", func(t *testing.T) {
		cmd := agentHarnessCmd()
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
			assert.True(t, found, "agent command should have %s subcommand", subcmd)
		}
	})
}

func TestAgentHarnessListCmd(t *testing.T) {
	t.Run("agent list command has correct use", func(t *testing.T) {
		cmd := agentHarnessListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "agents")
	})
}

func TestAgentHarnessRunCmd(t *testing.T) {
	t.Run("agent run command has correct use", func(t *testing.T) {
		cmd := agentHarnessRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Run scenarios")
	})

	t.Run("agent run has required flags", func(t *testing.T) {
		cmd := agentHarnessRunCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "out", "l3-mode", "ensemble", "verbose", "phase"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "agent run should have --%s flag", flagName)
		}
	})

	t.Run("agent run ensemble flag has default value", func(t *testing.T) {
		cmd := agentHarnessRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("ensemble")
		require.NotNil(t, flag)
		assert.Equal(t, "3", flag.DefValue)
	})

	t.Run("agent run phase flag has default value", func(t *testing.T) {
		cmd := agentHarnessRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("phase")
		require.NotNil(t, flag)
		assert.Equal(t, "all", flag.DefValue)
	})
}

func TestAgentHarnessAuditCmd(t *testing.T) {
	t.Run("agent audit command has correct use", func(t *testing.T) {
		cmd := agentHarnessAuditCmd()
		assert.Contains(t, cmd.Use, "audit")
		assert.Contains(t, cmd.Short, "Audit signed receipts")
	})

	t.Run("agent audit has required flags", func(t *testing.T) {
		cmd := agentHarnessAuditCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "out"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "agent audit should have --%s flag", flagName)
		}
	})
}

func TestApplyAgentHarnessFlags(t *testing.T) {
	t.Run("applyAgentHarnessFlags sets MTLS URL", func(t *testing.T) {
		harnessMTLSURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttp)
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttp), cfg.MTLSBaseURL)
		harnessMTLSURL = ""
	})

	t.Run("applyAgentHarnessFlags sets public URL", func(t *testing.T) {
		harnessPublicURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttps)
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttps), cfg.PublicBaseURL)
		harnessPublicURL = ""
	})

	t.Run("applyAgentHarnessFlags sets cert", func(t *testing.T) {
		harnessCert = "/path/to/cert.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/cert.pem", cfg.Auth.ClientCert)
		harnessCert = ""
	})

	t.Run("applyAgentHarnessFlags sets key", func(t *testing.T) {
		harnessKey = "/path/to/key.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/key.pem", cfg.Auth.ClientKey)
		harnessKey = ""
	})

	t.Run("applyAgentHarnessFlags sets CA bundle", func(t *testing.T) {
		harnessCA = "/path/to/ca.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/ca.pem", cfg.Auth.CABundle)
		harnessCA = ""
	})

	t.Run("applyAgentHarnessFlags sets API key", func(t *testing.T) {
		harnessAPIKey = "test-api-key"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "test-api-key", cfg.Auth.APIKey)
		harnessAPIKey = ""
	})

	t.Run("applyAgentHarnessFlags sets operator session ID", func(t *testing.T) {
		harnessSessionID = "session-123"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "session-123", cfg.OperatorSessionID)
		harnessSessionID = ""
	})

	t.Run("applyAgentHarnessFlags sets out directory", func(t *testing.T) {
		testOutDir := t.TempDir()
		harnessOutDir = testOutDir
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, testOutDir, cfg.OutDir)
		harnessOutDir = ""
	})

	t.Run("applyAgentHarnessFlags sets L3 mode", func(t *testing.T) {
		harnessL3Mode = "mock"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "mock", cfg.L3Mode)
		harnessL3Mode = ""
	})

	t.Run("applyAgentHarnessFlags sets ensemble size", func(t *testing.T) {
		harnessEnsemble = 5
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, 5, cfg.EnsembleSize)
		harnessEnsemble = 0
	})

	t.Run("applyAgentHarnessFlags sets verbose flag", func(t *testing.T) {
		harnessVerbose = true
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.True(t, cfg.Verbose)
		harnessVerbose = false
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

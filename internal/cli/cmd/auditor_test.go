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

	"github.com/g8e-ai/g8e/internal/auditor/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditorCmd(t *testing.T) {
	t.Run("auditor command has correct use and description", func(t *testing.T) {
		cmd := auditorCmd()
		assert.Equal(t, "auditor", cmd.Use)
		assert.Contains(t, cmd.Short, "Universal agent emulator")
		assert.Contains(t, cmd.Long, "impersonates arbitrary AI tools")
	})

	t.Run("auditor has expected subcommands", func(t *testing.T) {
		cmd := auditorCmd()
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
			assert.True(t, found, "auditor command should have %s subcommand", subcmd)
		}
	})
}

func TestAuditorListCmd(t *testing.T) {
	t.Run("auditor list command has correct use", func(t *testing.T) {
		cmd := auditorListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List available scenarios")
	})
}

func TestAuditorRunCmd(t *testing.T) {
	t.Run("auditor run command has correct use", func(t *testing.T) {
		cmd := auditorRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Run scenarios")
	})

	t.Run("auditor run has required flags", func(t *testing.T) {
		cmd := auditorRunCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "insecure", "out", "l3-mode", "ensemble", "verbose", "phase"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "auditor run should have --%s flag", flagName)
		}
	})

	t.Run("auditor run ensemble flag has default value", func(t *testing.T) {
		cmd := auditorRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("ensemble")
		require.NotNil(t, flag)
		assert.Equal(t, "3", flag.DefValue)
	})

	t.Run("auditor run phase flag has default value", func(t *testing.T) {
		cmd := auditorRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("phase")
		require.NotNil(t, flag)
		assert.Equal(t, "all", flag.DefValue)
	})
}

func TestAuditorAuditCmd(t *testing.T) {
	t.Run("auditor audit command has correct use", func(t *testing.T) {
		cmd := auditorAuditCmd()
		assert.Contains(t, cmd.Use, "audit")
		assert.Contains(t, cmd.Short, "Audit signed receipts")
	})

	t.Run("auditor audit has required flags", func(t *testing.T) {
		cmd := auditorAuditCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "cert", "key", "ca", "api-key", "operator-session", "insecure", "out"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "auditor audit should have --%s flag", flagName)
		}
	})
}

func TestApplyAuditorFlags(t *testing.T) {
	t.Run("applyAuditorFlags sets MTLS URL", func(t *testing.T) {
		auditorMTLSURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttp)
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttp), cfg.MTLSBaseURL)
		auditorMTLSURL = ""
	})

	t.Run("applyAuditorFlags sets public URL", func(t *testing.T) {
		auditorPublicURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttps)
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttps), cfg.PublicBaseURL)
		auditorPublicURL = ""
	})

	t.Run("applyAuditorFlags sets cert", func(t *testing.T) {
		auditorCert = "/path/to/cert.pem"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "/path/to/cert.pem", cfg.Auth.ClientCert)
		auditorCert = ""
	})

	t.Run("applyAuditorFlags sets key", func(t *testing.T) {
		auditorKey = "/path/to/key.pem"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "/path/to/key.pem", cfg.Auth.ClientKey)
		auditorKey = ""
	})

	t.Run("applyAuditorFlags sets CA bundle", func(t *testing.T) {
		auditorCA = "/path/to/ca.pem"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "/path/to/ca.pem", cfg.Auth.CABundle)
		auditorCA = ""
	})

	t.Run("applyAuditorFlags sets API key", func(t *testing.T) {
		auditorAPIKey = "test-api-key"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "test-api-key", cfg.Auth.APIKey)
		auditorAPIKey = ""
	})

	t.Run("applyAuditorFlags sets insecure flag", func(t *testing.T) {
		auditorInsecure = true
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.True(t, cfg.Auth.Insecure)
		auditorInsecure = false
	})

	t.Run("applyAuditorFlags sets operator session ID", func(t *testing.T) {
		auditorSessionID = "session-123"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "session-123", cfg.OperatorSessionID)
		auditorSessionID = ""
	})

	t.Run("applyAuditorFlags sets out directory", func(t *testing.T) {
		testOutDir := t.TempDir()
		auditorOutDir = testOutDir
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, testOutDir, cfg.OutDir)
		auditorOutDir = ""
	})

	t.Run("applyAuditorFlags sets L3 mode", func(t *testing.T) {
		auditorL3Mode = "mock"
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, "mock", cfg.L3Mode)
		auditorL3Mode = ""
	})

	t.Run("applyAuditorFlags sets ensemble size", func(t *testing.T) {
		auditorEnsemble = 5
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.Equal(t, 5, cfg.EnsembleSize)
		auditorEnsemble = 0
	})

	t.Run("applyAuditorFlags sets verbose flag", func(t *testing.T) {
		auditorVerbose = true
		cfg := config.Default()
		applyAuditorFlags(&cfg)
		assert.True(t, cfg.Verbose)
		auditorVerbose = false
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
		assert.Equal(t, "", result)
	})
}

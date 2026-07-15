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
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBinaryName(t *testing.T) {
	t.Run("returns ./g8e on non-Windows platforms", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping non-Windows test on Windows")
		}
		assert.Equal(t, constants.LocalBinaryName, getBinaryName())
	})

	t.Run("returns ./g8e.exe on Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Skipping Windows test on non-Windows platform")
		}
		assert.Equal(t, constants.LocalBinaryNameWindows, getBinaryName())
	})
}

func TestResolveStartConfig(t *testing.T) {
	t.Run("uses CLI flag values when no environment variables set", func(t *testing.T) {
		// Clear environment variables
		unsetEnvVars := []string{"G8E_VAULT_DIR", "G8E_VAULT_KEY"}
		for _, env := range unsetEnvVars {
			original := os.Getenv(env)
			os.Unsetenv(env)
			t.Cleanup(func() {
				if original != "" {
					os.Setenv(env, original)
				}
			})
		}

		cfg := resolveGatewayFlags(GatewayFlags{
			Posture:            "doctrine",
			HTTPPort:           8080,
			HTTPSPort:          8443,
			DataDir:            "/data",
			PKIDir:             "/pki",
			SecretsDir:         "/secrets",
			VaultDir:           "/vault",
			VaultKeyPath:       "/vault/key",
			PasskeyRpID:        "rp-id",
			PasskeyRpName:      "rp-name",
			RateLimitRPS:       100.0,
			RateLimitBurst:     50,
			LogLevel:           "info",
			CertIdentityMode:   "full",
			TribunalID:         "",
			TribunalURL:        "",
		})

		assert.Equal(t, "doctrine", cfg.Posture)
		assert.Equal(t, 8080, cfg.HTTPPort)
		assert.Equal(t, 8443, cfg.HTTPSPort)
		assert.Equal(t, "/data", cfg.DataDir)
		assert.Equal(t, "/pki", cfg.PKIDir)
		assert.Equal(t, "/secrets", cfg.SecretsDir)
		assert.Equal(t, "/vault", cfg.VaultDir)
		assert.Equal(t, "/vault/key", cfg.VaultKeyPath)
		assert.Equal(t, "rp-id", cfg.PasskeyRpID)
		assert.Equal(t, "rp-name", cfg.PasskeyRpName)
		assert.InEpsilon(t, 100.0, cfg.RateLimitRPS, 0.01)
		assert.Equal(t, 50, cfg.RateLimitBurst)
		assert.Equal(t, "info", cfg.LogLevel)
		assert.Equal(t, "full", cfg.CertIdentityMode)
	})

	t.Run("environment variable G8E_VAULT_DIR overrides empty CLI flag", func(t *testing.T) {
		original := os.Getenv("G8E_VAULT_DIR")
		os.Setenv("G8E_VAULT_DIR", "/env/vault")
		t.Cleanup(func() {
			if original != "" {
				os.Setenv("G8E_VAULT_DIR", original)
			} else {
				os.Unsetenv("G8E_VAULT_DIR")
			}
		})

		cfg := resolveGatewayFlags(GatewayFlags{
			Posture:            "doctrine",
			HTTPPort:           8080,
			HTTPSPort:          8443,
			DataDir:            "/data",
			PKIDir:             "/pki",
			SecretsDir:         "/secrets",
			VaultDir:           "", // empty vault dir
			VaultKeyPath:       "/vault/key",
			PasskeyRpID:        "rp-id",
			PasskeyRpName:      "rp-name",
			RateLimitRPS:       100.0,
			RateLimitBurst:     50,
			LogLevel:           "info",
			CertIdentityMode:   "full",
			TribunalID:         "",
			TribunalURL:        "",
		})

		assert.Equal(t, "/env/vault", cfg.VaultDir)
	})

	t.Run("environment variable G8E_VAULT_KEY overrides empty CLI flag", func(t *testing.T) {
		original := os.Getenv("G8E_VAULT_KEY")
		os.Setenv("G8E_VAULT_KEY", "/env/vault/key")
		t.Cleanup(func() {
			if original != "" {
				os.Setenv("G8E_VAULT_KEY", original)
			} else {
				os.Unsetenv("G8E_VAULT_KEY")
			}
		})

		cfg := resolveGatewayFlags(GatewayFlags{
			Posture:            "doctrine",
			HTTPPort:           8080,
			HTTPSPort:          8443,
			DataDir:            "/data",
			PKIDir:             "/pki",
			SecretsDir:         "/secrets",
			VaultDir:           "/vault",
			VaultKeyPath:       "", // empty vault key
			PasskeyRpID:        "rp-id",
			PasskeyRpName:      "rp-name",
			RateLimitRPS:       100.0,
			RateLimitBurst:     50,
			LogLevel:           "info",
			CertIdentityMode:   "full",
			TribunalID:         "",
			TribunalURL:        "",
		})

		assert.Equal(t, "/env/vault/key", cfg.VaultKeyPath)
	})

	t.Run("CLI flag value takes precedence over environment variable when set", func(t *testing.T) {
		original := os.Getenv("G8E_VAULT_DIR")
		os.Setenv("G8E_VAULT_DIR", "/env/vault")
		t.Cleanup(func() {
			if original != "" {
				os.Setenv("G8E_VAULT_DIR", original)
			} else {
				os.Unsetenv("G8E_VAULT_DIR")
			}
		})

		cfg := resolveGatewayFlags(GatewayFlags{
			Posture:            "doctrine",
			HTTPPort:           8080,
			HTTPSPort:          8443,
			DataDir:            "/data",
			PKIDir:             "/pki",
			SecretsDir:         "/secrets",
			VaultDir:           "/cli/vault", // CLI flag set
			VaultKeyPath:       "/vault/key",
			PasskeyRpID:        "rp-id",
			PasskeyRpName:      "rp-name",
			RateLimitRPS:       100.0,
			RateLimitBurst:     50,
			LogLevel:           "info",
			CertIdentityMode:   "full",
			TribunalID:         "",
			TribunalURL:        "",
		})

		assert.Equal(t, "/cli/vault", cfg.VaultDir)
	})
}

func TestDetectIdentity(t *testing.T) {
	t.Run("detectIdentity requires integration testing with real network detector", func(t *testing.T) {
		// detectIdentity uses network.NewDetector which performs real network detection.
		// This function requires integration testing with the //go:build integration tag
		// to test actual network identity detection behavior.
		// Unit tests for this function would require mocking the network.Detector struct,
		// which is not an interface and cannot be easily mocked without refactoring.
		t.Skip("detectIdentity requires integration testing - see internal/services/network/identity_test.go for network detector tests")
	})
}

func TestReExecArgsMatchStartCmdFlags(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	pm, err := platform.NewProcessManager(fileSvc)
	require.NoError(t, err)

	opts := platform.OperatorStartOptions{
		GatewayConfig: serve.GatewayConfig{
			Posture:            g8econfig.GatewayPosture("doctrine"),
			HTTPPort:           8080,
			HTTPSPort:          8443,
			DataDir:            "/data",
			PKIDir:             "/pki",
			SecretsDir:         "/secrets",
			VaultDir:           "/vault",
			VaultKeyPath:       "/vault/key",
			PasskeyRpID:        "localhost",
			PasskeyRpName:      "g8e",
			PasskeyRpOrigins:   []string{"http://localhost:8087", "https://localhost:8450"},
			RateLimitRPS:       100.0,
			RateLimitBurst:     50,
			LogLevel:           "info",
			CertIdentityMode:   "full",
			TribunalID:         "trib-1",
			TribunalURL:        "https://localhost:8443/tribunal/v1/deliberate",
			TribunalBootstrap:  "/etc/g8e/tribunal-bootstrap.json",
			MCPDownstreamURL:   "https://downstream.example.com/mcp",
			A2ADownstreamURL:   "https://downstream.example.com/a2a",
			PublicBaseURL:      "https://demo.g8e.ai",
			AllowedOrigins:     []string{"https://lovable.dev"},
		},
	}

	args, err := pm.BuildReExecArgs(opts)
	require.NoError(t, err)

	// Extract flag names from the re-exec args (skip positional args "gw", "start", and "--follow")
	emittedFlags := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			emittedFlags[strings.TrimPrefix(args[i], "--")] = true
			// Skip the value for non-boolean flags
			// (we don't know which are bools here, so we just skip the next arg
			// if it doesn't start with -- and isn't a positional)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
			}
		}
	}

	// Get all flags defined on gatewayStartCmd()
	startCmd := gatewayStartCmd()
	cobraFlags := startCmd.Flags()

	// Assert every emitted flag exists on the cobra command
	for flagName := range emittedFlags {
		if cobraFlags.Lookup(flagName) == nil {
			t.Errorf("re-exec emits --%s but gatewayStartCmd() has no such flag", flagName)
		}
	}

	// Assert every cobra flag is emitted when all options are populated,
	// except for UI-only flags that are not re-executed.
	skipReExec := map[string]bool{
		"interactive": true, // --interactive is a one-time UI flow, not re-executed
	}
	cobraFlags.VisitAll(func(f *pflag.Flag) {
		if skipReExec[f.Name] {
			return
		}
		if !emittedFlags[f.Name] {
			t.Errorf("gatewayStartCmd() defines --%s but BuildReExecArgs does not emit it", f.Name)
		}
	})
}

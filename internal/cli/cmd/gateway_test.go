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
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBinaryName(t *testing.T) {
	t.Run("returns ./g8e on non-Windows platforms", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping non-Windows test on Windows")
		}
		assert.Equal(t, "./g8e", getBinaryName())
	})

	t.Run("returns ./g8e.exe on Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Skipping Windows test on non-Windows platform")
		}
		assert.Equal(t, "./g8e.exe", getBinaryName())
	})
}

func TestResolveStartConfig(t *testing.T) {
	t.Run("uses CLI flag values when no environment variables set", func(t *testing.T) {
		// Clear environment variables
		unsetEnvVars := []string{"G8E_VAULT_DIR", "G8E_VAULT_KEY", "G8E_VAULT_REQUIRE_UNLOCK"}
		for _, env := range unsetEnvVars {
			original := os.Getenv(env)
			os.Unsetenv(env)
			t.Cleanup(func() {
				if original != "" {
					os.Setenv(env, original)
				}
			})
		}

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"/vault",
			"/vault/key",
			false,
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

		assert.Equal(t, "doctrine", cfg.Posture)
		assert.Equal(t, 8080, cfg.HTTPPort)
		assert.Equal(t, 8443, cfg.HTTPSPort)
		assert.Equal(t, "/data", cfg.DataDir)
		assert.Equal(t, "/pki", cfg.PKIDir)
		assert.Equal(t, "/secrets", cfg.SecretsDir)
		assert.Equal(t, "/vault", cfg.VaultDir)
		assert.Equal(t, "/vault/key", cfg.VaultKeyPath)
		assert.False(t, cfg.VaultRequireUnlock)
		assert.Equal(t, "rp-id", cfg.PasskeyRpID)
		assert.Equal(t, "rp-name", cfg.PasskeyRpName)
		assert.Equal(t, 100.0, cfg.RateLimitRPS)
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

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"", // empty vault dir
			"/vault/key",
			false,
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

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

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"/vault",
			"", // empty vault key
			false,
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

		assert.Equal(t, "/env/vault/key", cfg.VaultKeyPath)
	})

	t.Run("environment variable G8E_VAULT_REQUIRE_UNLOCK=true overrides false CLI flag", func(t *testing.T) {
		original := os.Getenv("G8E_VAULT_REQUIRE_UNLOCK")
		os.Setenv("G8E_VAULT_REQUIRE_UNLOCK", "true")
		t.Cleanup(func() {
			if original != "" {
				os.Setenv("G8E_VAULT_REQUIRE_UNLOCK", original)
			} else {
				os.Unsetenv("G8E_VAULT_REQUIRE_UNLOCK")
			}
		})

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"/vault",
			"/vault/key",
			false, // CLI flag false
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

		assert.True(t, cfg.VaultRequireUnlock)
	})

	t.Run("environment variable G8E_VAULT_REQUIRE_UNLOCK with non-true value does not override", func(t *testing.T) {
		original := os.Getenv("G8E_VAULT_REQUIRE_UNLOCK")
		os.Setenv("G8E_VAULT_REQUIRE_UNLOCK", "false")
		t.Cleanup(func() {
			if original != "" {
				os.Setenv("G8E_VAULT_REQUIRE_UNLOCK", original)
			} else {
				os.Unsetenv("G8E_VAULT_REQUIRE_UNLOCK")
			}
		})

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"/vault",
			"/vault/key",
			false,
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

		assert.False(t, cfg.VaultRequireUnlock)
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

		cfg := resolveStartConfig(
			"doctrine",
			8080,
			8443,
			"/data",
			"/pki",
			"/secrets",
			"/cli/vault", // CLI flag set
			"/vault/key",
			false,
			"rp-id",
			"rp-name",
			100.0,
			50,
			"info",
			"full",
		)

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

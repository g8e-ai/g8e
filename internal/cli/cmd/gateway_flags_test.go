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
// See the License for the specific language and governing permissions and
// limitations under the License.

package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	g8econfig "github.com/g8e-ai/g8e/internal/config"
)

func TestGatewayFlagsToServeConfig_AllFieldsTransferred(t *testing.T) {
	flags := GatewayFlags{
		Posture:           "consensus",
		HTTPPort:          8080,
		HTTPSPort:         8443,
		DataDir:           "/data",
		PKIDir:            "/pki",
		SecretsDir:        "/secrets",
		VaultDir:          "/vault",
		VaultKeyPath:      "/vault/key",
		PasskeyRpID:       "example.com",
		PasskeyRpName:     "Example",
		PasskeyRpOrigins:  []string{"https://example.com"},
		RateLimitRPS:      100.0,
		RateLimitBurst:    50,
		LogLevel:          "debug",
		CertIdentityMode:  "full",
		TribunalID:        "trib-1",
		TribunalURL:       "https://localhost:8443/tribunal/v1/deliberate",
		TribunalBootstrap: "/etc/g8e/tribunal.json",
		MCPDownstreamURL:  "https://downstream/mcp",
		A2ADownstreamURL:  "https://downstream/a2a",
		PublicBaseURL:     "https://demo.example.com",
		AllowedOrigins:    []string{"https://lovable.dev"},
	}

	cfg := gatewayFlagsToServeConfig(flags)

	assert.Equal(t, g8econfig.GatewayPosture("consensus"), cfg.Posture)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 8443, cfg.HTTPSPort)
	assert.Equal(t, "/data", cfg.DataDir)
	assert.Equal(t, "/pki", cfg.PKIDir)
	assert.Equal(t, "/secrets", cfg.SecretsDir)
	assert.Equal(t, "/vault", cfg.VaultDir)
	assert.Equal(t, "/vault/key", cfg.VaultKeyPath)
	assert.Equal(t, "example.com", cfg.PasskeyRpID)
	assert.Equal(t, "Example", cfg.PasskeyRpName)
	assert.Equal(t, []string{"https://example.com"}, cfg.PasskeyRpOrigins)
	assert.InEpsilon(t, 100.0, cfg.RateLimitRPS, 0.01)
	assert.Equal(t, 50, cfg.RateLimitBurst)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "full", cfg.CertIdentityMode)
	assert.Equal(t, "trib-1", cfg.TribunalID)
	assert.Equal(t, "https://localhost:8443/tribunal/v1/deliberate", cfg.TribunalURL)
	assert.Equal(t, "/etc/g8e/tribunal.json", cfg.TribunalBootstrap)
	assert.Equal(t, "https://downstream/mcp", cfg.MCPDownstreamURL)
	assert.Equal(t, "https://downstream/a2a", cfg.A2ADownstreamURL)
	assert.Equal(t, "https://demo.example.com", cfg.PublicBaseURL)
	assert.Equal(t, []string{"https://lovable.dev"}, cfg.AllowedOrigins)
}

func TestGatewayFlagsToServeConfig_EmptyFlags(t *testing.T) {
	flags := GatewayFlags{}

	cfg := gatewayFlagsToServeConfig(flags)

	assert.Equal(t, g8econfig.GatewayPosture(""), cfg.Posture)
	assert.Equal(t, 0, cfg.HTTPPort)
	assert.Equal(t, 0, cfg.HTTPSPort)
	assert.Empty(t, cfg.DataDir)
	assert.Empty(t, cfg.PKIDir)
	assert.Empty(t, cfg.SecretsDir)
	assert.Empty(t, cfg.VaultDir)
	assert.Empty(t, cfg.VaultKeyPath)
	assert.Empty(t, cfg.PasskeyRpID)
	assert.Empty(t, cfg.PasskeyRpName)
	assert.Nil(t, cfg.PasskeyRpOrigins)
	assert.Equal(t, 0.0, cfg.RateLimitRPS)
	assert.Equal(t, 0, cfg.RateLimitBurst)
	assert.Empty(t, cfg.LogLevel)
	assert.Empty(t, cfg.CertIdentityMode)
	assert.Empty(t, cfg.TribunalID)
	assert.Empty(t, cfg.TribunalURL)
	assert.Empty(t, cfg.TribunalBootstrap)
	assert.Empty(t, cfg.MCPDownstreamURL)
	assert.Empty(t, cfg.A2ADownstreamURL)
	assert.Empty(t, cfg.PublicBaseURL)
	assert.Nil(t, cfg.AllowedOrigins)
}

func TestAddGatewayFlags_RegistersAllFlags(t *testing.T) {
	cmd := &cobra.Command{}
	var flags GatewayFlags
	addGatewayFlags(cmd, &flags)

	expectedFlags := []struct {
		name     string
		defValue string
	}{
		{"posture", "doctrine"},
		{"http-port", "0"},
		{"https-port", "0"},
		{"data-dir", ""},
		{"pki-dir", ""},
		{"secrets-dir", ""},
		{"vault-dir", ""},
		{"vault-key", ""},
		{"passkey-rp-id", ""},
		{"passkey-rp-name", ""},
		{"rate-limit-rps", "0"},
		{"rate-limit-burst", "0"},
		{"log", "info"},
		{"cert-mode", ""},
		{"tribunal-id", ""},
		{"tribunal-url", ""},
		{"tribunal-bootstrap", ""},
		{"mcp-downstream-url", ""},
		{"a2a-downstream-url", ""},
		{"public-base-url", ""},
	}

	for _, ef := range expectedFlags {
		flag := cmd.Flags().Lookup(ef.name)
		require.NotNil(t, flag, "flag --%s should be registered", ef.name)
		assert.Equal(t, ef.defValue, flag.DefValue, "flag --%s default should be %s", ef.name, ef.defValue)
	}

	rpOriginFlag := cmd.Flags().Lookup("passkey-rp-origin")
	assert.NotNil(t, rpOriginFlag)

	corsFlag := cmd.Flags().Lookup("cors-origin")
	assert.NotNil(t, corsFlag)
}

func TestResolveGatewayFlags_TribunalEnvOverrides(t *testing.T) {
	envVars := map[string]string{
		"G8E_TRIBUNAL_ID":        "env-trib-id",
		"G8E_TRIBUNAL_URL":       "https://env:8443/tribunal",
		"G8E_TRIBUNAL_BOOTSTRAP": "/env/tribunal.json",
	}

	originalValues := make(map[string]string)
	for key := range envVars {
		originalValues[key] = os.Getenv(key)
		os.Setenv(key, envVars[key])
	}
	t.Cleanup(func() {
		for key, orig := range originalValues {
			if orig != "" {
				os.Setenv(key, orig)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	result := resolveGatewayFlags(GatewayFlags{})

	assert.Equal(t, "env-trib-id", result.TribunalID)
	assert.Equal(t, "https://env:8443/tribunal", result.TribunalURL)
	assert.Equal(t, "/env/tribunal.json", result.TribunalBootstrap)
}

func TestResolveGatewayFlags_TribunalCLITakesPrecedence(t *testing.T) {
	envVars := map[string]string{
		"G8E_TRIBUNAL_ID":        "env-trib-id",
		"G8E_TRIBUNAL_URL":       "https://env:8443/tribunal",
		"G8E_TRIBUNAL_BOOTSTRAP": "/env/tribunal.json",
	}

	originalValues := make(map[string]string)
	for key := range envVars {
		originalValues[key] = os.Getenv(key)
		os.Setenv(key, envVars[key])
	}
	t.Cleanup(func() {
		for key, orig := range originalValues {
			if orig != "" {
				os.Setenv(key, orig)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	result := resolveGatewayFlags(GatewayFlags{
		TribunalID:        "cli-trib-id",
		TribunalURL:       "https://cli:8443/tribunal",
		TribunalBootstrap: "/cli/tribunal.json",
	})

	assert.Equal(t, "cli-trib-id", result.TribunalID)
	assert.Equal(t, "https://cli:8443/tribunal", result.TribunalURL)
	assert.Equal(t, "/cli/tribunal.json", result.TribunalBootstrap)
}

func TestResolveGatewayFlags_NoOverridesWhenEnvUnset(t *testing.T) {
	envKeys := []string{
		"G8E_VAULT_DIR", "G8E_VAULT_KEY",
		"G8E_TRIBUNAL_ID", "G8E_TRIBUNAL_URL", "G8E_TRIBUNAL_BOOTSTRAP",
	}
	originalValues := make(map[string]string)
	for _, key := range envKeys {
		originalValues[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for key, orig := range originalValues {
			if orig != "" {
				os.Setenv(key, orig)
			}
		}
	})

	result := resolveGatewayFlags(GatewayFlags{
		VaultDir:          "/cli/vault",
		VaultKeyPath:      "/cli/key",
		TribunalID:        "cli-id",
		TribunalURL:       "https://cli/tribunal",
		TribunalBootstrap: "/cli/bootstrap.json",
	})

	assert.Equal(t, "/cli/vault", result.VaultDir)
	assert.Equal(t, "/cli/key", result.VaultKeyPath)
	assert.Equal(t, "cli-id", result.TribunalID)
	assert.Equal(t, "https://cli/tribunal", result.TribunalURL)
	assert.Equal(t, "/cli/bootstrap.json", result.TribunalBootstrap)
}

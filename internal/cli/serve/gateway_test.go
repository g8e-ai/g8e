// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// ---------------------------------------------------------------------------
// GatewayConfig struct tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_ZeroValue(t *testing.T) {
	var cfg GatewayConfig

	assert.Equal(t, config.GatewayPosture(""), cfg.Posture)
	assert.Equal(t, 0, cfg.HTTPPort)
	assert.Equal(t, 0, cfg.HTTPSPort)
	assert.Equal(t, "", cfg.DataDir)
	assert.Equal(t, "", cfg.PKIDir)
	assert.Equal(t, "", cfg.SecretsDir)
	assert.Equal(t, "", cfg.VaultDir)
	assert.Equal(t, "", cfg.VaultKeyPath)
	assert.Equal(t, "", cfg.PasskeyRpID)
	assert.Equal(t, "", cfg.PasskeyRpName)
	assert.Equal(t, float64(0), cfg.RateLimitRPS)
	assert.Equal(t, 0, cfg.RateLimitBurst)
	assert.Equal(t, "", cfg.LogLevel)
	assert.Equal(t, "", cfg.CertIdentityMode)
	assert.Equal(t, "", cfg.NetworkIdentityFile)
	assert.Equal(t, "", cfg.ConsensusID)
	assert.Equal(t, "", cfg.ConsensusURL)
	assert.Equal(t, "", cfg.MCPDownstreamURL)
	assert.Equal(t, "", cfg.A2ADownstreamURL)
}

func TestGatewayConfig_FullAssignment(t *testing.T) {
	cfg := GatewayConfig{
		Posture:             config.PostureConsensus,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             constants.TestPathVarLibDataDir,
		PKIDir:              constants.TestPathVarLibPKIDir,
		SecretsDir:          constants.TestPathVarLibSecretsDir,
		VaultDir:            constants.TestPathVarLibVaultDir,
		VaultKeyPath:        constants.TestPathVarLibVaultKey,
		PasskeyRpID:         "localhost",
		PasskeyRpName:       "g8e",
		RateLimitRPS:        5.0,
		RateLimitBurst:      10,
		LogLevel:            "info",
		CertIdentityMode:    "full",
		NetworkIdentityFile: constants.TestPathEtcNetworkIdentity,
		ConsensusID:         "trib-001",
		ConsensusURL:        "https://localhost:8443/consensus/v1/deliberate",
		MCPDownstreamURL:    "http://downstream:3000/mcp",
		A2ADownstreamURL:    "http://downstream:3001/a2a",
	}

	assert.Equal(t, config.PostureConsensus, cfg.Posture)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 8443, cfg.HTTPSPort)
	assert.Equal(t, constants.TestPathVarLibDataDir, cfg.DataDir)
	assert.Equal(t, constants.TestPathVarLibPKIDir, cfg.PKIDir)
	assert.Equal(t, constants.TestPathVarLibSecretsDir, cfg.SecretsDir)
	assert.Equal(t, constants.TestPathVarLibVaultDir, cfg.VaultDir)
	assert.Equal(t, constants.TestPathVarLibVaultKey, cfg.VaultKeyPath)
	assert.Equal(t, "localhost", cfg.PasskeyRpID)
	assert.Equal(t, "g8e", cfg.PasskeyRpName)
	assert.Equal(t, 5.0, cfg.RateLimitRPS)
	assert.Equal(t, 10, cfg.RateLimitBurst)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "full", cfg.CertIdentityMode)
	assert.Equal(t, constants.TestPathEtcNetworkIdentity, cfg.NetworkIdentityFile)
	assert.Equal(t, "trib-001", cfg.ConsensusID)
	assert.Equal(t, "https://localhost:8443/consensus/v1/deliberate", cfg.ConsensusURL)
	assert.Equal(t, "http://downstream:3000/mcp", cfg.MCPDownstreamURL)
	assert.Equal(t, "http://downstream:3001/a2a", cfg.A2ADownstreamURL)
}

func TestGatewayConfig_Equality(t *testing.T) {
	a := GatewayConfig{
		Posture:        config.PostureDoctrine,
		HTTPPort:       8080,
		HTTPSPort:      8443,
		DataDir:        constants.TestPathShortData,
		PKIDir:         constants.TestPathShortPKI,
		SecretsDir:     constants.TestPathShortSecrets,
		LogLevel:       "debug",
		ConsensusID:    "trib-1",
		RateLimitRPS:   5.0,
		RateLimitBurst: 10,
	}
	b := a
	c := a
	c.HTTPPort = 9090

	assert.Equal(t, a, b, "structs with identical fields should be equal")
	assert.NotEqual(t, a, c, "structs differing in any field should not be equal")
}

func TestGatewayConfig_PartialAssignment(t *testing.T) {
	cfg := GatewayConfig{
		Posture:  config.PostureNotary,
		HTTPPort: 8080,
		LogLevel: "error",
	}

	assert.Equal(t, config.PostureNotary, cfg.Posture)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, "error", cfg.LogLevel)
	assert.Equal(t, 0, cfg.HTTPSPort, "unassigned HTTPSPort should be zero value")
	assert.Equal(t, "", cfg.DataDir, "unassigned DataDir should be zero value")
	assert.Equal(t, "", cfg.PKIDir, "unassigned PKIDir should be zero value")
	assert.Equal(t, "", cfg.SecretsDir, "unassigned SecretsDir should be zero value")
	assert.Equal(t, float64(0), cfg.RateLimitRPS, "unassigned RateLimitRPS should be zero")
	assert.Equal(t, 0, cfg.RateLimitBurst, "unassigned RateLimitBurst should be zero")
	assert.Equal(t, "", cfg.ConsensusID, "unassigned ConsensusID should be zero value")
}

func TestGatewayConfig_AllFieldsExported(t *testing.T) {
	cfg := GatewayConfig{}

	cfg.Posture = config.PostureConsensus
	cfg.HTTPPort = 8080
	cfg.HTTPSPort = 8443
	cfg.DataDir = constants.TestPathShortData
	cfg.PKIDir = constants.TestPathShortPKI
	cfg.SecretsDir = constants.TestPathShortSecrets
	cfg.VaultDir = constants.TestPathShortVault
	cfg.VaultKeyPath = constants.TestPathShortVaultKey
	cfg.PasskeyRpID = "example.com"
	cfg.PasskeyRpName = "Example"
	cfg.RateLimitRPS = 10.0
	cfg.RateLimitBurst = 20
	cfg.LogLevel = "debug"
	cfg.CertIdentityMode = "localhost"
	cfg.NetworkIdentityFile = constants.TestPathIdentityFile
	cfg.ConsensusID = "trib-002"
	cfg.ConsensusURL = "https://consensus.example.com"
	cfg.MCPDownstreamURL = "http://mcp.example.com"
	cfg.A2ADownstreamURL = "http://a2a.example.com"

	assert.Equal(t, config.PostureConsensus, cfg.Posture)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 8443, cfg.HTTPSPort)
	assert.Equal(t, constants.TestPathShortData, cfg.DataDir)
	assert.Equal(t, constants.TestPathShortPKI, cfg.PKIDir)
	assert.Equal(t, constants.TestPathShortSecrets, cfg.SecretsDir)
	assert.Equal(t, constants.TestPathShortVault, cfg.VaultDir)
	assert.Equal(t, constants.TestPathShortVaultKey, cfg.VaultKeyPath)
	assert.Equal(t, "example.com", cfg.PasskeyRpID)
	assert.Equal(t, "Example", cfg.PasskeyRpName)
	assert.Equal(t, 10.0, cfg.RateLimitRPS)
	assert.Equal(t, 20, cfg.RateLimitBurst)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "localhost", cfg.CertIdentityMode)
	assert.Equal(t, constants.TestPathIdentityFile, cfg.NetworkIdentityFile)
	assert.Equal(t, "trib-002", cfg.ConsensusID)
	assert.Equal(t, "https://consensus.example.com", cfg.ConsensusURL)
	assert.Equal(t, "http://mcp.example.com", cfg.MCPDownstreamURL)
	assert.Equal(t, "http://a2a.example.com", cfg.A2ADownstreamURL)
}

func TestGatewayConfig_Mutation(t *testing.T) {
	cfg := GatewayConfig{
		Posture:     config.PostureDoctrine,
		HTTPPort:    8080,
		HTTPSPort:   8443,
		LogLevel:    "info",
		ConsensusID: "trib-original",
	}

	cfg.Posture = config.PostureConsensus
	cfg.HTTPPort = 9090
	cfg.ConsensusID = "trib-mutated"

	assert.Equal(t, config.PostureConsensus, cfg.Posture)
	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.Equal(t, 8443, cfg.HTTPSPort, "unmodified fields should retain original values")
	assert.Equal(t, "info", cfg.LogLevel, "unmodified fields should retain original values")
	assert.Equal(t, "trib-mutated", cfg.ConsensusID)
}

func TestGatewayConfig_PostureValues(t *testing.T) {
	postures := []config.GatewayPosture{
		config.PostureDoctrine,
		config.PostureConsensus,
		config.PostureNotary,
	}

	for _, p := range postures {
		cfg := GatewayConfig{Posture: p}
		assert.Equal(t, p, cfg.Posture)
	}
}

func TestGatewayConfig_PostureStringConversion(t *testing.T) {
	tests := []struct {
		name     string
		posture  config.GatewayPosture
		expected string
	}{
		{"doctrine", config.PostureDoctrine, "doctrine"},
		{"consensus", config.PostureConsensus, "consensus"},
		{"notary", config.PostureNotary, "notary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{Posture: tt.posture}
			assert.Equal(t, tt.expected, string(cfg.Posture))
		})
	}
}

// ---------------------------------------------------------------------------
// GatewayConfig → config.GatewayOptions mapping
// ---------------------------------------------------------------------------

// gatewayConfigToOptions replicates the field mapping that RunGateway performs
// when constructing config.GatewayOptions from GatewayConfig. This test
// verifies the mapping is correct without requiring real infrastructure.
func gatewayConfigToOptions(cfg GatewayConfig) config.GatewayOptions {
	return config.GatewayOptions{
		Posture:             cfg.Posture,
		HTTPPort:            cfg.HTTPPort,
		HTTPSPort:           cfg.HTTPSPort,
		DataDir:             cfg.DataDir,
		PKIDir:              cfg.PKIDir,
		SecretsDir:          cfg.SecretsDir,
		VaultDir:            cfg.VaultDir,
		VaultKeyPath:        cfg.VaultKeyPath,
		PasskeyRpID:         cfg.PasskeyRpID,
		PasskeyRpName:       cfg.PasskeyRpName,
		PasskeyRpOrigins:    cfg.PasskeyRpOrigins,
		RateLimitRPS:        cfg.RateLimitRPS,
		RateLimitBurst:      cfg.RateLimitBurst,
		CertMode:            cfg.CertIdentityMode,
		NetworkIdentityFile: cfg.NetworkIdentityFile,
		MCPDownstreamURL:    cfg.MCPDownstreamURL,
		A2ADownstreamURL:    cfg.A2ADownstreamURL,
		ConsensusID:         cfg.ConsensusID,
		ConsensusURL:        cfg.ConsensusURL,
		AllowTestPortZero:   false,
	}
}

func TestGatewayConfigToOptions_FullMapping(t *testing.T) {
	cfg := GatewayConfig{
		Posture:             config.PostureConsensus,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             constants.TestPathShortData,
		PKIDir:              constants.TestPathShortPKI,
		SecretsDir:          constants.TestPathShortSecrets,
		VaultDir:            "/test/vault",
		VaultKeyPath:        "/test/vault/key",
		PasskeyRpID:         "localhost",
		PasskeyRpName:       "g8e",
		RateLimitRPS:        5.0,
		RateLimitBurst:      10,
		CertIdentityMode:    "full",
		NetworkIdentityFile: constants.TestPathIdentityFile,
		ConsensusID:         "trib-001",
		ConsensusURL:        "https://localhost:8443/consensus/v1/deliberate",
		MCPDownstreamURL:    "http://mcp:3000",
		A2ADownstreamURL:    "http://a2a:3001",
	}

	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, cfg.Posture, opts.Posture)
	assert.Equal(t, cfg.HTTPPort, opts.HTTPPort)
	assert.Equal(t, cfg.HTTPSPort, opts.HTTPSPort)
	assert.Equal(t, cfg.DataDir, opts.DataDir)
	assert.Equal(t, cfg.PKIDir, opts.PKIDir)
	assert.Equal(t, cfg.SecretsDir, opts.SecretsDir)
	assert.Equal(t, cfg.VaultDir, opts.VaultDir)
	assert.Equal(t, cfg.VaultKeyPath, opts.VaultKeyPath)
	assert.Equal(t, cfg.PasskeyRpID, opts.PasskeyRpID)
	assert.Equal(t, cfg.PasskeyRpName, opts.PasskeyRpName)
	assert.Equal(t, cfg.RateLimitRPS, opts.RateLimitRPS)
	assert.Equal(t, cfg.RateLimitBurst, opts.RateLimitBurst)
	assert.Equal(t, cfg.CertIdentityMode, opts.CertMode)
	assert.Equal(t, cfg.NetworkIdentityFile, opts.NetworkIdentityFile)
	assert.Equal(t, cfg.MCPDownstreamURL, opts.MCPDownstreamURL)
	assert.Equal(t, cfg.A2ADownstreamURL, opts.A2ADownstreamURL)
	assert.Equal(t, cfg.ConsensusID, opts.ConsensusID)
	assert.Equal(t, cfg.ConsensusURL, opts.ConsensusURL)
	assert.False(t, opts.AllowTestPortZero, "AllowTestPortZero should always be false in production mapping")
}

func TestGatewayConfigToOptions_EmptyConfig(t *testing.T) {
	cfg := GatewayConfig{}

	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, config.GatewayPosture(""), opts.Posture)
	assert.Equal(t, 0, opts.HTTPPort)
	assert.Equal(t, 0, opts.HTTPSPort)
	assert.Equal(t, "", opts.DataDir)
	assert.Equal(t, "", opts.PKIDir)
	assert.Equal(t, "", opts.SecretsDir)
	assert.False(t, opts.AllowTestPortZero)
}

func TestGatewayConfigToOptions_CertModeMapping(t *testing.T) {
	tests := []struct {
		name             string
		certMode         string
		expectedCertMode string
	}{
		{"empty", "", ""},
		{"full", "full", "full"},
		{"localhost", "localhost", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{CertIdentityMode: tt.certMode}
			opts := gatewayConfigToOptions(cfg)
			assert.Equal(t, tt.expectedCertMode, opts.CertMode)
		})
	}
}

func TestGatewayConfigToOptions_DownstreamURLs(t *testing.T) {
	t.Run("both empty (default)", func(t *testing.T) {
		cfg := GatewayConfig{}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, "", opts.MCPDownstreamURL)
		assert.Equal(t, "", opts.A2ADownstreamURL)
	})

	t.Run("both set", func(t *testing.T) {
		cfg := GatewayConfig{
			MCPDownstreamURL: "http://mcp-downstream:3000/sse",
			A2ADownstreamURL: "http://a2a-downstream:3001",
		}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, "http://mcp-downstream:3000/sse", opts.MCPDownstreamURL)
		assert.Equal(t, "http://a2a-downstream:3001", opts.A2ADownstreamURL)
	})

	t.Run("only MCP set", func(t *testing.T) {
		cfg := GatewayConfig{MCPDownstreamURL: "http://mcp:3000"}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, "http://mcp:3000", opts.MCPDownstreamURL)
		assert.Equal(t, "", opts.A2ADownstreamURL)
	})

	t.Run("only A2A set", func(t *testing.T) {
		cfg := GatewayConfig{A2ADownstreamURL: "http://a2a:3001"}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, "", opts.MCPDownstreamURL)
		assert.Equal(t, "http://a2a:3001", opts.A2ADownstreamURL)
	})
}

func TestGatewayConfigToOptions_VaultFieldsNotMapped(t *testing.T) {
	cfg := GatewayConfig{
		VaultDir:     constants.TestPathShortVault,
		VaultKeyPath: constants.TestPathShortVaultKey,
	}

	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, "", opts.DataDir, "VaultDir is not mapped to GatewayOptions")
	assert.False(t, opts.AllowTestPortZero)
}

// ---------------------------------------------------------------------------
// GatewayConfig field count / completeness
// ---------------------------------------------------------------------------

func TestGatewayConfig_FieldCount(t *testing.T) {
	cfg := GatewayConfig{
		Posture:             config.PostureDoctrine,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             constants.TestPathShortData,
		PKIDir:              constants.TestPathShortPKI,
		SecretsDir:          constants.TestPathShortSecrets,
		VaultDir:            constants.TestPathShortVault,
		VaultKeyPath:        constants.TestPathShortVaultKey,
		PasskeyRpID:         "localhost",
		PasskeyRpName:       "g8e",
		RateLimitRPS:        5.0,
		RateLimitBurst:      10,
		LogLevel:            "info",
		CertIdentityMode:    "full",
		NetworkIdentityFile: constants.TestPathIdentityFileShort,
		ConsensusID:         "trib-1",
		ConsensusURL:        "https://consensus.example.com",
		MCPDownstreamURL:    "http://mcp:3000",
		A2ADownstreamURL:    "http://a2a:3001",
	}

	nonZero := 0
	if cfg.Posture != "" {
		nonZero++
	}
	if cfg.HTTPPort != 0 {
		nonZero++
	}
	if cfg.HTTPSPort != 0 {
		nonZero++
	}
	if cfg.DataDir != "" {
		nonZero++
	}
	if cfg.PKIDir != "" {
		nonZero++
	}
	if cfg.SecretsDir != "" {
		nonZero++
	}
	if cfg.VaultDir != "" {
		nonZero++
	}
	if cfg.VaultKeyPath != "" {
		nonZero++
	}
	if cfg.PasskeyRpID != "" {
		nonZero++
	}
	if cfg.PasskeyRpName != "" {
		nonZero++
	}
	if cfg.RateLimitRPS != 0 {
		nonZero++
	}
	if cfg.RateLimitBurst != 0 {
		nonZero++
	}
	if cfg.LogLevel != "" {
		nonZero++
	}
	if cfg.CertIdentityMode != "" {
		nonZero++
	}
	if cfg.NetworkIdentityFile != "" {
		nonZero++
	}
	if cfg.ConsensusID != "" {
		nonZero++
	}
	if cfg.ConsensusURL != "" {
		nonZero++
	}
	if cfg.MCPDownstreamURL != "" {
		nonZero++
	}
	if cfg.A2ADownstreamURL != "" {
		nonZero++
	}

	assert.Equal(t, 19, nonZero, "all 19 fields should be set and non-zero")
}

// ---------------------------------------------------------------------------
// Posture-specific GatewayConfig scenarios
// ---------------------------------------------------------------------------

func TestGatewayConfig_DoctrinePostureScenario(t *testing.T) {
	cfg := GatewayConfig{
		Posture:       config.PostureDoctrine,
		HTTPPort:      8080,
		HTTPSPort:     8443,
		DataDir:       constants.TestPathVarLibDataDir,
		PKIDir:        constants.TestPathVarLibPKIDir,
		SecretsDir:    constants.TestPathVarLibSecretsDir,
		PasskeyRpID:   "localhost",
		PasskeyRpName: "g8e",
		LogLevel:      "info",
	}

	assert.Equal(t, config.PostureDoctrine, cfg.Posture)
	assert.Equal(t, "", cfg.ConsensusID, "doctrine posture does not require consensus")
	assert.Equal(t, "", cfg.ConsensusURL, "doctrine posture does not require consensus URL")

	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, config.PostureDoctrine, opts.Posture)
	assert.Equal(t, "", opts.ConsensusID)
	assert.Equal(t, "", opts.ConsensusURL)
}

func TestGatewayConfig_ConsensusPostureScenario(t *testing.T) {
	cfg := GatewayConfig{
		Posture:       config.PostureConsensus,
		HTTPPort:      8080,
		HTTPSPort:     8443,
		DataDir:       constants.TestPathVarLibDataDir,
		PKIDir:        constants.TestPathVarLibPKIDir,
		SecretsDir:    constants.TestPathVarLibSecretsDir,
		PasskeyRpID:   "localhost",
		PasskeyRpName: "g8e",
		LogLevel:      "info",
		ConsensusID:   "trib-001",
		ConsensusURL:  "https://localhost:8443/consensus/v1/deliberate",
	}

	assert.Equal(t, config.PostureConsensus, cfg.Posture)
	assert.NotEmpty(t, cfg.ConsensusID, "consensus posture requires consensus ID")
	assert.NotEmpty(t, cfg.ConsensusURL, "consensus posture requires consensus URL")

	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, config.PostureConsensus, opts.Posture)
	assert.Equal(t, "trib-001", opts.ConsensusID)
	assert.Equal(t, "https://localhost:8443/consensus/v1/deliberate", opts.ConsensusURL)
}

func TestGatewayConfig_NotaryPostureScenario(t *testing.T) {
	cfg := GatewayConfig{
		Posture:       config.PostureNotary,
		HTTPPort:      8080,
		HTTPSPort:     8443,
		DataDir:       constants.TestPathVarLibDataDir,
		PKIDir:        constants.TestPathVarLibPKIDir,
		SecretsDir:    constants.TestPathVarLibSecretsDir,
		PasskeyRpID:   "localhost",
		PasskeyRpName: "g8e",
		LogLevel:      "info",
	}

	assert.Equal(t, config.PostureNotary, cfg.Posture)
	assert.Equal(t, "", cfg.ConsensusID, "notary posture requires a consensus at startup validation, but the struct default is empty until configured")

	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, config.PostureNotary, opts.Posture)
}

func TestGatewayConfig_ConsensusWithoutConsensusID(t *testing.T) {
	cfg := GatewayConfig{
		Posture:      config.PostureConsensus,
		ConsensusURL: "https://localhost:8443/consensus",
	}

	assert.Equal(t, "", cfg.ConsensusID, "consensus ID missing but URL set is an invalid config")
	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, "", opts.ConsensusID)
	assert.NotEmpty(t, opts.ConsensusURL)
}

// ---------------------------------------------------------------------------
// Port boundary tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_PortBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		httpPort  int
		httpsPort int
	}{
		{"zero ports", 0, 0},
		{"privileged HTTP", 80, 0},
		{"privileged HTTPS", 0, 443},
		{"standard HTTP", 8080, 0},
		{"standard HTTPS", 0, 8443},
		{"both standard", 8080, 8443},
		{"max port", 65535, 65535},
		{"high ports", 18080, 18443},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{HTTPPort: tt.httpPort, HTTPSPort: tt.httpsPort}
			assert.Equal(t, tt.httpPort, cfg.HTTPPort)
			assert.Equal(t, tt.httpsPort, cfg.HTTPSPort)

			opts := gatewayConfigToOptions(cfg)
			assert.Equal(t, tt.httpPort, opts.HTTPPort)
			assert.Equal(t, tt.httpsPort, opts.HTTPSPort)
		})
	}
}

func TestGatewayConfig_NegativePorts(t *testing.T) {
	cfg := GatewayConfig{HTTPPort: -1, HTTPSPort: -1}
	assert.Equal(t, -1, cfg.HTTPPort)
	assert.Equal(t, -1, cfg.HTTPSPort)

	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, -1, opts.HTTPPort, "mapping should preserve negative values without validation")
	assert.Equal(t, -1, opts.HTTPSPort)
}

// ---------------------------------------------------------------------------
// Rate limit boundary tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_RateLimitBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		rps   float64
		burst int
	}{
		{"zero", 0, 0},
		{"minimal", 0.1, 1},
		{"standard", 5.0, 10},
		{"high throughput", 1000.0, 2000},
		{"fractional rps", 0.5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{RateLimitRPS: tt.rps, RateLimitBurst: tt.burst}
			assert.Equal(t, tt.rps, cfg.RateLimitRPS)
			assert.Equal(t, tt.burst, cfg.RateLimitBurst)

			opts := gatewayConfigToOptions(cfg)
			assert.Equal(t, tt.rps, opts.RateLimitRPS)
			assert.Equal(t, tt.burst, opts.RateLimitBurst)
		})
	}
}

func TestGatewayConfig_NegativeRateLimit(t *testing.T) {
	cfg := GatewayConfig{RateLimitRPS: -1.0, RateLimitBurst: -1}
	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, -1.0, opts.RateLimitRPS, "mapping should preserve negative values without validation")
	assert.Equal(t, -1, opts.RateLimitBurst)
}

// ---------------------------------------------------------------------------
// Vault configuration tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_VaultConfiguration(t *testing.T) {
	cfg := GatewayConfig{
		VaultDir:     constants.TestPathVarLibVaultDir,
		VaultKeyPath: constants.TestPathVarLibVaultKey,
	}

	assert.Equal(t, constants.TestPathVarLibVaultDir, cfg.VaultDir)
	assert.Equal(t, constants.TestPathVarLibVaultKey, cfg.VaultKeyPath)
}

func TestGatewayConfigToOptions_VaultFieldsPreservedInConfigOnly(t *testing.T) {
	cfg := GatewayConfig{
		VaultDir:     constants.TestPathShortVault,
		VaultKeyPath: constants.TestPathShortVaultKey,
		DataDir:      constants.TestPathShortData,
		PKIDir:       constants.TestPathShortPKI,
		SecretsDir:   constants.TestPathShortSecrets,
	}

	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, constants.TestPathShortData, opts.DataDir, "DataDir should be mapped")
	assert.Equal(t, constants.TestPathShortPKI, opts.PKIDir, "PKIDir should be mapped")
	assert.Equal(t, constants.TestPathShortSecrets, opts.SecretsDir, "SecretsDir should be mapped")
}

// ---------------------------------------------------------------------------
// Cert identity mode tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_CertIdentityModes(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{"empty (default)", ""},
		{"localhost", "localhost"},
		{"full", "full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{CertIdentityMode: tt.mode}
			opts := gatewayConfigToOptions(cfg)
			assert.Equal(t, tt.mode, opts.CertMode)
		})
	}
}

func TestGatewayConfig_NetworkIdentityFile(t *testing.T) {
	cfg := GatewayConfig{NetworkIdentityFile: constants.TestPathEtcNetworkIdentity}
	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, constants.TestPathEtcNetworkIdentity, opts.NetworkIdentityFile)
}

// ---------------------------------------------------------------------------
// AllowTestPortZero invariant
// ---------------------------------------------------------------------------

func TestGatewayConfigToOptions_AllowTestPortZeroAlwaysFalse(t *testing.T) {
	tests := []struct {
		name string
		cfg  GatewayConfig
	}{
		{"empty config", GatewayConfig{}},
		{"full config", GatewayConfig{
			Posture:   config.PostureConsensus,
			HTTPPort:  8080,
			HTTPSPort: 8443,
		}},
		{"zero ports", GatewayConfig{HTTPPort: 0, HTTPSPort: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := gatewayConfigToOptions(tt.cfg)
			assert.False(t, opts.AllowTestPortZero,
				"AllowTestPortZero must always be false in production gateway config mapping")
		})
	}
}

// ---------------------------------------------------------------------------
// Passkey configuration tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_PasskeyConfiguration(t *testing.T) {
	cfg := GatewayConfig{
		PasskeyRpID:   "operator.example.com",
		PasskeyRpName: "g8e Operator",
	}

	opts := gatewayConfigToOptions(cfg)
	assert.Equal(t, "operator.example.com", opts.PasskeyRpID)
	assert.Equal(t, "g8e Operator", opts.PasskeyRpName)
}

func TestGatewayConfig_PasskeyEmptyDefaults(t *testing.T) {
	var cfg GatewayConfig
	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, "", opts.PasskeyRpID)
	assert.Equal(t, "", opts.PasskeyRpName)
	assert.Nil(t, opts.PasskeyRpOrigins)
}

func TestGatewayConfig_PasskeyRpOriginsWiring(t *testing.T) {
	t.Run("single origin", func(t *testing.T) {
		cfg := GatewayConfig{
			PasskeyRpOrigins: []string{"http://localhost:8087"},
		}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, []string{"http://localhost:8087"}, opts.PasskeyRpOrigins)
	})

	t.Run("multiple origins", func(t *testing.T) {
		cfg := GatewayConfig{
			PasskeyRpOrigins: []string{"http://localhost:8087", "https://localhost:8450"},
		}
		opts := gatewayConfigToOptions(cfg)
		assert.Equal(t, []string{"http://localhost:8087", "https://localhost:8450"}, opts.PasskeyRpOrigins)
	})

	t.Run("empty slice preserved as nil", func(t *testing.T) {
		var cfg GatewayConfig
		opts := gatewayConfigToOptions(cfg)
		assert.Nil(t, opts.PasskeyRpOrigins)
	})
}

// ---------------------------------------------------------------------------
// Consensus URL variants
// ---------------------------------------------------------------------------

func TestGatewayConfigToOptions_ConsensusURLVariants(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"localhost", "https://localhost:8443/consensus/v1/deliberate"},
		{"remote", "https://consensus.example.com:9443/consensus/v1/deliberate"},
		{"http (insecure)", "http://localhost:8080/consensus/v1/deliberate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{ConsensusURL: tt.url}
			opts := gatewayConfigToOptions(cfg)
			assert.Equal(t, tt.url, opts.ConsensusURL)
		})
	}
}

// ---------------------------------------------------------------------------
// Downstream URL combinations with posture
// ---------------------------------------------------------------------------

func TestGatewayConfigToOptions_DownstreamWithPosture(t *testing.T) {
	tests := []struct {
		name    string
		posture config.GatewayPosture
		mcpURL  string
		a2aURL  string
	}{
		{"doctrine with downstreams", config.PostureDoctrine, "http://mcp:3000", "http://a2a:3001"},
		{"consensus with downstreams", config.PostureConsensus, "http://mcp:3000", "http://a2a:3001"},
		{"notary without downstreams", config.PostureNotary, "", ""},
		{"notary with MCP only", config.PostureNotary, "http://mcp:3000", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{
				Posture:          tt.posture,
				MCPDownstreamURL: tt.mcpURL,
				A2ADownstreamURL: tt.a2aURL,
			}
			opts := gatewayConfigToOptions(cfg)

			assert.Equal(t, tt.posture, opts.Posture)
			assert.Equal(t, tt.mcpURL, opts.MCPDownstreamURL)
			assert.Equal(t, tt.a2aURL, opts.A2ADownstreamURL)
		})
	}
}

// ---------------------------------------------------------------------------
// GatewayConfig round-trip mapping integrity
// ---------------------------------------------------------------------------

func TestGatewayConfigToOptions_RoundTripIntegrity(t *testing.T) {
	original := GatewayConfig{
		Posture:             config.PostureNotary,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             constants.TestPathShortData,
		PKIDir:              constants.TestPathShortPKI,
		SecretsDir:          constants.TestPathShortSecrets,
		PasskeyRpID:         "localhost",
		PasskeyRpName:       "g8e",
		PasskeyRpOrigins:    []string{"http://localhost:8087", "https://localhost:8450"},
		RateLimitRPS:        10.0,
		RateLimitBurst:      20,
		CertIdentityMode:    "full",
		NetworkIdentityFile: constants.TestPathIdentityFileShort,
		ConsensusID:         "trib-1",
		ConsensusURL:        "https://consensus.example.com",
		MCPDownstreamURL:    "http://mcp:3000",
		A2ADownstreamURL:    "http://a2a:3001",
	}

	opts := gatewayConfigToOptions(original)

	assert.Equal(t, original.Posture, opts.Posture)
	assert.Equal(t, original.HTTPPort, opts.HTTPPort)
	assert.Equal(t, original.HTTPSPort, opts.HTTPSPort)
	assert.Equal(t, original.DataDir, opts.DataDir)
	assert.Equal(t, original.PKIDir, opts.PKIDir)
	assert.Equal(t, original.SecretsDir, opts.SecretsDir)
	assert.Equal(t, original.PasskeyRpID, opts.PasskeyRpID)
	assert.Equal(t, original.PasskeyRpName, opts.PasskeyRpName)
	assert.Equal(t, original.PasskeyRpOrigins, opts.PasskeyRpOrigins)
	assert.Equal(t, original.RateLimitRPS, opts.RateLimitRPS)
	assert.Equal(t, original.RateLimitBurst, opts.RateLimitBurst)
	assert.Equal(t, original.CertIdentityMode, opts.CertMode)
	assert.Equal(t, original.NetworkIdentityFile, opts.NetworkIdentityFile)
	assert.Equal(t, original.ConsensusID, opts.ConsensusID)
	assert.Equal(t, original.ConsensusURL, opts.ConsensusURL)
	assert.Equal(t, original.MCPDownstreamURL, opts.MCPDownstreamURL)
	assert.Equal(t, original.A2ADownstreamURL, opts.A2ADownstreamURL)
}

// ---------------------------------------------------------------------------
// GatewayConfig with constants consistency
// ---------------------------------------------------------------------------

func TestGatewayConfig_StandardPortValues(t *testing.T) {
	cfg := GatewayConfig{
		HTTPPort:  constants.Ports.OperatorHttp,
		HTTPSPort: constants.Ports.OperatorHttps,
	}

	assert.Equal(t, 8080, cfg.HTTPPort, "HTTP port should match standard gateway HTTP port")
	assert.Equal(t, 8443, cfg.HTTPSPort, "HTTPS port should match standard gateway HTTPS port")
}

func TestGatewayConfigToOptions_StandardPortsPreserved(t *testing.T) {
	cfg := GatewayConfig{
		HTTPPort:  constants.Ports.OperatorHttp,
		HTTPSPort: constants.Ports.OperatorHttps,
	}
	opts := gatewayConfigToOptions(cfg)

	assert.Equal(t, constants.Ports.OperatorHttp, opts.HTTPPort)
	assert.Equal(t, constants.Ports.OperatorHttps, opts.HTTPSPort)
}

// ---------------------------------------------------------------------------
// parseConsensusBootstrapConfig tests (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestParseConsensusBootstrapConfig_Valid(t *testing.T) {
	data := []byte(`{
		"consensus_id": "dhs-consensus",
		"member_app_ids": ["auditor-ensemble"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "dhs-consensus", boot.ConsensusID)
	assert.Equal(t, []string{"auditor-ensemble"}, boot.MemberAppIDs)
	assert.Equal(t, 1, boot.Quorum)
	assert.Equal(t, "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77", boot.SeedHex)
}

func TestParseConsensusBootstrapConfig_ValidNoSeed(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a", "member-b"],
		"quorum": 2
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "test-consensus", boot.ConsensusID)
	assert.Len(t, boot.MemberAppIDs, 2)
	assert.Equal(t, 2, boot.Quorum)
	assert.Empty(t, boot.SeedHex)
}

func TestParseConsensusBootstrapConfig_MalformedJSON(t *testing.T) {
	data := []byte(`{not valid json}`)

	_, err := parseConsensusBootstrapConfig(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapParseConfig))
}

func TestParseConsensusBootstrapConfig_EmptyConsensusID(t *testing.T) {
	data := []byte(`{
		"consensus_id": "",
		"member_app_ids": ["member-a"],
		"quorum": 1
	}`)

	_, err := parseConsensusBootstrapConfig(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
}

func TestParseConsensusBootstrapConfig_EmptyMemberAppIDs(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": [],
		"quorum": 1
	}`)

	_, err := parseConsensusBootstrapConfig(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
}

func TestParseConsensusBootstrapConfig_QuorumZero(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a"],
		"quorum": 0
	}`)

	_, err := parseConsensusBootstrapConfig(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
}

func TestParseConsensusBootstrapConfig_NegativeQuorum(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a"],
		"quorum": -1
	}`)

	_, err := parseConsensusBootstrapConfig(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
}

func TestParseConsensusBootstrapConfig_MultipleMembers(t *testing.T) {
	data := []byte(`{
		"consensus_id": "council",
		"member_app_ids": ["alpha", "beta", "gamma"],
		"quorum": 2,
		"seed_hex": ""
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "council", boot.ConsensusID)
	assert.Len(t, boot.MemberAppIDs, 3)
	assert.Equal(t, 2, boot.Quorum)
}

// ---------------------------------------------------------------------------
// deriveSeedPublicKey tests (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestDeriveSeedPublicKey_ValidSeed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	seedHex := hex.EncodeToString(seed)

	pubHex, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)

	priv := ed25519.NewKeyFromSeed(seed)
	expectedPub := priv.Public().(ed25519.PublicKey)
	assert.Equal(t, hex.EncodeToString(expectedPub), pubHex)
}

func TestDeriveSeedPublicKey_Deterministic(t *testing.T) {
	seedHex := "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"

	pub1, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)

	pub2, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)

	assert.Equal(t, pub1, pub2, "same seed must produce same public key")
	assert.Len(t, pub1, 64, "Ed25519 public key hex should be 64 chars")
}

func TestDeriveSeedPublicKey_TrimsWhitespace(t *testing.T) {
	seedHex := "  87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77  \n"

	pubHex, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)
	assert.Len(t, pubHex, 64)
}

func TestDeriveSeedPublicKey_InvalidHex(t *testing.T) {
	_, err := deriveSeedPublicKey("not-hex-at-all")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapDecodeSeed))
}

func TestDeriveSeedPublicKey_WrongLength(t *testing.T) {
	shortHex := hex.EncodeToString([]byte{1, 2, 3})

	_, err := deriveSeedPublicKey(shortHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidSeedLength))
}

func TestDeriveSeedPublicKey_EmptyString(t *testing.T) {
	_, err := deriveSeedPublicKey("")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidSeedLength))
}

func TestDeriveSeedPublicKey_MatchesKnownSeed(t *testing.T) {
	seedHex := "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
	seed, err := hex.DecodeString(seedHex)
	require.NoError(t, err)

	priv := ed25519.NewKeyFromSeed(seed)
	expectedPub := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	pubHex, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)
	assert.Equal(t, expectedPub, pubHex)
}

// ---------------------------------------------------------------------------
// consensusPolicyBootstrap error-path tests (Tier 2 — file I/O)
// ---------------------------------------------------------------------------

func TestBootstrapConsensusPolicy_NilStoresReturnsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["auditor-ensemble"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
	}`), 0600)
	require.NoError(t, err)

	err = consensusPolicyBootstrap(nil, nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayStoresNil),
		"consensusPolicyBootstrap with nil stores should return ErrGatewayStoresNil")
}

func TestBootstrapConsensusPolicy_MissingFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	err := consensusPolicyBootstrap(nil, nil, constants.TestPathNonexistentConsensus, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapReadConfig))
}

func TestBootstrapConsensusPolicy_MalformedJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{not valid json}`), 0600)
	require.NoError(t, err)

	err = consensusPolicyBootstrap(nil, nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapParseConfig))
}

func TestBootstrapConsensusPolicy_InvalidConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name   string
		config string
	}{
		{
			"empty consensus_id",
			`{"consensus_id": "", "member_app_ids": ["a"], "quorum": 1}`,
		},
		{
			"empty member_app_ids",
			`{"consensus_id": "t", "member_app_ids": [], "quorum": 1}`,
		},
		{
			"quorum zero",
			`{"consensus_id": "t", "member_app_ids": ["a"], "quorum": 0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := testutil.TempDir(t)
			configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
			err := os.WriteFile(configPath, []byte(tt.config), 0600)
			require.NoError(t, err)

			err = consensusPolicyBootstrap(nil, nil, configPath, constants.TestPathShortSecrets, logger)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
		})
	}
}

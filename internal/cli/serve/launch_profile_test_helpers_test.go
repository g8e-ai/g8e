// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
)

// validTestGatewayConfig returns a GatewayConfig with every field populated
// to non-default values so round-trip tests can detect field loss. This helper
// is shared by both Tier 1 (pure validation) and Tier 2 (file-based) test
// files, so it must not use file I/O or carry a build tag.
func validTestGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Posture:             g8econfig.PostureConsensus,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             "/data",
		PKIDir:              "/pki",
		SecretsDir:          "/secrets",
		VaultDir:            "/vault",
		VaultKeyPath:        "/vault/key",
		PasskeyRpID:         "your-app.lovable.app",
		PasskeyRpName:       "g8e",
		PasskeyRpOrigins:    []string{"https://your-app.lovable.app"},
		RateLimitRPS:        5.0,
		RateLimitBurst:      10,
		LogLevel:            "info",
		CertIdentityMode:    "full",
		NetworkIdentityFile: "/tmp/ephemeral-identity.json",
		ConsensusID:         "trib-001",
		ConsensusURL:        "https://localhost:8443/consensus/v1/deliberate",
		ConsensusBootstrap:  "/etc/g8e/consensus-bootstrap.json",
		MCPDownstreamURL:    "http://downstream:3000/mcp",
		A2ADownstreamURL:    "http://downstream:3001/a2a",
		PublicBaseURL:       "https://your-app.lovable.app",
		AllowedOrigins:      []string{"https://your-app.lovable.app"},
		DoctrineDir:         "/etc/g8e/doctrine",
	}
}

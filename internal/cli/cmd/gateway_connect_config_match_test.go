// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
)

// TestDeriveConnectConfig_AppliesBrowserFieldsToBaseConfig verifies that
// deriveConnectConfig applies the frontend origin, RP ID, and RP name to a
// copy of the base config while preserving all other fields.
func TestDeriveConnectConfig_AppliesBrowserFieldsToBaseConfig(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	base := serve.GatewayConfig{
		Posture:          g8econfig.PostureConsensus,
		HTTPPort:         8080,
		HTTPSPort:        8443,
		LogLevel:         "info",
		CertIdentityMode: "full",
		ConsensusID:      "trib-001",
		ConsensusURL:     "https://localhost:8443/consensus/v1/deliberate",
		RateLimitRPS:     5.0,
		RateLimitBurst:   10,
		DoctrineDir:      "/etc/g8e/doctrine",
	}

	result := deriveConnectConfig(origin, "your-app.lovable.app", "g8e", base)

	// Browser fields are applied.
	assert.Equal(t, []string{"https://your-app.lovable.app"}, result.AllowedOrigins)
	assert.Equal(t, []string{"https://your-app.lovable.app"}, result.PasskeyRpOrigins)
	assert.Equal(t, "your-app.lovable.app", result.PasskeyRpID)
	assert.Equal(t, "g8e", result.PasskeyRpName)

	// Non-browser fields are preserved from the base.
	assert.Equal(t, g8econfig.PostureConsensus, result.Posture)
	assert.Equal(t, 8080, result.HTTPPort)
	assert.Equal(t, 8443, result.HTTPSPort)
	assert.Equal(t, "info", result.LogLevel)
	assert.Equal(t, "full", result.CertIdentityMode)
	assert.Equal(t, "trib-001", result.ConsensusID)
	assert.Equal(t, "https://localhost:8443/consensus/v1/deliberate", result.ConsensusURL)
	assert.Equal(t, 5.0, result.RateLimitRPS)
	assert.Equal(t, 10, result.RateLimitBurst)
	assert.Equal(t, "/etc/g8e/doctrine", result.DoctrineDir)

	// The base config is not mutated.
	assert.Nil(t, base.AllowedOrigins)
	assert.Nil(t, base.PasskeyRpOrigins)
	assert.Equal(t, "", base.PasskeyRpID)
}

// TestDeriveConnectConfig_OverridesExistingBrowserFields verifies that
// deriveConnectConfig replaces existing browser fields in the base config
// rather than appending to them.
func TestDeriveConnectConfig_OverridesExistingBrowserFields(t *testing.T) {
	origin, err := browserorigin.Parse("https://new-app.lovable.app")
	assert.NoError(t, err)

	base := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://old-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://old-app.lovable.app"},
		PasskeyRpID:      "old-app.lovable.app",
		PasskeyRpName:    "old-name",
	}

	result := deriveConnectConfig(origin, "new-app.lovable.app", "new-name", base)

	assert.Equal(t, []string{"https://new-app.lovable.app"}, result.AllowedOrigins)
	assert.Equal(t, []string{"https://new-app.lovable.app"}, result.PasskeyRpOrigins)
	assert.Equal(t, "new-app.lovable.app", result.PasskeyRpID)
	assert.Equal(t, "new-name", result.PasskeyRpName)
}

// TestBrowserConfigMatches_ExactMatchReturnsTrueNoDeltas verifies that
// browserConfigMatches returns true with no deltas when the profile's browser
// fields exactly match the requested origin and RP ID.
func TestBrowserConfigMatches_ExactMatchReturnsTrueNoDeltas(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://your-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://your-app.lovable.app"},
		PasskeyRpID:      "your-app.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.True(t, matched)
	assert.Empty(t, deltas)
}

// TestBrowserConfigMatches_MissingCORSOriginReturnsDelta verifies that a
// missing CORS origin produces a delta with the current and proposed values.
func TestBrowserConfigMatches_MissingCORSOriginReturnsDelta(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://other-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://your-app.lovable.app"},
		PasskeyRpID:      "your-app.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.False(t, matched)
	assert.Len(t, deltas, 1)
	assert.Equal(t, "CORS origin (--cors-origin)", deltas[0].Field)
	assert.Equal(t, "https://other-app.lovable.app", deltas[0].Current)
	assert.Equal(t, "https://your-app.lovable.app", deltas[0].Proposed)
}

// TestBrowserConfigMatches_MismatchedRPIDReturnsDelta verifies that a
// mismatched Passkey RP ID produces a delta.
func TestBrowserConfigMatches_MismatchedRPIDReturnsDelta(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://your-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://your-app.lovable.app"},
		PasskeyRpID:      "old-rp-id.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.False(t, matched)
	assert.Len(t, deltas, 1)
	assert.Equal(t, "Passkey RP ID (--passkey-rp-id)", deltas[0].Field)
	assert.Equal(t, "old-rp-id.lovable.app", deltas[0].Current)
	assert.Equal(t, "your-app.lovable.app", deltas[0].Proposed)
}

// TestBrowserConfigMatches_MissingPasskeyOriginReturnsDelta verifies that a
// missing passkey RP origin produces a delta.
func TestBrowserConfigMatches_MissingPasskeyOriginReturnsDelta(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://your-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://other-app.lovable.app"},
		PasskeyRpID:      "your-app.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.False(t, matched)
	assert.Len(t, deltas, 1)
	assert.Equal(t, "Passkey RP origin (--passkey-rp-origin)", deltas[0].Field)
}

// TestBrowserConfigMatches_AllFieldsMismatchReturnsThreeDeltas verifies that
// when all three browser fields differ, three deltas are returned in
// deterministic order: CORS origin, RP ID, RP origin.
func TestBrowserConfigMatches_AllFieldsMismatchReturnsThreeDeltas(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{"https://old.lovable.app"},
		PasskeyRpOrigins: []string{"https://old.lovable.app"},
		PasskeyRpID:      "old.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.False(t, matched)
	assert.Len(t, deltas, 3)
	assert.Equal(t, "CORS origin (--cors-origin)", deltas[0].Field)
	assert.Equal(t, "Passkey RP ID (--passkey-rp-id)", deltas[1].Field)
	assert.Equal(t, "Passkey RP origin (--passkey-rp-origin)", deltas[2].Field)
}

// TestBrowserConfigMatches_EmptySlicesProduceUnsetCurrentValue verifies that
// empty AllowedOrigins and PasskeyRpOrigins slices produce deltas with an
// empty (unset) current value.
func TestBrowserConfigMatches_EmptySlicesProduceUnsetCurrentValue(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		PasskeyRpID: "your-app.lovable.app",
	}

	matched, deltas := browserConfigMatches(profile, origin, "your-app.lovable.app", "")
	assert.False(t, matched)
	assert.Len(t, deltas, 2)
	assert.Equal(t, "", deltas[0].Current)
	assert.Equal(t, "", deltas[1].Current)
}

func TestBrowserConfigMatches_MismatchedRPNameReturnsDelta(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	profile := serve.GatewayConfig{
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "old-name",
	}

	matched, deltas := browserConfigMatches(profile, origin, origin.RPID, "new-name")
	assert.False(t, matched)
	assert.Len(t, deltas, 1)
	assert.Equal(t, "Passkey RP name (--passkey-rp-name)", deltas[0].Field)
	assert.Equal(t, "old-name", deltas[0].Current)
	assert.Equal(t, "new-name", deltas[0].Proposed)
}

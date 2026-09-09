// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"testing"

	"github.com/stretchr/testify/assert"

	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// validProfile returns a GatewayLaunchProfile with every field valid for
// ValidateLaunchProfile. NetworkIdentityFile is empty (as WriteLaunchProfile
// clears it before persistence). These tests call ValidateLaunchProfile
// directly — no file I/O, no network, pure Tier 1.
func validProfile() GatewayLaunchProfile {
	cfg := validTestGatewayConfig()
	cfg.NetworkIdentityFile = ""
	return GatewayLaunchProfile{Version: LaunchProfileVersion, Config: cfg}
}

func TestValidateLaunchProfile_HTTPPortExceeds65535ReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.HTTPPort = 65536
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "exceeds 65535")
}

func TestValidateLaunchProfile_HTTPSPortExceeds65535ReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.HTTPSPort = 70000
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "exceeds 65535")
}

func TestValidateLaunchProfile_PortCollisionReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.HTTPPort = 9000
	p.Config.HTTPSPort = 9000
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "must differ")
}

func TestValidateLaunchProfile_InvalidLogLevelReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.LogLevel = "bogus"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "log_level")
}

func TestValidateLaunchProfile_InvalidCertIdentityModeReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.CertIdentityMode = "bogus-mode"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "cert_identity_mode")
}

func TestValidateLaunchProfile_InvalidAllowedOriginReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.AllowedOrigins = []string{"not-a-url"}
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "allowed origin")
}

func TestValidateLaunchProfile_InvalidPasskeyOriginReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.PasskeyRpOrigins = []string{"ftp://bad.example"}
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "passkey origin")
}

func TestValidateLaunchProfile_PasskeyOriginRPMismatchReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.PasskeyRpID = "your-app.lovable.app"
	p.Config.PasskeyRpOrigins = []string{"https://other-app.lovable.app"}
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "passkey RP configuration")
}

func TestValidateLaunchProfile_InvalidPublicBaseURLReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.PublicBaseURL = "ftp://bad.example"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "public_base_url")
}

func TestValidateLaunchProfile_ConsensusURLMustBeHTTPSReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.ConsensusURL = "http://insecure.example/deliberate"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "consensus_url")
}

func TestValidateLaunchProfile_InvalidMCPDownstreamURLReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.MCPDownstreamURL = "ftp://bad.example/mcp"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "mcp_downstream_url")
}

func TestValidateLaunchProfile_InvalidA2ADownstreamURLReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.A2ADownstreamURL = "gopher://bad.example/a2a"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
	assert.Contains(t, err.Error(), "a2a_downstream_url")
}

func TestValidateLaunchProfile_PublicBaseURLWithFragmentReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.PublicBaseURL = "https://example.com#frag"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestValidateLaunchProfile_PublicBaseURLWithUserInfoReturnsErrLaunchProfileInvalid(t *testing.T) {
	p := validProfile()
	p.Config.PublicBaseURL = "https://user:pass@example.com"
	err := ValidateLaunchProfile(p)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestValidateLaunchProfile_AcceptsAllValidPostures(t *testing.T) {
	validPostures := []g8econfig.GatewayPosture{
		g8econfig.PostureDoctrine,
		g8econfig.PostureConsensus,
		g8econfig.PostureRatify,
		g8econfig.PostureNotary,
	}
	for _, p := range validPostures {
		profile := validProfile()
		profile.Config.Posture = p
		assert.NoError(t, ValidateLaunchProfile(profile), "posture %s should be valid", p)
	}
}

func TestValidateLaunchProfile_AcceptsEmptyCertIdentityMode(t *testing.T) {
	p := validProfile()
	p.Config.CertIdentityMode = ""
	assert.NoError(t, ValidateLaunchProfile(p))
}

func TestValidateLaunchProfile_AcceptsEmptyServiceURLs(t *testing.T) {
	p := validProfile()
	p.Config.PublicBaseURL = ""
	p.Config.ConsensusURL = ""
	p.Config.MCPDownstreamURL = ""
	p.Config.A2ADownstreamURL = ""
	assert.NoError(t, ValidateLaunchProfile(p))
}

func TestValidateLaunchProfile_AcceptsHTTPPublicBaseURL(t *testing.T) {
	p := validProfile()
	p.Config.PublicBaseURL = "http://localhost:3000"
	assert.NoError(t, ValidateLaunchProfile(p))
}

func TestValidateLaunchProfile_AcceptsHTTPMCPDownstreamURL(t *testing.T) {
	p := validProfile()
	p.Config.MCPDownstreamURL = "http://downstream:3000/mcp"
	assert.NoError(t, ValidateLaunchProfile(p))
}

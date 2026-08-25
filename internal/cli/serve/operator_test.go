// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestServeOperatorOptions_ZeroValue(t *testing.T) {
	var opts ServeOperatorOptions

	assert.Equal(t, "", opts.LogLevel)
	assert.Equal(t, "", opts.Endpoint)
	assert.Equal(t, "", opts.TrustBundlePath)
	assert.Equal(t, "", opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert)
	assert.Equal(t, "", opts.WorkingDir)
	assert.Equal(t, "", opts.LaunchDir)
	assert.False(t, opts.CloudMode)
	assert.Equal(t, "", opts.CloudProvider)
	assert.False(t, opts.ExecutionVault)
	assert.False(t, opts.NoGit)
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval)
	assert.Equal(t, "", opts.LatticeEndpoint)
	assert.Equal(t, "", opts.LatticeClientID)
	assert.Equal(t, "", opts.LatticeClientSecret)
	assert.Equal(t, "", opts.LatticeSandboxesToken)
	assert.Equal(t, "", opts.LatticeEntityName)
	assert.Equal(t, "", opts.LatticePostureFloor)
}

func TestServeOperatorOptions_FullAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:              "debug",
		Endpoint:              "192.168.1.10",
		TrustBundlePath:       constants.DefaultPKIDir + "/" + constants.PkiFileGatewayBundle,
		PrivateKey:            constants.DefaultOperatorKeyDesc,
		ClientCert:            constants.DefaultOperatorCertDesc,
		WorkingDir:            constants.DefaultDataDir,
		LaunchDir:             constants.DefaultPKIDir,
		CloudMode:             true,
		CloudProvider:         "aws",
		ExecutionVault:        true,
		NoGit:                 true,
		HeartbeatInterval:     30 * time.Second,
		LatticeEndpoint:       "lattice.example.com:443",
		LatticeClientID:       "client-id",
		LatticeClientSecret:   "secret",
		LatticeSandboxesToken: "sandbox-token",
		LatticeEntityName:     "g8e-op-1",
		LatticePostureFloor:   "notary",
	}

	assert.Equal(t, "debug", opts.LogLevel)
	assert.Equal(t, "192.168.1.10", opts.Endpoint)
	assert.Equal(t, constants.DefaultPKIDir+"/"+constants.PkiFileGatewayBundle, opts.TrustBundlePath)
	assert.Equal(t, constants.DefaultOperatorKeyDesc, opts.PrivateKey)
	assert.Equal(t, constants.DefaultOperatorCertDesc, opts.ClientCert)
	assert.Equal(t, constants.DefaultDataDir, opts.WorkingDir)
	assert.Equal(t, constants.DefaultPKIDir, opts.LaunchDir)
	assert.True(t, opts.CloudMode)
	assert.Equal(t, "aws", opts.CloudProvider)
	assert.True(t, opts.ExecutionVault)
	assert.True(t, opts.NoGit)
	assert.Equal(t, 30*time.Second, opts.HeartbeatInterval)
	assert.Equal(t, "lattice.example.com:443", opts.LatticeEndpoint)
	assert.Equal(t, "client-id", opts.LatticeClientID)
	assert.Equal(t, "secret", opts.LatticeClientSecret)
	assert.Equal(t, "sandbox-token", opts.LatticeSandboxesToken)
	assert.Equal(t, "g8e-op-1", opts.LatticeEntityName)
	assert.Equal(t, "notary", opts.LatticePostureFloor)
}

func TestServeOperatorOptions_Equality(t *testing.T) {
	a := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       constants.DefaultEndpoint,
		PrivateKey:     constants.DefaultOperatorKeyDesc,
		ClientCert:     constants.DefaultOperatorCertDesc,
		WorkingDir:     constants.DefaultDataDir,
		LaunchDir:      constants.DefaultPKIDir,
		CloudMode:      false,
		ExecutionVault: true,
	}
	b := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       constants.DefaultEndpoint,
		PrivateKey:     constants.DefaultOperatorKeyDesc,
		ClientCert:     constants.DefaultOperatorCertDesc,
		WorkingDir:     constants.DefaultDataDir,
		LaunchDir:      constants.DefaultPKIDir,
		CloudMode:      false,
		ExecutionVault: true,
	}
	c := a
	c.ExecutionVault = false

	require.True(t, a == b, "structs with identical fields should be equal")
	require.False(t, a == c, "structs differing in any field should not be equal")
}

func TestServeOperatorOptions_PartialAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:   "info",
		Endpoint:   "10.0.0.1",
		PrivateKey: constants.DefaultOperatorKeyDesc,
	}

	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "10.0.0.1", opts.Endpoint)
	assert.Equal(t, constants.DefaultOperatorKeyDesc, opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert, "unassigned ClientCert should be zero value")
	assert.Equal(t, "", opts.WorkingDir, "unassigned WorkingDir should be zero value")
	assert.Equal(t, "", opts.LaunchDir, "unassigned LaunchDir should be zero value")
	assert.False(t, opts.CloudMode, "unassigned CloudMode should be false")
	assert.False(t, opts.ExecutionVault, "unassigned ExecutionVault should be false")
	assert.False(t, opts.NoGit, "unassigned NoGit should be false")
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval, "unassigned HeartbeatInterval should be zero")
}

func TestServeOperatorOptions_HeartbeatInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"one second", 1 * time.Second, 1 * time.Second},
		{"thirty seconds", 30 * time.Second, 30 * time.Second},
		{"one minute", time.Minute, 60 * time.Second},
		{"five minutes", 5 * time.Minute, 300 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServeOperatorOptions{HeartbeatInterval: tt.duration}
			assert.Equal(t, tt.expected, opts.HeartbeatInterval)
		})
	}
}

func TestServeOperatorOptions_CloudProviderValues(t *testing.T) {
	providers := []string{"aws", "gcp", "azure", ""}

	for _, p := range providers {
		opts := ServeOperatorOptions{CloudMode: true, CloudProvider: p}
		assert.Equal(t, p, opts.CloudProvider)
		assert.True(t, opts.CloudMode)
	}
}

func TestResolveOperatorEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string returns default", "", constants.DefaultEndpoint},
		{"whitespace only returns default", "   ", constants.DefaultEndpoint},
		{"tab only returns default", "\t", constants.DefaultEndpoint},
		{"simple hostname", constants.DefaultEndpoint, constants.DefaultEndpoint},
		{"ip address", "192.168.1.10", "192.168.1.10"},
		{"hostname with port", "gateway.local:8080", "gateway.local:8080"},
		{"leading whitespace trimmed", "  " + constants.DefaultEndpoint, constants.DefaultEndpoint},
		{"trailing whitespace trimmed", constants.DefaultEndpoint + "  ", constants.DefaultEndpoint},
		{"surrounding whitespace trimmed", "  10.0.0.1  ", "10.0.0.1"},
		{"fqdn", "operator.example.com", "operator.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOperatorEndpoint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkingDir(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"working dir set, launch dir set", constants.DefaultDataDir, constants.DefaultPKIDir, constants.DefaultDataDir},
		{"working dir set, launch dir empty", constants.DefaultDataDir, "", constants.DefaultDataDir},
		{"working dir empty, launch dir set", "", constants.DefaultPKIDir, constants.DefaultPKIDir},
		{"both empty", "", "", ""},
		{"working dir takes precedence", constants.RuntimeDirname, constants.PkiDirname, constants.RuntimeDirname},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// resolveOperatorEndpoint — additional edge cases
// ---------------------------------------------------------------------------

func TestResolveOperatorEndpoint_ReturnsConstantsDefault(t *testing.T) {
	result := resolveOperatorEndpoint("")
	assert.Equal(t, constants.DefaultEndpoint, result,
		"empty endpoint should return constants.DefaultEndpoint, not a hardcoded string")
}

func TestResolveOperatorEndpoint_NewlineTrimmed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"leading newline", "\n" + constants.DefaultEndpoint},
		{"trailing newline", constants.DefaultEndpoint + "\n"},
		{"carriage return", "\r" + constants.DefaultEndpoint + "\r"},
		{"mixed newline and tab", "\n\t" + constants.DefaultEndpoint + "\n\t"},
		{"CRLF", "\r\n" + constants.DefaultEndpoint + "\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOperatorEndpoint(tt.input)
			assert.Equal(t, constants.DefaultEndpoint, result)
		})
	}
}

func TestResolveOperatorEndpoint_PreservesInternalWhitespace(t *testing.T) {
	result := resolveOperatorEndpoint("  local  host  ")
	assert.Equal(t, "local  host", result,
		"only leading/trailing whitespace should be trimmed; internal whitespace preserved")
}

func TestResolveOperatorEndpoint_MixedWhitespaceTypes(t *testing.T) {
	result := resolveOperatorEndpoint("\t\n\r  10.0.0.1  \r\n\t")
	assert.Equal(t, "10.0.0.1", result)
}

func TestResolveOperatorEndpoint_LongEndpoint(t *testing.T) {
	long := strings.Repeat("a", 1000)
	result := resolveOperatorEndpoint(long)
	assert.Equal(t, long, result)
}

func TestResolveOperatorEndpoint_WhitespaceOnlyWithNewlines(t *testing.T) {
	result := resolveOperatorEndpoint("\n\r\t  \n\r")
	assert.Equal(t, constants.DefaultEndpoint, result,
		"input that is only whitespace (including newlines) should return default")
}

// ---------------------------------------------------------------------------
// ServeOperatorOptions — additional edge cases
// ---------------------------------------------------------------------------

func TestServeOperatorOptions_NegativeHeartbeatInterval(t *testing.T) {
	opts := ServeOperatorOptions{HeartbeatInterval: -5 * time.Second}
	assert.Equal(t, -5*time.Second, opts.HeartbeatInterval,
		"negative durations should be stored as-is; validation is the caller's responsibility")
}

func TestServeOperatorOptions_AllBooleansTrue(t *testing.T) {
	opts := ServeOperatorOptions{
		CloudMode:      true,
		ExecutionVault: true,
		NoGit:          true,
	}
	assert.True(t, opts.CloudMode)
	assert.True(t, opts.ExecutionVault)
	assert.True(t, opts.NoGit)
}

func TestServeOperatorOptions_AllBooleansFalse(t *testing.T) {
	opts := ServeOperatorOptions{
		CloudMode:      false,
		ExecutionVault: false,
		NoGit:          false,
	}
	assert.False(t, opts.CloudMode)
	assert.False(t, opts.ExecutionVault)
	assert.False(t, opts.NoGit)
}

func TestServeOperatorOptions_Equality_DifferInEachField(t *testing.T) {
	base := ServeOperatorOptions{
		LogLevel:          "info",
		Endpoint:          constants.DefaultEndpoint,
		TrustBundlePath:   constants.DefaultPKIDir + "/" + constants.PkiFileGatewayBundle,
		PrivateKey:        constants.DefaultOperatorKeyDesc,
		ClientCert:        constants.DefaultOperatorCertDesc,
		WorkingDir:        constants.DefaultDataDir,
		LaunchDir:         constants.DefaultPKIDir,
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 30 * time.Second,
	}

	fields := []struct {
		name string
		mut  func(o *ServeOperatorOptions)
	}{
		{"LogLevel", func(o *ServeOperatorOptions) { o.LogLevel = "debug" }},
		{"Endpoint", func(o *ServeOperatorOptions) { o.Endpoint = "other" }},
		{"TrustBundlePath", func(o *ServeOperatorOptions) { o.TrustBundlePath = "/other" }},
		{"PrivateKey", func(o *ServeOperatorOptions) { o.PrivateKey = "/other" }},
		{"ClientCert", func(o *ServeOperatorOptions) { o.ClientCert = "/other" }},
		{"WorkingDir", func(o *ServeOperatorOptions) { o.WorkingDir = "/other" }},
		{"LaunchDir", func(o *ServeOperatorOptions) { o.LaunchDir = "/other" }},
		{"CloudMode", func(o *ServeOperatorOptions) { o.CloudMode = false }},
		{"CloudProvider", func(o *ServeOperatorOptions) { o.CloudProvider = "gcp" }},
		{"ExecutionVault", func(o *ServeOperatorOptions) { o.ExecutionVault = false }},
		{"NoGit", func(o *ServeOperatorOptions) { o.NoGit = false }},
		{"HeartbeatInterval", func(o *ServeOperatorOptions) { o.HeartbeatInterval = 60 * time.Second }},
		{"LatticeEndpoint", func(o *ServeOperatorOptions) { o.LatticeEndpoint = "other" }},
		{"LatticeClientID", func(o *ServeOperatorOptions) { o.LatticeClientID = "other" }},
		{"LatticeClientSecret", func(o *ServeOperatorOptions) { o.LatticeClientSecret = "other" }},
		{"LatticeSandboxesToken", func(o *ServeOperatorOptions) { o.LatticeSandboxesToken = "other" }},
		{"LatticeEntityName", func(o *ServeOperatorOptions) { o.LatticeEntityName = "other" }},
		{"LatticePostureFloor", func(o *ServeOperatorOptions) { o.LatticePostureFloor = "other" }},
	}

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			modified := base
			f.mut(&modified)
			require.False(t, base == modified,
				"structs differing in %s should not be equal", f.name)
		})
	}
}

func TestServeOperatorOptions_Equality_AllFieldsEqual(t *testing.T) {
	a := ServeOperatorOptions{
		LogLevel:          "debug",
		Endpoint:          "10.0.0.1",
		TrustBundlePath:   constants.DefaultPKIDir + "/" + constants.PkiFileGatewayBundle,
		PrivateKey:        constants.DefaultOperatorKeyDesc,
		ClientCert:        constants.DefaultOperatorCertDesc,
		WorkingDir:        constants.DefaultDataDir,
		LaunchDir:         constants.DefaultPKIDir,
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 45 * time.Second,
	}
	b := a
	require.True(t, a == b, "structs with all fields identical should be equal")
}

// ---------------------------------------------------------------------------
// buildOperatorLoadOptions
// ---------------------------------------------------------------------------

func TestBuildOperatorLoadOptions_BasicMapping(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:          "debug",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 45 * time.Second,
	}

	loadOpts := buildOperatorLoadOptions(opts, "10.0.0.1", "/work/dir")

	assert.Equal(t, "10.0.0.1", loadOpts.OperatorEndpoint)
	assert.Equal(t, 0, loadOpts.HTTPPort)
	assert.Equal(t, 0, loadOpts.HTTPSPort)
	assert.True(t, loadOpts.CloudMode)
	assert.Equal(t, "aws", loadOpts.CloudProvider)
	assert.True(t, loadOpts.ExecutionVaultEnabled)
	assert.True(t, loadOpts.NoGit)
	assert.Equal(t, "debug", loadOpts.LogLevel)
	assert.Equal(t, "/work/dir", loadOpts.WorkDir)
	assert.Equal(t, 45*time.Second, loadOpts.HeartbeatInterval)
}

func TestBuildOperatorLoadOptions_EmptyEndpoint(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "", "/work")
	assert.Equal(t, "", loadOpts.OperatorEndpoint)
	assert.Equal(t, "/work", loadOpts.WorkDir)
}

func TestBuildOperatorLoadOptions_ZeroValues(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, constants.DefaultPKIDir)

	assert.False(t, loadOpts.CloudMode)
	assert.False(t, loadOpts.ExecutionVaultEnabled)
	assert.False(t, loadOpts.NoGit)
	assert.Equal(t, "", loadOpts.CloudProvider)
	assert.Equal(t, time.Duration(0), loadOpts.HeartbeatInterval)
	assert.Equal(t, "", loadOpts.PKIDir)
	assert.Equal(t, "", loadOpts.SecretsDir)
	assert.Equal(t, config.GatewayPosture(""), loadOpts.Posture)
}

func TestBuildOperatorLoadOptions_EnvVarsPropagated(t *testing.T) {
	t.Setenv(string(constants.EnvVar.Shell), "/bin/zsh")
	t.Setenv(string(constants.EnvVar.Lang), "en_US.UTF-8")
	t.Setenv(string(constants.EnvVar.Term), "xterm-256color")
	t.Setenv(string(constants.EnvVar.TZ), "America/Los_Angeles")

	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, constants.DefaultDataDir)

	assert.Equal(t, "/bin/zsh", loadOpts.Shell)
	assert.Equal(t, "en_US.UTF-8", loadOpts.Lang)
	assert.Equal(t, "xterm-256color", loadOpts.Term)
	assert.Equal(t, "America/Los_Angeles", loadOpts.TZ)
}

func TestBuildOperatorLoadOptions_PortAlwaysZero(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "10.0.0.1", "/work")
	assert.Equal(t, 0, loadOpts.HTTPPort, "HTTPPort should always be 0 for operator mode")
	assert.Equal(t, 0, loadOpts.HTTPSPort, "HTTPSPort should always be 0 for operator mode")
}

func TestBuildOperatorLoadOptions_PostureAlwaysEmpty(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, constants.DefaultDataDir)
	assert.Equal(t, config.GatewayPosture(""), loadOpts.Posture, "Posture should always be empty for operator mode")
}

func TestBuildOperatorLoadOptions_PKIAndSecretsAlwaysEmpty(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, constants.DefaultDataDir)
	assert.Equal(t, "", loadOpts.PKIDir, "PKIDir should always be empty for operator mode")
	assert.Equal(t, "", loadOpts.SecretsDir, "SecretsDir should always be empty for operator mode")
}

func TestBuildOperatorLoadOptions_AllFieldsPopulated(t *testing.T) {
	t.Setenv(string(constants.EnvVar.Shell), "/bin/bash")
	t.Setenv(string(constants.EnvVar.Lang), "C.UTF-8")
	t.Setenv(string(constants.EnvVar.Term), "screen")
	t.Setenv(string(constants.EnvVar.TZ), "UTC")

	opts := ServeOperatorOptions{
		LogLevel:          "warn",
		CloudMode:         true,
		CloudProvider:     "gcp",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 90 * time.Second,
	}

	loadOpts := buildOperatorLoadOptions(opts, "operator.internal:8443", "/custom/work")

	assert.Equal(t, "operator.internal:8443", loadOpts.OperatorEndpoint)
	assert.Equal(t, 0, loadOpts.HTTPPort)
	assert.Equal(t, 0, loadOpts.HTTPSPort)
	assert.True(t, loadOpts.CloudMode)
	assert.Equal(t, "gcp", loadOpts.CloudProvider)
	assert.True(t, loadOpts.ExecutionVaultEnabled)
	assert.True(t, loadOpts.NoGit)
	assert.Equal(t, "warn", loadOpts.LogLevel)
	assert.Equal(t, "/custom/work", loadOpts.WorkDir)
	assert.Equal(t, 90*time.Second, loadOpts.HeartbeatInterval)
	assert.Equal(t, "", loadOpts.PKIDir)
	assert.Equal(t, "", loadOpts.SecretsDir)
	assert.Equal(t, config.GatewayPosture(""), loadOpts.Posture)
	assert.Equal(t, "/bin/bash", loadOpts.Shell)
	assert.Equal(t, "C.UTF-8", loadOpts.Lang)
	assert.Equal(t, "screen", loadOpts.Term)
	assert.Equal(t, "UTC", loadOpts.TZ)
}

func TestBuildOperatorLoadOptions_HeartbeatIntervalPropagated(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"one second", 1 * time.Second},
		{"thirty seconds", 30 * time.Second},
		{"one minute", time.Minute},
		{"five minutes", 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServeOperatorOptions{HeartbeatInterval: tt.duration}
			loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")
			assert.Equal(t, tt.duration, loadOpts.HeartbeatInterval)
		})
	}
}

func TestBuildOperatorLoadOptions_EnvVarsUnset(t *testing.T) {
	t.Setenv(string(constants.EnvVar.Shell), "")
	t.Setenv(string(constants.EnvVar.Lang), "")
	t.Setenv(string(constants.EnvVar.Term), "")
	t.Setenv(string(constants.EnvVar.TZ), "")

	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")

	assert.Equal(t, "", loadOpts.Shell)
	assert.Equal(t, "", loadOpts.Lang)
	assert.Equal(t, "", loadOpts.Term)
	assert.Equal(t, "", loadOpts.TZ)
}

func TestBuildOperatorLoadOptions_LatticeNilWhenEndpointEmpty(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")
	assert.Nil(t, loadOpts.Lattice)
}

func TestBuildOperatorLoadOptions_LatticeFieldsPropagated(t *testing.T) {
	opts := ServeOperatorOptions{
		LatticeEndpoint:       "lattice.example.com:443",
		LatticeClientID:       "client-id",
		LatticeClientSecret:   "secret",
		LatticeSandboxesToken: "sandbox-token",
		LatticeEntityName:     "g8e-op-1",
		LatticePostureFloor:   "notary",
	}

	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")

	require.NotNil(t, loadOpts.Lattice)
	assert.True(t, loadOpts.Lattice.Enabled)
	assert.Equal(t, "lattice.example.com:443", loadOpts.Lattice.Endpoint)
	assert.Equal(t, "client-id", loadOpts.Lattice.ClientID)
	assert.Equal(t, "secret", loadOpts.Lattice.ClientSecret)
	assert.Equal(t, "sandbox-token", loadOpts.Lattice.SandboxesToken)
	assert.Equal(t, "g8e-op-1", loadOpts.Lattice.Entity.Name)
	assert.Equal(t, "g8e-operator", loadOpts.Lattice.Entity.PlatformType)
	assert.Equal(t, "notary", loadOpts.Lattice.PostureFloor)
}

func TestBuildOperatorLoadOptions_LatticeEnvVarFallback(t *testing.T) {
	t.Setenv(string(constants.EnvVar.LatticeEndpoint), "env-lattice:443")
	t.Setenv(string(constants.EnvVar.LatticeClientID), "env-client-id")
	t.Setenv(string(constants.EnvVar.LatticeClientSecret), "env-secret")
	t.Setenv(string(constants.EnvVar.LatticeSandboxesToken), "env-sandbox-token")
	t.Setenv(string(constants.EnvVar.LatticeEntityName), "env-entity")
	t.Setenv(string(constants.EnvVar.LatticePostureFloor), "doctrine")

	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")

	require.NotNil(t, loadOpts.Lattice)
	assert.True(t, loadOpts.Lattice.Enabled)
	assert.Equal(t, "env-lattice:443", loadOpts.Lattice.Endpoint)
	assert.Equal(t, "env-client-id", loadOpts.Lattice.ClientID)
	assert.Equal(t, "env-secret", loadOpts.Lattice.ClientSecret)
	assert.Equal(t, "env-sandbox-token", loadOpts.Lattice.SandboxesToken)
	assert.Equal(t, "env-entity", loadOpts.Lattice.Entity.Name)
	assert.Equal(t, "g8e-operator", loadOpts.Lattice.Entity.PlatformType)
	assert.Equal(t, "doctrine", loadOpts.Lattice.PostureFloor)
}

func TestBuildOperatorLoadOptions_LatticeFlagOverridesEnvVar(t *testing.T) {
	t.Setenv(string(constants.EnvVar.LatticeEndpoint), "env-lattice:443")
	t.Setenv(string(constants.EnvVar.LatticeClientID), "env-client-id")

	opts := ServeOperatorOptions{
		LatticeEndpoint: "flag-lattice:443",
		LatticeClientID: "flag-client-id",
	}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")

	require.NotNil(t, loadOpts.Lattice)
	assert.Equal(t, "flag-lattice:443", loadOpts.Lattice.Endpoint)
	assert.Equal(t, "flag-client-id", loadOpts.Lattice.ClientID)
}

func TestBuildOperatorLoadOptions_LatticeDefaultPostureFloor(t *testing.T) {
	opts := ServeOperatorOptions{
		LatticeEndpoint: "lattice.example.com:443",
	}
	loadOpts := buildOperatorLoadOptions(opts, constants.DefaultEndpoint, "/work")

	require.NotNil(t, loadOpts.Lattice)
	assert.Equal(t, "", loadOpts.Lattice.PostureFloor,
		"PostureFloor should be empty when not set; Validate() defaults it to consensus")
}

func TestResolveLatticeOpt(t *testing.T) {
	t.Run("flag value takes precedence", func(t *testing.T) {
		t.Setenv(string(constants.EnvVar.LatticeEndpoint), "env-value")
		result := resolveLatticeOpt("flag-value", constants.EnvVar.LatticeEndpoint)
		assert.Equal(t, "flag-value", result)
	})

	t.Run("env var fallback when flag empty", func(t *testing.T) {
		t.Setenv(string(constants.EnvVar.LatticeEndpoint), "env-value")
		result := resolveLatticeOpt("", constants.EnvVar.LatticeEndpoint)
		assert.Equal(t, "env-value", result)
	})

	t.Run("empty when both flag and env unset", func(t *testing.T) {
		result := resolveLatticeOpt("", constants.EnvVar.LatticeEndpoint)
		assert.Equal(t, "", result)
	})
}

func TestClassifyConfigLoadError_NonPostureError_ReturnsConfigError(t *testing.T) {
	exitCode, actionable := classifyConfigLoadError(errors.New("some other config error"))
	assert.Equal(t, constants.ExitConfigError, exitCode,
		"all config.Load errors should return ExitConfigError")
	assert.Empty(t, actionable, "config errors should not produce an actionable message")
}

func TestClassifyConfigLoadError_NilError_ReturnsConfigError(t *testing.T) {
	exitCode, actionable := classifyConfigLoadError(nil)
	assert.Equal(t, constants.ExitConfigError, exitCode)
	assert.Empty(t, actionable)
}

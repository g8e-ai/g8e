// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// --- NewModel: initial state ---

func TestNewModel_InitialStep(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, StepNetwork, m.step)
	assert.False(t, m.quitting)
}

func TestNewModel_InitialFocusIndex(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, 0, m.focusIndex)
}

func TestNewModel_PublicBaseURLInputFocused(t *testing.T) {
	m := NewModel(Options{})
	assert.True(t, m.publicBaseURLInput.Focused(), "first input should be focused on init")
}

// --- NewModel: pre-fill from initial config ---

func TestNewModel_PublicBaseURLPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{PublicBaseURL: "https://demo.g8e.ai"},
	})
	assert.Equal(t, "https://demo.g8e.ai", m.publicBaseURLInput.Value())
}

func TestNewModel_PublicBaseURLDefaultWhenEmpty(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, "", m.publicBaseURLInput.Value())
}

func TestNewModel_PasskeyRpOriginPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			PasskeyRpOrigins: []string{"https://demo.g8e.ai"},
		},
	})
	assert.Equal(t, "https://demo.g8e.ai", m.passkeyRpOriginInput.Value())
}

func TestNewModel_PasskeyRpOriginEmptyWhenNoInitialConfig(t *testing.T) {
	m := NewModel(Options{})
	assert.Empty(t, m.passkeyRpOriginInput.Value())
}

func TestNewModel_CorsOriginPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			AllowedOrigins: []string{"https://console.g8e.ai"},
		},
	})
	assert.Equal(t, "https://console.g8e.ai", m.corsOriginInput.Value())
}

func TestNewModel_CorsOriginEmptyWhenNoInitialConfig(t *testing.T) {
	m := NewModel(Options{})
	assert.Empty(t, m.corsOriginInput.Value())
}

func TestNewModel_ConsensusIDPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{ConsensusID: "trib-prod-01"},
	})
	assert.Equal(t, "trib-prod-01", m.consensusIDInput.Value())
}

func TestNewModel_ConsensusURLPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{ConsensusURL: "https://consensus.g8e.ai"},
	})
	assert.Equal(t, "https://consensus.g8e.ai", m.consensusURLInput.Value())
}

func TestNewModel_ConsensusBootstrapPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{ConsensusBootstrap: "/etc/g8e/bootstrap.json"},
	})
	assert.Equal(t, "/etc/g8e/bootstrap.json", m.consensusBootstrapInput.Value())
}

func TestNewModel_PasskeyRpIDPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{PasskeyRpID: "demo.g8e.ai"},
	})
	assert.Equal(t, "demo.g8e.ai", m.passkeyRpIDInput.Value())
}

func TestNewModel_PasskeyRpIDDefaultsToLocalthost(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, "localhost", m.passkeyRpIDInput.Value())
}

func TestNewModel_PasskeyRpNamePreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{PasskeyRpName: "custom"},
	})
	assert.Equal(t, "custom", m.passkeyRpNameInput.Value())
}

func TestNewModel_PasskeyRpNameDefaultsToG8e(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, "g8e", m.passkeyRpNameInput.Value())
}

func TestNewModel_MCPDownstreamPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{MCPDownstreamURL: "http://mcp:3000"},
	})
	assert.Equal(t, "http://mcp:3000", m.mcpDownstreamInput.Value())
}

func TestNewModel_A2ADownstreamPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{A2ADownstreamURL: "http://a2a:3001"},
	})
	assert.Equal(t, "http://a2a:3001", m.a2aDownstreamInput.Value())
}

// --- NewModel: cert mode choice from config ---

func TestNewModel_CertModeChoiceLocalhostDefault(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, 0, m.certModeChoice)
}

func TestNewModel_CertModeChoiceFullFromConfig(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{CertIdentityMode: "full"},
	})
	assert.Equal(t, 1, m.certModeChoice)
}

func TestNewModel_CertModeChoiceLocalhostFromConfig(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{CertIdentityMode: "localhost"},
	})
	assert.Equal(t, 0, m.certModeChoice)
}

// --- NewModel: posture choice from config ---

func TestNewModel_PostureChoiceDoctrineDefault(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, 0, m.postureChoice)
}

func TestNewModel_PostureChoiceConsensusFromConfig(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{Posture: "consensus"},
	})
	assert.Equal(t, 1, m.postureChoice)
}

func TestNewModel_PostureChoiceNotaryFromConfig(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{Posture: "notary"},
	})
	assert.Equal(t, 2, m.postureChoice)
}

// --- NewModel: toggle initialization from config (Bug #5) ---

func TestNewModel_TogglesInitializedFromConfig(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			MCPDownstreamURL: "http://mcp:3000",
			A2ADownstreamURL: "http://a2a:3001",
			AllowedOrigins:   []string{"https://console.g8e.ai"},
		},
	})
	assert.True(t, m.routeToMCP, "routeToMCP should be true when MCPDownstreamURL is set")
	assert.True(t, m.routeToA2A, "routeToA2A should be true when A2ADownstreamURL is set")
	assert.True(t, m.needsWebFrontend, "needsWebFrontend should be true when AllowedOrigins is set")
}

func TestNewModel_TogglesFalseWhenConfigEmpty(t *testing.T) {
	m := NewModel(Options{})
	assert.False(t, m.routeToMCP)
	assert.False(t, m.routeToA2A)
	assert.False(t, m.needsWebFrontend)
}

// --- Init ---

func TestInit_ReturnsBlink(t *testing.T) {
	m := NewModel(Options{})
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

// --- result(): config mapping ---

func TestResult_AllFieldsPopulated(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			PublicBaseURL:      "https://demo.g8e.ai",
			CertIdentityMode:   "full",
			AllowedOrigins:     []string{"https://console.g8e.ai"},
			Posture:            "consensus",
			ConsensusID:        "trib-prod-01",
			ConsensusURL:       "https://consensus.g8e.ai",
			ConsensusBootstrap: "/etc/g8e/bootstrap.json",
			PasskeyRpID:        "demo.g8e.ai",
			PasskeyRpName:      "g8e",
			PasskeyRpOrigins:   []string{"https://demo.g8e.ai"},
			MCPDownstreamURL:   "http://mcp:3000",
			A2ADownstreamURL:   "http://a2a:3001",
		},
	})

	result := m.result()
	assert.False(t, result.Cancel)
	cfg := result.Config
	assert.Equal(t, "https://demo.g8e.ai", cfg.PublicBaseURL)
	assert.Equal(t, "full", cfg.CertIdentityMode)
	assert.Equal(t, []string{"https://console.g8e.ai"}, cfg.AllowedOrigins)
	assert.Equal(t, "consensus", cfg.Posture)
	assert.Equal(t, "trib-prod-01", cfg.ConsensusID)
	assert.Equal(t, "https://consensus.g8e.ai", cfg.ConsensusURL)
	assert.Equal(t, "/etc/g8e/bootstrap.json", cfg.ConsensusBootstrap)
	assert.Equal(t, "demo.g8e.ai", cfg.PasskeyRpID)
	assert.Equal(t, "g8e", cfg.PasskeyRpName)
	assert.Equal(t, "http://mcp:3000", cfg.MCPDownstreamURL)
	assert.Equal(t, "http://a2a:3001", cfg.A2ADownstreamURL)
}

func TestResult_DefaultValues(t *testing.T) {
	m := NewModel(Options{})
	result := m.result()
	cfg := result.Config
	assert.Equal(t, "doctrine", cfg.Posture)
	assert.Equal(t, "localhost", cfg.CertIdentityMode)
}

func TestResult_PasskeyOriginStoresExactValue(t *testing.T) {
	m := NewModel(Options{})
	m.passkeyRpOriginInput.SetValue("https://demo.g8e.ai:8443/some/path?q=1")
	result := m.result()
	assert.Equal(t, []string{"https://demo.g8e.ai:8443/some/path?q=1"}, result.Config.PasskeyRpOrigins)
}

func TestResult_PasskeyOriginEmptyWhenNotSet(t *testing.T) {
	m := NewModel(Options{})
	result := m.result()
	assert.Nil(t, result.Config.PasskeyRpOrigins)
}

// --- result(): stale config cleared when toggles disabled (Bug #2) ---

func TestResult_DisabledWebFrontendClearsAllowedOrigins(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			AllowedOrigins: []string{"https://stale.example.com"},
		},
	})
	m.needsWebFrontend = false
	result := m.result()
	assert.Nil(t, result.Config.AllowedOrigins)
}

func TestResult_DisabledRouteToMCPClearsDownstreamURL(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			MCPDownstreamURL: "http://stale-mcp:3000",
		},
	})
	m.routeToMCP = false
	result := m.result()
	assert.Empty(t, result.Config.MCPDownstreamURL)
}

func TestResult_DisabledRouteToA2AClearsDownstreamURL(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			A2ADownstreamURL: "http://stale-a2a:3001",
		},
	})
	m.routeToA2A = false
	result := m.result()
	assert.Empty(t, result.Config.A2ADownstreamURL)
}

func TestResult_EnabledTogglesPreserveValues(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			AllowedOrigins:   []string{"https://console.g8e.ai"},
			MCPDownstreamURL: "http://mcp:3000",
			A2ADownstreamURL: "http://a2a:3001",
		},
	})
	result := m.result()
	assert.Equal(t, []string{"https://console.g8e.ai"}, result.Config.AllowedOrigins)
	assert.Equal(t, "http://mcp:3000", result.Config.MCPDownstreamURL)
	assert.Equal(t, "http://a2a:3001", result.Config.A2ADownstreamURL)
}

// --- result(): cancel ---

func TestResult_CancelReturnsCancelTrue(t *testing.T) {
	m := NewModel(Options{})
	m.quitting = true
	result := m.result()
	assert.True(t, result.Cancel)
}

// --- result(): conditional consensus ---

func TestResult_ConditionalConsensus_DoctrineKeepsValuesFromInputs(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			Posture:      "doctrine",
			ConsensusID:  "stale-id",
			ConsensusURL: "https://stale.example.com",
		},
	})
	result := m.result()
	assert.Equal(t, "doctrine", result.Config.Posture)
	assert.Equal(t, "stale-id", result.Config.ConsensusID)
}

func TestResult_ConditionalConsensus_ConsensusPreservesValues(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			Posture:      "consensus",
			ConsensusID:  "trib-prod-01",
			ConsensusURL: "https://consensus.g8e.ai",
		},
	})
	result := m.result()
	assert.Equal(t, "consensus", result.Config.Posture)
	assert.Equal(t, "trib-prod-01", result.Config.ConsensusID)
	assert.Equal(t, "https://consensus.g8e.ai", result.Config.ConsensusURL)
}

// --- newTextInput ---

func TestNewTextInput_SetsPlaceholder(t *testing.T) {
	ti := newTextInput("https://example.com", "Label", 40)
	assert.Equal(t, "https://example.com", ti.Placeholder)
}

func TestNewTextInput_SetsPrompt(t *testing.T) {
	ti := newTextInput("placeholder", "My Label", 40)
	assert.Equal(t, "My Label: ", ti.Prompt)
}

func TestNewTextInput_SetsWidth(t *testing.T) {
	ti := newTextInput("placeholder", "Label", 50)
	assert.Equal(t, 50, ti.Width)
}

func TestNewTextInput_SetsCharLimit(t *testing.T) {
	ti := newTextInput("placeholder", "Label", 40)
	assert.Equal(t, 200, ti.CharLimit)
}

// --- Model implements tea.Model ---

func TestModel_ImplementsTeaModel(t *testing.T) {
	var _ tea.Model = Model{}
}

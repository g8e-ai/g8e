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

func TestNewModel_TribunalIDPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{TribunalID: "trib-prod-01"},
	})
	assert.Equal(t, "trib-prod-01", m.tribunalIDInput.Value())
}

func TestNewModel_TribunalURLPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{TribunalURL: "https://tribunal.g8e.ai"},
	})
	assert.Equal(t, "https://tribunal.g8e.ai", m.tribunalURLInput.Value())
}

func TestNewModel_TribunalBootstrapPreFilled(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{TribunalBootstrap: "/etc/g8e/bootstrap.json"},
	})
	assert.Equal(t, "/etc/g8e/bootstrap.json", m.tribunalBootstrapInput.Value())
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
			PublicBaseURL:     "https://demo.g8e.ai",
			CertIdentityMode:  "full",
			AllowedOrigins:    []string{"https://console.g8e.ai"},
			Posture:           "consensus",
			TribunalID:        "trib-prod-01",
			TribunalURL:       "https://tribunal.g8e.ai",
			TribunalBootstrap: "/etc/g8e/bootstrap.json",
			PasskeyRpID:       "demo.g8e.ai",
			PasskeyRpName:     "g8e",
			PasskeyRpOrigins:  []string{"https://demo.g8e.ai"},
			MCPDownstreamURL:  "http://mcp:3000",
			A2ADownstreamURL:  "http://a2a:3001",
		},
	})

	result := m.result()
	assert.False(t, result.Cancel)
	cfg := result.Config
	assert.Equal(t, "https://demo.g8e.ai", cfg.PublicBaseURL)
	assert.Equal(t, "full", cfg.CertIdentityMode)
	assert.Equal(t, []string{"https://console.g8e.ai"}, cfg.AllowedOrigins)
	assert.Equal(t, "consensus", cfg.Posture)
	assert.Equal(t, "trib-prod-01", cfg.TribunalID)
	assert.Equal(t, "https://tribunal.g8e.ai", cfg.TribunalURL)
	assert.Equal(t, "/etc/g8e/bootstrap.json", cfg.TribunalBootstrap)
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

// --- result(): conditional tribunal ---

func TestResult_ConditionalTribunal_DoctrineKeepsValuesFromInputs(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			Posture:     "doctrine",
			TribunalID:  "stale-id",
			TribunalURL: "https://stale.example.com",
		},
	})
	result := m.result()
	assert.Equal(t, "doctrine", result.Config.Posture)
	assert.Equal(t, "stale-id", result.Config.TribunalID)
}

func TestResult_ConditionalTribunal_ConsensusPreservesValues(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			Posture:     "consensus",
			TribunalID:  "trib-prod-01",
			TribunalURL: "https://tribunal.g8e.ai",
		},
	})
	result := m.result()
	assert.Equal(t, "consensus", result.Config.Posture)
	assert.Equal(t, "trib-prod-01", result.Config.TribunalID)
	assert.Equal(t, "https://tribunal.g8e.ai", result.Config.TribunalURL)
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

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

	"github.com/stretchr/testify/assert"
)

// --- View: step routing ---

func TestView_NetworkStep_Renders(t *testing.T) {
	m := NewModel(Options{})
	s := m.View()
	assert.Contains(t, s, "Network & Identity")
	assert.Contains(t, s, "Public base URL")
}

func TestView_PostureStep_Renders(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	s := m.View()
	assert.Contains(t, s, "Security & Governance Posture")
}

func TestView_RoutingStep_Renders(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	s := m.View()
	assert.Contains(t, s, "Agent Tooling & Routing")
}

func TestView_ReviewStep_Renders(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			PublicBaseURL: "https://demo.g8e.ai",
			Posture:       "consensus",
		},
	})
	m.step = StepReview
	s := m.View()
	assert.Contains(t, s, "GATEWAY CONFIGURATION REVIEW")
	assert.Contains(t, s, "https://demo.g8e.ai")
	assert.Contains(t, s, "consensus")
}

func TestView_Quitting_RendersCancelled(t *testing.T) {
	m := NewModel(Options{})
	m.quitting = true
	s := m.View()
	assert.Contains(t, s, "cancelled")
}

func TestView_UnknownStep_ReturnsEmpty(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepDone
	s := m.View()
	assert.Empty(t, s)
}

// --- View: cert mode focus indicator (Bug #1) ---

func TestView_CertModeFocusIndicatorAtFocusIndexOne(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	s := m.View()
	assert.Contains(t, s, "▶▶", "cert mode choice should show focused indicator at focusIndex 1")
}

func TestView_CertModeNoFocusIndicatorAtFocusIndexZero(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	s := m.View()
	assert.NotContains(t, s, "▶▶", "cert mode choice should not show double indicator at focusIndex 0")
}

// --- View: conditional rendering — posture tribunal fields ---

func TestView_PostureDoctrine_TribunalFieldsHidden(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 0
	s := m.View()
	assert.NotContains(t, s, "Tribunal Policy ID")
	assert.NotContains(t, s, "Tribunal Service URL")
}

func TestView_PostureConsensus_TribunalFieldsShown(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 1
	s := m.View()
	assert.Contains(t, s, "Tribunal Policy ID")
	assert.Contains(t, s, "Tribunal Service URL")
}

func TestView_PostureNotary_TribunalFieldsShown(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 2
	s := m.View()
	assert.Contains(t, s, "Tribunal Policy ID")
	assert.Contains(t, s, "Tribunal Service URL")
}

// --- View: conditional rendering — web frontend / CORS ---

func TestView_WebFrontendNo_CorsFieldHidden(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = false
	s := m.View()
	assert.NotContains(t, s, "Allowed CORS origin")
}

func TestView_WebFrontendYes_CorsFieldShown(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = true
	s := m.View()
	assert.Contains(t, s, "Allowed CORS origin")
}

// --- View: conditional rendering — routing toggles ---

func TestView_RoutingMCPDisabled_MCPInputHidden(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = false
	s := m.View()
	assert.NotContains(t, s, "MCP Server URL")
}

func TestView_RoutingMCPEnabled_MCPInputShown(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = true
	s := m.View()
	assert.Contains(t, s, "MCP Server URL")
}

func TestView_RoutingA2ADisabled_A2AInputHidden(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = false
	s := m.View()
	assert.NotContains(t, s, "A2A Server URL")
}

func TestView_RoutingA2AEnabled_A2AInputShown(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = true
	s := m.View()
	assert.Contains(t, s, "A2A Server URL")
}

// --- View: review shows actual config, not stale ---

func TestView_ReviewShowsActualConfig(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepReview
	m.publicBaseURLInput.SetValue("https://actual.example.com")
	m.postureChoice = 2
	s := m.View()
	assert.Contains(t, s, "https://actual.example.com")
	assert.Contains(t, s, "notary")
}

// --- renderChoice ---

func TestRenderChoice_FocusedShowsDoubleIndicator(t *testing.T) {
	s := renderChoice([]string{"A", "B"}, 0, true)
	assert.Contains(t, s, "▶▶")
}

func TestRenderChoice_NotFocusedShowsSingleIndicator(t *testing.T) {
	s := renderChoice([]string{"A", "B"}, 0, false)
	assert.Contains(t, s, "▶ ")
	assert.NotContains(t, s, "▶▶")
}

func TestRenderChoice_SelectedItemHighlighted(t *testing.T) {
	s := renderChoice([]string{"First", "Second"}, 1, false)
	assert.Contains(t, s, "Second")
}

func TestRenderChoice_UnselectedItemMuted(t *testing.T) {
	s := renderChoice([]string{"First", "Second"}, 1, false)
	assert.Contains(t, s, "First")
}

// --- renderToggle ---

func TestRenderToggle_FocusedShowsIndicator(t *testing.T) {
	s := renderToggle("Test Label", true, true)
	assert.Contains(t, s, "▶")
}

func TestRenderToggle_NotFocusedNoIndicator(t *testing.T) {
	s := renderToggle("Test Label", true, false)
	assert.NotContains(t, s, "▶ Test Label")
}

func TestRenderToggle_YesValueHighlighted(t *testing.T) {
	s := renderToggle("Test Label", true, false)
	assert.Contains(t, s, "Yes")
}

func TestRenderToggle_NoValueHighlighted(t *testing.T) {
	s := renderToggle("Test Label", false, false)
	assert.Contains(t, s, "No")
}

// --- certModeLabel ---

func TestCertModeLabel_Full(t *testing.T) {
	assert.Equal(t, "Full Network", certModeLabel("full"))
}

func TestCertModeLabel_Localhost(t *testing.T) {
	assert.Equal(t, "Local Development Only", certModeLabel("localhost"))
}

func TestCertModeLabel_UnknownDefaultsToLocal(t *testing.T) {
	assert.Equal(t, "Local Development Only", certModeLabel("unknown"))
}

func TestCertModeLabel_EmptyDefaultsToLocal(t *testing.T) {
	assert.Equal(t, "Local Development Only", certModeLabel(""))
}

// --- joinOrNone ---

func TestJoinOrNone_EmptyReturnsNone(t *testing.T) {
	assert.Equal(t, "(none)", joinOrNone(nil))
}

func TestJoinOrNone_SingleValue(t *testing.T) {
	assert.Equal(t, "https://example.com", joinOrNone([]string{"https://example.com"}))
}

func TestJoinOrNone_MultipleValues(t *testing.T) {
	assert.Equal(t, "a, b, c", joinOrNone([]string{"a", "b", "c"}))
}

// --- orNone ---

func TestOrNone_EmptyReturnsNone(t *testing.T) {
	assert.Equal(t, "(none)", orNone(""))
}

func TestOrNone_NonEmptyReturnsValue(t *testing.T) {
	assert.Equal(t, "https://example.com", orNone("https://example.com"))
}

// --- yesNo ---

func TestYesNo_TrueReturnsYes(t *testing.T) {
	assert.Equal(t, "Yes", yesNo(true))
}

func TestYesNo_FalseReturnsNo(t *testing.T) {
	assert.Equal(t, "No", yesNo(false))
}

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

// --- Update: Ctrl+C cancel ---

func TestUpdate_CtrlC_SetsQuitting(t *testing.T) {
	m := NewModel(Options{})
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	final := m2.(Model)
	assert.True(t, final.quitting)
	assert.NotNil(t, cmd)
}

func TestUpdate_CtrlC_ResultIsCancelled(t *testing.T) {
	m := NewModel(Options{})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	result := m2.(Model).result()
	assert.True(t, result.Cancel)
}

// --- Update: Esc back navigation ---

func TestUpdate_EscFromRouting_GoesToPosture(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, StepPosture, m2.(Model).step)
}

func TestUpdate_EscFromPosture_GoesToNetwork(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, StepNetwork, m2.(Model).step)
}

func TestUpdate_EscFromNetwork_NoOp(t *testing.T) {
	m := NewModel(Options{})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, StepNetwork, m2.(Model).step)
}

func TestUpdate_Esc_ResetsValidationError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.validationError = "some error"
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Empty(t, m2.(Model).validationError)
}

func TestUpdate_Esc_ResetsFocusIndex(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.focusIndex = 3
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, 0, m2.(Model).focusIndex)
}

// --- Update: Enter step transitions ---

func TestUpdate_EnterFromNetwork_AdvancesToPosture(t *testing.T) {
	m := NewModel(Options{InitialConfig: Config{PublicBaseURL: "https://demo.g8e.ai"}})
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		m2, _ = m2.Update(msg)
	}
	assert.Equal(t, StepPosture, m2.(Model).step)
}

func TestUpdate_EnterFromPosture_AdvancesToRouting(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		m2, _ = m2.Update(msg)
	}
	assert.Equal(t, StepRouting, m2.(Model).step)
}

func TestUpdate_EnterFromRouting_AdvancesToReview(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		m2, _ = m2.Update(msg)
	}
	assert.Equal(t, StepReview, m2.(Model).step)
}

func TestUpdate_EnterFromReview_SetsConfirmedAndQuit(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepReview
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m2.(Model).reviewConfirmed)
	assert.NotNil(t, cmd)
}

// --- Update: Enter validation failures ---

func TestUpdate_EnterFromNetwork_InvalidURL_KeepsStep(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("not-a-url")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, StepNetwork, m2.(Model).step)
	assert.NotEmpty(t, m2.(Model).validationError)
}

func TestUpdate_EnterFromNetwork_EmptyURL_KeepsStep(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, StepNetwork, m2.(Model).step)
	assert.NotEmpty(t, m2.(Model).validationError)
}

func TestUpdate_EnterFromRouting_MCPEnabled_InvalidURL_KeepsStep(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = true
	m.mcpDownstreamInput.SetValue("not-a-url")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, StepRouting, m2.(Model).step)
	assert.NotEmpty(t, m2.(Model).validationError)
}

func TestUpdate_EnterFromPosture_Consensus_EmptyConsensusID_KeepsStep(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 1
	m.consensusIDInput.SetValue("")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, StepPosture, m2.(Model).step)
	assert.NotEmpty(t, m2.(Model).validationError)
}

// --- Update: stepTransitionMsg ---

func TestUpdate_StepTransitionMsg_AdvancesStep(t *testing.T) {
	m := NewModel(Options{})
	m2, _ := m.Update(stepTransitionMsg{to: StepPosture})
	assert.Equal(t, StepPosture, m2.(Model).step)
	assert.Equal(t, 0, m2.(Model).focusIndex)
}

func TestUpdate_StepTransitionMsg_DoneReturnsQuit(t *testing.T) {
	m := NewModel(Options{})
	m2, cmd := m.Update(stepTransitionMsg{to: StepDone})
	assert.Equal(t, StepDone, m2.(Model).step)
	assert.NotNil(t, cmd)
}

func TestUpdate_StepTransitionMsg_ClearsValidationError(t *testing.T) {
	m := NewModel(Options{})
	m.validationError = "some error"
	m2, _ := m.Update(stepTransitionMsg{to: StepPosture})
	assert.Empty(t, m2.(Model).validationError)
}

// --- Update: WindowSizeMsg ---

func TestUpdate_WindowSizeMsg_SetsDimensions(t *testing.T) {
	m := NewModel(Options{})
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Equal(t, 120, m2.(Model).width)
	assert.Equal(t, 40, m2.(Model).height)
}

// --- handleUp: cert mode (Bug #1) ---

func TestHandleUp_CertModeAtFocusIndexOne_Decrements(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	m.certModeChoice = 1
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m2.(Model).certModeChoice)
}

func TestHandleUp_CertModeAtFocusIndexOne_ClampsAtZero(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	m.certModeChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m2.(Model).certModeChoice)
}

func TestHandleUp_CertModeAtFocusIndexZero_NoOp(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	m.certModeChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m2.(Model).certModeChoice)
}

// --- handleDown: cert mode (Bug #1) ---

func TestHandleDown_CertModeAtFocusIndexOne_Increments(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	m.certModeChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m2.(Model).certModeChoice)
}

func TestHandleDown_CertModeAtFocusIndexOne_ClampsAtOne(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	m.certModeChoice = 1
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m2.(Model).certModeChoice)
}

func TestHandleDown_CertModeAtFocusIndexZero_NoOp(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	m.certModeChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 0, m2.(Model).certModeChoice, "Down at focusIndex 0 should not change cert mode")
}

// --- handleUp/handleDown: web frontend toggle ---

func TestHandleUp_WebFrontendToggle_TogglesOn(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 2
	assert.False(t, m.needsWebFrontend)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.True(t, m2.(Model).needsWebFrontend)
}

func TestHandleDown_WebFrontendToggle_TogglesOff(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 2
	m.needsWebFrontend = true
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.False(t, m2.(Model).needsWebFrontend)
}

// --- handleUp/handleDown: posture choice ---

func TestHandleUp_PostureChoice_Decrements(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 0
	m.postureChoice = 2
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, m2.(Model).postureChoice)
}

func TestHandleUp_PostureChoice_ClampsAtZero(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 0
	m.postureChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m2.(Model).postureChoice)
}

func TestHandleDown_PostureChoice_Increments(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 0
	m.postureChoice = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m2.(Model).postureChoice)
}

func TestHandleDown_PostureChoice_ClampsAtThree(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 0
	m.postureChoice = 3
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 3, m2.(Model).postureChoice)
}

// --- handleUp/handleDown: routing toggles ---

func TestHandleUp_RouteToMCP_TogglesOn(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.focusIndex = 0
	assert.False(t, m.routeToMCP)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.True(t, m2.(Model).routeToMCP)
}

func TestHandleDown_RouteToMCP_TogglesOff(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.focusIndex = 0
	m.routeToMCP = true
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.False(t, m2.(Model).routeToMCP)
}

func TestHandleUp_RouteToA2A_TogglesOn(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.focusIndex = 2
	assert.False(t, m.routeToA2A)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.True(t, m2.(Model).routeToA2A)
}

func TestHandleDown_RouteToA2A_TogglesOff(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.focusIndex = 2
	m.routeToA2A = true
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.False(t, m2.(Model).routeToA2A)
}

// --- handleTab: focus wrapping (Bug #6) ---

func TestHandleTab_NetworkStep_WrapsAtMaxWithoutWebFrontend(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 2
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, m2.(Model).focusIndex, "Tab should wrap from 2 to 0")
}

func TestHandleTab_NetworkStep_WrapsAtMaxWithWebFrontend(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = true
	m.focusIndex = 3
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, m2.(Model).focusIndex, "Tab should wrap from 3 to 0")
}

func TestHandleTab_NetworkStep_AdvancesFromZero(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 1, m2.(Model).focusIndex)
}

func TestHandleTab_PostureStep_WrapsAtMax(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 6
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, m2.(Model).focusIndex, "Tab should wrap from 6 to 0")
}

func TestHandleTab_RoutingStep_WrapsWithA2A(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = true
	m.focusIndex = 3
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, m2.(Model).focusIndex, "Tab should wrap from 3 to 0")
}

func TestHandleTab_RoutingStep_WrapsWithoutA2A(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = false
	m.focusIndex = 2
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, m2.(Model).focusIndex, "Tab should wrap from 2 to 0")
}

// --- handleShiftTab: focus wrapping (Bug #6) ---

func TestHandleShiftTab_NetworkStep_WrapsFromZeroToMax(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 2, m2.(Model).focusIndex, "ShiftTab should wrap from 0 to max (2)")
}

func TestHandleShiftTab_NetworkStep_WrapsFromZeroToMaxWithWebFrontend(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = true
	m.focusIndex = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 3, m2.(Model).focusIndex, "ShiftTab should wrap from 0 to max (3)")
}

func TestHandleShiftTab_NetworkStep_DecrementsFromOne(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 1
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 0, m2.(Model).focusIndex)
}

func TestHandleShiftTab_PostureStep_WrapsFromZeroToMax(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.focusIndex = 0
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 6, m2.(Model).focusIndex)
}

// --- stepFocusMax (Bug #6) ---

func TestStepFocusMax_NetworkWithoutWebFrontend(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = false
	assert.Equal(t, 2, m.stepFocusMax())
}

func TestStepFocusMax_NetworkWithWebFrontend(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.needsWebFrontend = true
	assert.Equal(t, 3, m.stepFocusMax())
}

func TestStepFocusMax_Posture(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	assert.Equal(t, 6, m.stepFocusMax())
}

func TestStepFocusMax_RoutingWithoutA2A(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = false
	assert.Equal(t, 2, m.stepFocusMax())
}

func TestStepFocusMax_RoutingWithA2A(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = true
	assert.Equal(t, 3, m.stepFocusMax())
}

func TestStepFocusMax_Review(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepReview
	assert.Equal(t, 0, m.stepFocusMax())
}

// --- handleTextInput: clears validation error ---

func TestHandleTextInput_ClearsValidationError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepNetwork
	m.focusIndex = 0
	m.validationError = "some error"
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.Empty(t, m2.(Model).validationError)
}

// --- validateNetwork ---

func TestValidateNetwork_ValidURL_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("https://demo.g8e.ai")
	assert.Empty(t, m.validateNetwork())
}

func TestValidateNetwork_EmptyURL_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("")
	assert.NotEmpty(t, m.validateNetwork())
}

func TestValidateNetwork_InvalidURL_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("not-a-url")
	assert.NotEmpty(t, m.validateNetwork())
}

func TestValidateNetwork_WebFrontendEnabled_EmptyCORS_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("https://demo.g8e.ai")
	m.needsWebFrontend = true
	m.corsOriginInput.SetValue("")
	assert.NotEmpty(t, m.validateNetwork())
}

func TestValidateNetwork_WebFrontendEnabled_InvalidCORS_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("https://demo.g8e.ai")
	m.needsWebFrontend = true
	m.corsOriginInput.SetValue("https://console.g8e.ai/path")
	assert.NotEmpty(t, m.validateNetwork())
}

func TestValidateNetwork_WebFrontendEnabled_ValidCORS_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.publicBaseURLInput.SetValue("https://demo.g8e.ai")
	m.needsWebFrontend = true
	m.corsOriginInput.SetValue("https://console.g8e.ai")
	assert.Empty(t, m.validateNetwork())
}

// --- validatePosture ---

func TestValidatePosture_Doctrine_NoConsensusRequired(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 0
	m.passkeyRpNameInput.SetValue("g8e")
	assert.Empty(t, m.validatePosture())
}

func TestValidatePosture_Ratify_NoConsensusRequired(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 2
	m.passkeyRpNameInput.SetValue("g8e")
	assert.Empty(t, m.validatePosture())
}

func TestValidatePosture_Consensus_RequiresConsensusID(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 1
	m.consensusIDInput.SetValue("")
	m.passkeyRpNameInput.SetValue("g8e")
	assert.NotEmpty(t, m.validatePosture())
}

func TestValidatePosture_Consensus_ValidConsensus_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 1
	m.consensusIDInput.SetValue("trib-prod-01")
	m.passkeyRpNameInput.SetValue("g8e")
	assert.Empty(t, m.validatePosture())
}

func TestValidatePosture_EmptyRpName_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 0
	m.passkeyRpNameInput.SetValue("")
	assert.NotEmpty(t, m.validatePosture())
}

func TestValidatePosture_InvalidPasskeyOrigin_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepPosture
	m.postureChoice = 0
	m.passkeyRpNameInput.SetValue("g8e")
	m.passkeyRpOriginInput.SetValue("not-a-url")
	assert.NotEmpty(t, m.validatePosture())
}

// --- validateRouting ---

func TestValidateRouting_NoToggles_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = false
	m.routeToA2A = false
	assert.Empty(t, m.validateRouting())
}

func TestValidateRouting_MCPEnabled_EmptyURL_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = true
	m.mcpDownstreamInput.SetValue("")
	assert.NotEmpty(t, m.validateRouting())
}

func TestValidateRouting_MCPEnabled_ValidURL_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToMCP = true
	m.mcpDownstreamInput.SetValue("http://mcp:3000")
	assert.Empty(t, m.validateRouting())
}

func TestValidateRouting_A2AEnabled_EmptyURL_ReturnsError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = true
	m.a2aDownstreamInput.SetValue("")
	assert.NotEmpty(t, m.validateRouting())
}

func TestValidateRouting_A2AEnabled_ValidURL_NoError(t *testing.T) {
	m := NewModel(Options{})
	m.step = StepRouting
	m.routeToA2A = true
	m.a2aDownstreamInput.SetValue("http://a2a:3001")
	assert.Empty(t, m.validateRouting())
}

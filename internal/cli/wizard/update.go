// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles all messages for the wizard state machine.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEsc:
			// Go back to previous step (no validation on back)
			if m.step == StepNetwork {
				// Already at first step — no-op
				return m, nil
			}
			m.step = prevStep(m.step)
			m.validationError = ""
			m.focusIndex = 0
			focusFirstInput(&m)
			return m, nil

		case tea.KeyEnter:
			return m.handleEnter()

		case tea.KeyUp:
			return m.handleUp()

		case tea.KeyDown:
			return m.handleDown()

		case tea.KeyTab:
			return m.handleTab()

		case tea.KeyShiftTab:
			return m.handleShiftTab()

		default:
			// Forward text input to the currently focused textinput
			return m.handleTextInput(msg)
		}

	case stepTransitionMsg:
		m.step = msg.to
		m.validationError = ""
		m.focusIndex = 0
		focusFirstInput(&m)
		if m.step == StepDone {
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

// handleEnter processes Enter key based on the current step.
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepNetwork:
		if err := m.validateNetwork(); err != "" {
			m.validationError = err
			return m, nil
		}
		return m, func() tea.Msg { return stepTransitionMsg{to: nextStep(StepNetwork)} }

	case StepPosture:
		if err := m.validatePosture(); err != "" {
			m.validationError = err
			return m, nil
		}
		return m, func() tea.Msg { return stepTransitionMsg{to: nextStep(StepPosture)} }

	case StepRouting:
		if err := m.validateRouting(); err != "" {
			m.validationError = err
			return m, nil
		}
		return m, func() tea.Msg { return stepTransitionMsg{to: nextStep(StepRouting)} }

	case StepReview:
		m.reviewConfirmed = true
		return m, tea.Quit

	case StepDone:
		return m, tea.Quit
	}

	return m, nil
}

// handleUp processes Up arrow for choice navigation and toggles.
func (m Model) handleUp() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepNetwork:
		switch m.focusIndex {
		case 1:
			if m.certModeChoice > 0 {
				m.certModeChoice--
			}
		case 2:
			m.needsWebFrontend = !m.needsWebFrontend
		}
	case StepPosture:
		if m.focusIndex == 0 {
			if m.postureChoice > 0 {
				m.postureChoice--
			}
		}
	case StepRouting:
		switch m.focusIndex {
		case 0:
			m.routeToMCP = !m.routeToMCP
		case 2:
			m.routeToA2A = !m.routeToA2A
		}
	}
	return m, nil
}

// handleDown processes Down arrow for choice navigation and toggles.
func (m Model) handleDown() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepNetwork:
		switch m.focusIndex {
		case 1:
			if m.certModeChoice < 1 {
				m.certModeChoice++
			}
		case 2:
			m.needsWebFrontend = !m.needsWebFrontend
		}
	case StepPosture:
		if m.focusIndex == 0 {
			if m.postureChoice < 2 {
				m.postureChoice++
			}
		}
	case StepRouting:
		switch m.focusIndex {
		case 0:
			m.routeToMCP = !m.routeToMCP
		case 2:
			m.routeToA2A = !m.routeToA2A
		}
	}
	return m, nil
}

// handleTab cycles focus to the next input field within a step.
func (m Model) handleTab() (tea.Model, tea.Cmd) {
	blurAllInputs(&m)
	max := m.stepFocusMax()
	m.focusIndex = (m.focusIndex + 1) % (max + 1)
	focusFirstInput(&m)
	return m, nil
}

// handleShiftTab cycles focus to the previous input field within a step.
func (m Model) handleShiftTab() (tea.Model, tea.Cmd) {
	blurAllInputs(&m)
	max := m.stepFocusMax()
	m.focusIndex = (m.focusIndex - 1 + max + 1) % (max + 1)
	focusFirstInput(&m)
	return m, nil
}

// stepFocusMax returns the maximum focusIndex for the current step,
// accounting for conditional fields that are only visible when their
// toggle is enabled.
func (m Model) stepFocusMax() int {
	switch m.step {
	case StepNetwork:
		if m.needsWebFrontend {
			return 3
		}
		return 2
	case StepPosture:
		return 6
	case StepRouting:
		if m.routeToA2A {
			return 3
		}
		return 2
	default:
		return 0
	}
}

// handleTextInput forwards key events to the currently focused textinput.
func (m Model) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear validation error on any typing
	if m.validationError != "" {
		m.validationError = ""
	}

	var cmd tea.Cmd
	switch m.step {
	case StepNetwork:
		switch m.focusIndex {
		case 0:
			m.publicBaseURLInput, cmd = m.publicBaseURLInput.Update(msg)
		case 1:
			// cert mode is a choice, not text input
		case 2:
			// web frontend is a toggle, not text input
		case 3:
			if m.needsWebFrontend {
				m.corsOriginInput, cmd = m.corsOriginInput.Update(msg)
			}
		}
	case StepPosture:
		switch m.focusIndex {
		case 0:
			// posture is a choice, not text input
		case 1:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusIDInput, cmd = m.consensusIDInput.Update(msg)
			}
		case 2:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusBootstrapInput, cmd = m.consensusBootstrapInput.Update(msg)
			}
		case 3:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusURLInput, cmd = m.consensusURLInput.Update(msg)
			}
		case 4:
			m.passkeyRpIDInput, cmd = m.passkeyRpIDInput.Update(msg)
		case 5:
			m.passkeyRpNameInput, cmd = m.passkeyRpNameInput.Update(msg)
		case 6:
			m.passkeyRpOriginInput, cmd = m.passkeyRpOriginInput.Update(msg)
		}
	case StepRouting:
		switch m.focusIndex {
		case 0:
			// MCP toggle
		case 1:
			if m.routeToMCP {
				m.mcpDownstreamInput, cmd = m.mcpDownstreamInput.Update(msg)
			}
		case 2:
			// A2A toggle
		case 3:
			if m.routeToA2A {
				m.a2aDownstreamInput, cmd = m.a2aDownstreamInput.Update(msg)
			}
		}
	}
	return m, cmd
}

// blurAllInputs removes focus from all text inputs.
func blurAllInputs(m *Model) {
	m.publicBaseURLInput.Blur()
	m.corsOriginInput.Blur()
	m.consensusIDInput.Blur()
	m.consensusURLInput.Blur()
	m.consensusBootstrapInput.Blur()
	m.passkeyRpIDInput.Blur()
	m.passkeyRpNameInput.Blur()
	m.passkeyRpOriginInput.Blur()
	m.mcpDownstreamInput.Blur()
	m.a2aDownstreamInput.Blur()
}

// focusFirstInput focuses the text input corresponding to the current focusIndex.
func focusFirstInput(m *Model) {
	switch m.step {
	case StepNetwork:
		switch m.focusIndex {
		case 0:
			m.publicBaseURLInput.Focus()
		case 3:
			if m.needsWebFrontend {
				m.corsOriginInput.Focus()
			}
		}
	case StepPosture:
		switch m.focusIndex {
		case 1:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusIDInput.Focus()
			}
		case 2:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusBootstrapInput.Focus()
			}
		case 3:
			if m.postureChoice == 1 || m.postureChoice == 2 {
				m.consensusURLInput.Focus()
			}
		case 4:
			m.passkeyRpIDInput.Focus()
		case 5:
			m.passkeyRpNameInput.Focus()
		case 6:
			m.passkeyRpOriginInput.Focus()
		}
	case StepRouting:
		switch m.focusIndex {
		case 1:
			if m.routeToMCP {
				m.mcpDownstreamInput.Focus()
			}
		case 3:
			if m.routeToA2A {
				m.a2aDownstreamInput.Focus()
			}
		}
	}
}

// Validation methods — return error string or empty if valid.

func (m Model) validateNetwork() string {
	url := m.publicBaseURLInput.Value()
	if err := validatePublicBaseURL(url); err != nil {
		return err.Error()
	}
	if m.needsWebFrontend {
		origin := m.corsOriginInput.Value()
		if origin == "" {
			return "CORS origin is required when web frontend is enabled"
		}
		if err := validateCORSOrigin(origin); err != nil {
			return err.Error()
		}
	}
	return ""
}

func (m Model) validatePosture() string {
	if m.postureChoice == 1 || m.postureChoice == 2 {
		consensusID := m.consensusIDInput.Value()
		if err := validateConsensusID(consensusID); err != nil {
			return err.Error()
		}
		bootstrapPath := m.consensusBootstrapInput.Value()
		if bootstrapPath != "" {
			if err := validateConsensusBootstrap(bootstrapPath, consensusID); err != nil {
				return err.Error()
			}
		}
		consensusURL := m.consensusURLInput.Value()
		if err := validateConsensusURL(consensusURL); err != nil {
			return err.Error()
		}
	}
	rpID := m.passkeyRpIDInput.Value()
	rpName := m.passkeyRpNameInput.Value()
	if rpName == "" {
		return "Passkey RP Name is required"
	}
	rpOrigin := m.passkeyRpOriginInput.Value()
	if rpOrigin != "" {
		if err := validatePasskeyRP(rpID, rpOrigin); err != nil {
			return err.Error()
		}
	}
	return ""
}

func (m Model) validateRouting() string {
	if m.routeToMCP {
		url := m.mcpDownstreamInput.Value()
		if err := validateDownstreamURL(url); err != nil {
			return err.Error()
		}
	}
	if m.routeToA2A {
		url := m.a2aDownstreamInput.Value()
		if err := validateDownstreamURL(url); err != nil {
			return err.Error()
		}
	}
	return ""
}

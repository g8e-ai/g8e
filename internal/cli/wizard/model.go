// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the bubbletea state container for the onboarding wizard.
type Model struct {
	step     Step
	width    int
	height   int
	quitting bool

	// Collected values — populated as the user progresses through steps
	config Config

	// Step 1: Network
	publicBaseURLInput textinput.Model
	certModeChoice     int // 0=local, 1=full
	needsWebFrontend   bool
	corsOriginInput    textinput.Model

	// Step 2: Posture
	postureChoice           int // 0=doctrine, 1=consensus, 2=notary
	consensusURLInput       textinput.Model
	consensusIDInput        textinput.Model
	consensusBootstrapInput textinput.Model
	passkeyRpIDInput        textinput.Model
	passkeyRpNameInput      textinput.Model
	passkeyRpOriginInput    textinput.Model

	// Step 3: Routing
	routeToMCP         bool
	mcpDownstreamInput textinput.Model
	routeToA2A         bool
	a2aDownstreamInput textinput.Model

	// Step 4: Review
	reviewConfirmed bool

	// Validation
	validationError string

	// Track which input field is focused within a step (for Tab cycling)
	focusIndex int
}

// NewModel creates a new wizard model from the given options.
func NewModel(opts Options) Model {
	m := Model{
		step:   StepNetwork,
		config: opts.InitialConfig,
	}

	// Initialize text inputs with pre-filled values from the resolved config
	m.publicBaseURLInput = newTextInput("https://demo.g8e.ai", "Public base URL", 40)
	m.publicBaseURLInput.SetValue(opts.InitialConfig.PublicBaseURL)

	m.corsOriginInput = newTextInput("https://console.g8e.ai", "Allowed CORS origin", 40)
	if len(opts.InitialConfig.AllowedOrigins) > 0 {
		m.corsOriginInput.SetValue(opts.InitialConfig.AllowedOrigins[0])
	}

	m.consensusIDInput = newTextInput("trib-prod-01", "Consensus Policy ID", 30)
	m.consensusIDInput.SetValue(opts.InitialConfig.ConsensusID)

	m.consensusURLInput = newTextInput("https://consensus.g8e.ai", "Consensus Service URL (optional)", 40)
	m.consensusURLInput.SetValue(opts.InitialConfig.ConsensusURL)

	m.consensusBootstrapInput = newTextInput("", "Path to bootstrap JSON file (optional)", 50)
	m.consensusBootstrapInput.SetValue(opts.InitialConfig.ConsensusBootstrap)

	m.passkeyRpIDInput = newTextInput("localhost", "Passkey RP ID", 30)
	rpID := opts.InitialConfig.PasskeyRpID
	if rpID == "" {
		rpID = "localhost"
	}
	m.passkeyRpIDInput.SetValue(rpID)

	m.passkeyRpNameInput = newTextInput("g8e", "Passkey RP Name", 30)
	rpName := opts.InitialConfig.PasskeyRpName
	if rpName == "" {
		rpName = "g8e"
	}
	m.passkeyRpNameInput.SetValue(rpName)

	m.passkeyRpOriginInput = newTextInput("https://localhost:8443", "Passkey RP Origin", 40)
	if len(opts.InitialConfig.PasskeyRpOrigins) > 0 {
		m.passkeyRpOriginInput.SetValue(opts.InitialConfig.PasskeyRpOrigins[0])
	}

	m.mcpDownstreamInput = newTextInput("http://mcp:3000", "MCP Server URL", 40)
	m.mcpDownstreamInput.SetValue(opts.InitialConfig.MCPDownstreamURL)

	m.a2aDownstreamInput = newTextInput("http://a2a:3001", "A2A Server URL", 40)
	m.a2aDownstreamInput.SetValue(opts.InitialConfig.A2ADownstreamURL)

	// Set initial cert mode choice from config
	if opts.InitialConfig.CertIdentityMode == "full" {
		m.certModeChoice = 1
	}

	// Set initial posture choice from config
	switch opts.InitialConfig.Posture {
	case "consensus":
		m.postureChoice = 1
	case "notary":
		m.postureChoice = 2
	default:
		m.postureChoice = 0
	}

	// Initialize toggles from initial config so pre-filled inputs are visible
	m.routeToMCP = opts.InitialConfig.MCPDownstreamURL != ""
	m.routeToA2A = opts.InitialConfig.A2ADownstreamURL != ""
	m.needsWebFrontend = len(opts.InitialConfig.AllowedOrigins) > 0

	// Focus the first input
	m.publicBaseURLInput.Focus()
	m.focusIndex = 0

	return m
}

// Init returns the initial command for the wizard.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// result produces the final Result from the model state.
func (m Model) result() Result {
	if m.quitting {
		return Result{Cancel: true}
	}

	cfg := m.config

	// Step 1: Network
	cfg.PublicBaseURL = m.publicBaseURLInput.Value()
	if m.certModeChoice == 1 {
		cfg.CertIdentityMode = "full"
	} else {
		cfg.CertIdentityMode = "localhost"
	}
	if m.needsWebFrontend {
		cfg.AllowedOrigins = []string{m.corsOriginInput.Value()}
	} else {
		cfg.AllowedOrigins = nil
	}

	// Step 2: Posture
	switch m.postureChoice {
	case 1:
		cfg.Posture = "consensus"
	case 2:
		cfg.Posture = "notary"
	default:
		cfg.Posture = "doctrine"
	}
	cfg.ConsensusID = m.consensusIDInput.Value()
	cfg.ConsensusURL = m.consensusURLInput.Value()
	cfg.ConsensusBootstrap = m.consensusBootstrapInput.Value()
	cfg.PasskeyRpID = m.passkeyRpIDInput.Value()
	cfg.PasskeyRpName = m.passkeyRpNameInput.Value()
	if m.passkeyRpOriginInput.Value() != "" {
		cfg.PasskeyRpOrigins = []string{m.passkeyRpOriginInput.Value()}
	}

	// Step 3: Routing
	if m.routeToMCP {
		cfg.MCPDownstreamURL = m.mcpDownstreamInput.Value()
	} else {
		cfg.MCPDownstreamURL = ""
	}
	if m.routeToA2A {
		cfg.A2ADownstreamURL = m.a2aDownstreamInput.Value()
	} else {
		cfg.A2ADownstreamURL = ""
	}

	return Result{Config: cfg}
}

// newTextInput creates a styled textinput.Model with the given placeholder and width.
func newTextInput(placeholder, label string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = label + ": "
	ti.Width = width
	ti.CharLimit = 200
	return ti
}

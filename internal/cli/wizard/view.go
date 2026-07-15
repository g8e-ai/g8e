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
	"fmt"
	"strings"
)

// View renders the current wizard state.
func (m Model) View() string {
	if m.quitting {
		return "Onboarding cancelled.\n"
	}

	switch m.step {
	case StepNetwork:
		return m.viewNetwork()
	case StepPosture:
		return m.viewPosture()
	case StepRouting:
		return m.viewRouting()
	case StepVault:
		return m.viewVault()
	case StepReview:
		return m.viewReview()
	default:
		return ""
	}
}

// renderHeader renders the step header with progress indicator.
func (m Model) renderHeader() string {
	def, ok := stepDefs[m.step]
	if !ok {
		return ""
	}
	num := stepNumber(m.step)
	total := len(stepOrder)
	return headerStyle.Render(fmt.Sprintf("Step %d of %d: %s", num, total, def.title)) +
		"\n" + mutedStyle.Render(def.subtitle) + "\n"
}

// renderFooter renders navigation hints.
func (m Model) renderFooter() string {
	return footerStyle.Render("↑↓ navigate · Enter confirm · Tab next field · Esc back · Ctrl+C cancel")
}

// renderValidationError renders the validation error if present.
func (m Model) renderValidationError() string {
	if m.validationError != "" {
		return "\n" + errorStyle.Render("✗ "+m.validationError) + "\n"
	}
	return ""
}

// renderChoice renders a choice list with the selected item highlighted.
// When focused is true, a focus indicator shows the active field.
func renderChoice(items []string, selected int, focused bool) string {
	var b strings.Builder
	for i, item := range items {
		if i == selected {
			prefix := "▶ "
			if focused {
				prefix = "▶▶"
			}
			b.WriteString(selectedStyle.Render(prefix + item))
		} else {
			b.WriteString(mutedStyle.Render("  " + item))
		}
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderToggle renders a Yes/No toggle with the current value highlighted.
// When focused is true, a focus indicator shows the active field.
func renderToggle(label string, value bool, focused bool) string {
	yes := "Yes"
	no := "No"
	indicator := "  "
	if focused {
		indicator = "▶ "
	}
	if value {
		return indicator + label + ": " + selectedStyle.Render("▶ "+yes) + "  " + mutedStyle.Render("  "+no)
	}
	return indicator + label + ": " + mutedStyle.Render("  "+yes) + "  " + selectedStyle.Render("▶ "+no)
}

func (m Model) viewNetwork() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString(m.publicBaseURLInput.View())
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render("How will this gateway be exposed to the network?"))
	b.WriteString("\n")
	b.WriteString(renderChoice([]string{"Local Development Only", "Full Network"}, m.certModeChoice, m.focusIndex == 0))
	b.WriteString("\n\n")

	b.WriteString(renderToggle("Do you need a web frontend to connect from a browser?", m.needsWebFrontend, m.focusIndex == 2))
	b.WriteString("\n")

	if m.needsWebFrontend {
		b.WriteString("\n")
		b.WriteString(m.corsOriginInput.View())
	}

	b.WriteString(m.renderValidationError())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return borderStyle.Render(b.String())
}

func (m Model) viewPosture() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString(mutedStyle.Render("Select your required Governance Posture:"))
	b.WriteString("\n")
	postures := []string{
		"Doctrine — L1 enforced, L2/L3 audited (default)",
		"Consensus — L1+L2 enforced, L3 audited",
		"Notary — L1+L2+L3 strictly enforced",
	}
	b.WriteString(renderChoice(postures, m.postureChoice, m.focusIndex == 0))
	b.WriteString("\n\n")

	if m.postureChoice == 1 || m.postureChoice == 2 {
		b.WriteString(m.tribunalIDInput.View())
		b.WriteString("\n\n")
		b.WriteString(m.tribunalBootstrapInput.View())
		b.WriteString("\n\n")
		b.WriteString(m.tribunalURLInput.View())
		b.WriteString("\n\n")
	}

	b.WriteString(m.passkeyRpIDInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.passkeyRpNameInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.passkeyRpOriginInput.View())

	b.WriteString(m.renderValidationError())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return borderStyle.Render(b.String())
}

func (m Model) viewRouting() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString(renderToggle("Route to an MCP Server?", m.routeToMCP, m.focusIndex == 0))
	b.WriteString("\n")
	if m.routeToMCP {
		b.WriteString("\n")
		b.WriteString(m.mcpDownstreamInput.View())
	}
	b.WriteString("\n\n")

	b.WriteString(renderToggle("Route to an A2A Server?", m.routeToA2A, m.focusIndex == 2))
	b.WriteString("\n")
	if m.routeToA2A {
		b.WriteString("\n")
		b.WriteString(m.a2aDownstreamInput.View())
	}

	b.WriteString(m.renderValidationError())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return borderStyle.Render(b.String())
}

func (m Model) viewVault() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString("Should the gateway refuse to start if the secure vault cannot be unlocked?\n\n")
	b.WriteString(renderToggle("Vault Strict Mode", m.vaultRequireUnlock, true))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Fail startup if the configured vault key cannot be read or used."))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("This does not initialize a vault or create a key."))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Configure or initialize the vault before enabling this option."))

	b.WriteString(m.renderValidationError())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return borderStyle.Render(b.String())
}

func (m Model) viewReview() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	b.WriteString(headerStyle.Render("GATEWAY CONFIGURATION REVIEW"))
	b.WriteString("\n\n")

	cfg := m.result().Config
	rows := [][2]string{
		{"Public Base URL", cfg.PublicBaseURL},
		{"Network Exposure", certModeLabel(cfg.CertIdentityMode)},
		{"CORS Origins", joinOrNone(cfg.AllowedOrigins)},
		{"Posture", cfg.Posture},
		{"Tribunal ID", orNone(cfg.TribunalID)},
		{"Tribunal URL", orNone(cfg.TribunalURL)},
		{"Tribunal Bootstrap", orNone(cfg.TribunalBootstrap)},
		{"Passkey RP ID", cfg.PasskeyRpID},
		{"Passkey RP Name", cfg.PasskeyRpName},
		{"Passkey RP Origins", joinOrNone(cfg.PasskeyRpOrigins)},
		{"MCP Downstream", orNone(cfg.MCPDownstreamURL)},
		{"A2A Downstream", orNone(cfg.A2ADownstreamURL)},
		{"Vault Strict", yesNo(cfg.VaultRequireUnlock)},
	}

	for _, row := range rows {
		b.WriteString(labelStyle.Render(row[0]))
		b.WriteString(valueStyle.Render(row[1]))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(passedStyle.Render("Press Enter to start · Esc to go back"))

	return borderStyle.Render(b.String())
}

func certModeLabel(mode string) string {
	if mode == "full" {
		return "Full Network"
	}
	return "Local Development Only"
}

func joinOrNone(ss []string) string {
	if len(ss) == 0 {
		return "(none)"
	}
	return strings.Join(ss, ", ")
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

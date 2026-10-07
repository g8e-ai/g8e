// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import tea "github.com/charmbracelet/bubbletea"

// handleGatewayStatusKey applies movement on the read-only Gateway status
// view. The global r key performs the refresh used by the other views.
func (m Model) handleGatewayStatusKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.gatewayStatusSelected = clamp(m.gatewayStatusSelected-1, max(len(m.gatewayStatusWorkloads())-1, 0))
	case "down", "j":
		m.gatewayStatusSelected = clamp(m.gatewayStatusSelected+1, max(len(m.gatewayStatusWorkloads())-1, 0))
	}
	return m, nil
}

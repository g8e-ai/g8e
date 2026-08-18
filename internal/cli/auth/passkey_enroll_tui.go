// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// programSender is the interface for sending messages to a bubbletea program.
// *tea.Program implements this via its Send method. Tests use a channel-based
// mock to verify message delivery without running a real terminal program.
type programSender interface {
	Send(msg tea.Msg)
}

type passkeyRegisteredMsg struct{}

type enrollErrMsg struct {
	err error
}

type enrollTickMsg struct{}

const enrollTickInterval = 200 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(enrollTickInterval, func(time.Time) tea.Msg {
		return enrollTickMsg{}
	})
}

// enrollModel is a minimal bubbletea model for the passkey enrollment waiting UX.
// It displays a spinner, the console URL, and exits when a passkey.registered
// event is received, an error occurs, or the user cancels.
//
// On user cancellation (q/ctrl+c), the model exits with context.Canceled so
// the registrar can cancel the SSE context and clean up the stream.
type enrollModel struct {
	consoleURL string
	done       bool
	err        error
	tick       int
}

func newEnrollModel(consoleURL string) enrollModel {
	return enrollModel{consoleURL: consoleURL}
}

func (m enrollModel) Init() tea.Cmd {
	return tickCmd()
}

func (m enrollModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case enrollTickMsg:
		m.tick++
		return m, tickCmd()
	case passkeyRegisteredMsg:
		m.done = true
		return m, tea.Quit
	case enrollErrMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		key := msg.String()
		if key == "q" || key == "ctrl+c" {
			m.err = context.Canceled
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m enrollModel) View() string {
	if m.done {
		return "\n Passkey registered successfully!\n\n"
	}
	if m.err != nil {
		return fmt.Sprintf("\n Enrollment failed: %s\n\n", m.err)
	}
	spinner := spinnerChar(m.tick)
	return fmt.Sprintf(
		"\n %s Waiting for passkey registration...\n\n"+
			"   Console URL: %s\n\n"+
			"   Press q to cancel.\n",
		spinner, m.consoleURL,
	)
}

func spinnerChar(tick int) string {
	chars := []string{"|", "/", "-", "\\"}
	return chars[tick%len(chars)]
}

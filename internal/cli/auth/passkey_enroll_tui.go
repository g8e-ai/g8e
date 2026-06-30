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

package auth

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type passkeyRegisteredMsg struct{}

type enrollErrMsg struct {
	err error
}

type enrollTickMsg struct{}

// enrollModel is a minimal bubbletea model for the passkey enrollment waiting UX.
// It displays a spinner, the console URL, and exits when a passkey.registered
// event is received or the user cancels.
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
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return enrollTickMsg{}
	})
}

func (m enrollModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case enrollTickMsg:
		m.tick++
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return enrollTickMsg{}
		})
	case passkeyRegisteredMsg:
		m.done = true
		return m, tea.Quit
	case enrollErrMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
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

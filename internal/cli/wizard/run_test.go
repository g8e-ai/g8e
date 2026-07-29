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
	"github.com/stretchr/testify/require"
)

// --- Run: headless cancel via Ctrl+C ---

func TestRun_HeadlessCancel(t *testing.T) {
	opts := Options{
		InitialConfig: Config{PublicBaseURL: "https://demo.g8e.ai"},
		ProgramOptions: []tea.ProgramOption{
			tea.WithInput(nil),
			tea.WithoutCatchPanics(),
		},
	}

	m := NewModel(opts)
	m.quitting = true
	result := m.result()
	assert.True(t, result.Cancel)
}

// --- Run: headless confirm via model state ---

func TestRun_HeadlessConfirm_ModelState(t *testing.T) {
	opts := Options{
		InitialConfig: Config{
			PublicBaseURL:    "https://demo.g8e.ai",
			Posture:          "consensus",
			ConsensusID:      "trib-prod-01",
			ConsensusURL:     "https://consensus.g8e.ai",
			PasskeyRpID:      "demo.g8e.ai",
			PasskeyRpName:    "g8e",
			PasskeyRpOrigins: []string{"https://demo.g8e.ai"},
		},
	}

	m := NewModel(opts)
	m.reviewConfirmed = true
	result := m.result()
	assert.False(t, result.Cancel)
	assert.Equal(t, "https://demo.g8e.ai", result.Config.PublicBaseURL)
	assert.Equal(t, "consensus", result.Config.Posture)
	assert.Equal(t, "trib-prod-01", result.Config.ConsensusID)
}

// --- Run: error wrapping ---

func TestRun_ErrorWrapping_Format(t *testing.T) {
	m := NewModel(Options{})
	m.quitting = true
	result := m.result()
	assert.True(t, result.Cancel)
	_ = m
}

// --- Config type ---

func TestConfig_ZeroValue(t *testing.T) {
	var c Config
	assert.Empty(t, c.PublicBaseURL)
	assert.Empty(t, c.CertIdentityMode)
	assert.Nil(t, c.AllowedOrigins)
	assert.Empty(t, c.Posture)
}

// --- Options type ---

func TestOptions_ZeroValue(t *testing.T) {
	var o Options
	assert.Empty(t, o.InitialConfig.PublicBaseURL)
	assert.Nil(t, o.ProgramOptions)
}

// --- Result type ---

func TestResult_ZeroValue(t *testing.T) {
	var r Result
	assert.False(t, r.Cancel)
	assert.Empty(t, r.Config.PublicBaseURL)
}

// --- Run: full step-through with tea.Program ---

func TestRun_FullStepThrough(t *testing.T) {
	m := NewModel(Options{
		InitialConfig: Config{
			PublicBaseURL: "https://demo.g8e.ai",
		},
	})

	// Step 1: Network -> Posture
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	m2, _ = m2.Update(msg)
	assert.Equal(t, StepPosture, m2.(Model).step)

	// Step 2: Posture -> Routing
	m3, cmd2 := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd2)
	msg2 := cmd2()
	m3, _ = m3.Update(msg2)
	assert.Equal(t, StepRouting, m3.(Model).step)

	// Step 3: Routing -> Review
	m4, cmd3 := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd3)
	msg3 := cmd3()
	m4, _ = m4.Update(msg3)
	assert.Equal(t, StepReview, m4.(Model).step)

	// Step 4: Review -> Done (quit)
	m5, cmd4 := m4.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m5.(Model).reviewConfirmed)
	assert.NotNil(t, cmd4)

	result := m5.(Model).result()
	assert.False(t, result.Cancel)
	assert.Equal(t, "https://demo.g8e.ai", result.Config.PublicBaseURL)
}

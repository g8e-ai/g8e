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

package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// newTestProgram builds a headless bubbletea program (no renderer, no real
// input/output) suitable for driving Model updates from tests.
func newTestProgram(t *testing.T) *tea.Program {
	t.Helper()
	m := NewModel(Options{})
	return tea.NewProgram(m, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
}

// headlessProgramOptions returns program options that allow Run to execute
// without a real terminal. The provided input string is fed to the program;
// "q" triggers a clean quit via the key handler.
func headlessProgramOptions(input string) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(io.Discard),
	}
}

func TestRun_EmptySSEURLReturnsQuickly(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after simulated quit keypress")
	}
}

func TestRun_QuitViaCtrlC(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			ProgramOptions: headlessProgramOptions("ctrl+c"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after ctrl+c")
	}
}

func TestRun_PassesOptionsToModel(t *testing.T) {
	opts := Options{
		Version:        "v1.3.6",
		NodeName:       "node-alpha",
		NetLabel:       "mTLS",
		Quorum:         4,
		Total:          7,
		ProgramOptions: headlessProgramOptions("q"),
	}

	done := make(chan struct{}, 1)
	var capturedModel Model
	go func() {
		m := NewModel(opts)
		capturedModel = m
		done <- struct{}{}
	}()
	<-done

	assert.Equal(t, "v1.3.6", capturedModel.version)
	assert.Equal(t, "node-alpha", capturedModel.nodeName)
	assert.Equal(t, "mTLS", capturedModel.netLabel)
	assert.Equal(t, 4, capturedModel.quorum)
	assert.Equal(t, 7, capturedModel.total)
}

func TestRun_AdapterGoroutineCleanedUpOnExit(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			SSEURL:         "http://127.0.0.1:1/nonexistent",
			HTTPClient:     nil,
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return; adapter goroutine may not be cleaned up")
	}
}

func TestRun_CancelledContextExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

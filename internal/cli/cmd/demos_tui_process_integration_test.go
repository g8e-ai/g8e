//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const demoTUIChildProcessEnv = "G8E_DEMO_TUI_CHILD_PROCESS"

func TestRunDemosWithTUILifecycle_EarlyQuitTerminatesAndWaitsForChildProcess(t *testing.T) {
	childStarted := make(chan error, 1)
	childExited := make(chan struct{})
	var child *exec.Cmd
	program := &stubDemoProgram{
		run: func() (tea.Model, error) {
			require.NoError(t, <-childStarted)
			return tui.NewModel(tui.Options{}), nil
		},
		sent: make(chan tea.Msg, 1),
	}

	err := runDemosWithTUILifecycle(t.Context(), program, func(ctx context.Context) error {
		child = exec.CommandContext(ctx, os.Args[0], "-test.run=TestDemoTUIChildProcess_WaitsForCancellation")
		child.Env = append(os.Environ(), demoTUIChildProcessEnv+"=1")
		err := child.Start()
		childStarted <- err
		if err != nil {
			return err
		}
		err = child.Wait()
		close(childExited)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	})

	assert.ErrorIs(t, err, constants.ErrDemoScenarioCancelled)
	assert.ErrorIs(t, err, context.Canceled)
	assertClosed(t, childExited)
	require.NotNil(t, child)
	require.NotNil(t, child.ProcessState)
	assert.False(t, child.ProcessState.Success())
}

func TestDemoTUIChildProcess_WaitsForCancellation(t *testing.T) {
	if os.Getenv(demoTUIChildProcessEnv) != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestRunDemosWithTUILifecycle_CompletionCheckpointRemainsVisibleUntilQuit(t *testing.T) {
	checkpoint := make(chan string, 1)
	tuiModel, _ := tui.NewModel(tui.Options{}).Update(tea.WindowSizeMsg{Width: 144, Height: 45})
	model := &checkpointObservingModel{
		model:      tuiModel.(tui.Model),
		checkpoint: checkpoint,
	}
	program := tea.NewProgram(
		model,
		tea.WithContext(t.Context()),
		tea.WithoutRenderer(),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
	)
	result := make(chan error, 1)
	go func() {
		result <- runDemosWithTUILifecycle(t.Context(), program, func(context.Context) error {
			return nil
		})
	}()

	select {
	case view := <-checkpoint:
		assert.Contains(t, view, "PRESENTATION CHECKPOINT: scenario-succeeded")
	case <-time.After(5 * time.Second):
		t.Fatal("completion checkpoint was not rendered")
	}

	select {
	case err := <-result:
		t.Fatalf("TUI exited before operator quit: %v", err)
	default:
	}

	program.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	select {
	case err := <-result:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("TUI did not exit after operator quit")
	}
}

type checkpointObservingModel struct {
	model      tui.Model
	checkpoint chan<- string
}

func (m *checkpointObservingModel) Init() tea.Cmd {
	return m.model.Init()
}

func (m *checkpointObservingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.model.Update(msg)
	m.model = model.(tui.Model)
	if _, ok := msg.(tui.ScenarioCompleteMsg); ok {
		m.checkpoint <- m.model.View()
	}
	return m, cmd
}

func (m *checkpointObservingModel) View() string {
	return m.model.View()
}

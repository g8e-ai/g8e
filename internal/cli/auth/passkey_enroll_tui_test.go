// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewEnrollModel(t *testing.T) {
	m := newEnrollModel()
	assert.False(t, m.done)
	assert.Nil(t, m.err)
}

func TestEnrollModelInit(t *testing.T) {
	m := newEnrollModel()
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestEnrollModelUpdate(t *testing.T) {
	t.Run("tick increments counter and returns tick command", func(t *testing.T) {
		m := newEnrollModel()
		next, cmd := m.Update(enrollTickMsg{})
		assert.Equal(t, 1, next.(enrollModel).tick)
		assert.NotNil(t, cmd)
	})

	t.Run("passkeyRegisteredMsg sets done and quits", func(t *testing.T) {
		m := newEnrollModel()
		next, cmd := m.Update(passkeyRegisteredMsg{})
		assert.True(t, next.(enrollModel).done)
		assert.NotNil(t, cmd)
		_, ok := cmd().(tea.QuitMsg)
		assert.True(t, ok, "cmd should return tea.QuitMsg")
	})

	t.Run("enrollErrMsg sets err and quits", func(t *testing.T) {
		testErr := fmt.Errorf("timeout")
		m := newEnrollModel()
		next, cmd := m.Update(enrollErrMsg{err: testErr})
		assert.Equal(t, testErr, next.(enrollModel).err)
		assert.NotNil(t, cmd)
		_, ok := cmd().(tea.QuitMsg)
		assert.True(t, ok, "cmd should return tea.QuitMsg")
	})

	t.Run("q key quits", func(t *testing.T) {
		m := newEnrollModel()
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		assert.NotNil(t, cmd)
		_, ok := cmd().(tea.QuitMsg)
		assert.True(t, ok, "cmd should return tea.QuitMsg")
		_ = next
	})

	t.Run("ctrl+c quits", func(t *testing.T) {
		m := newEnrollModel()
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		assert.NotNil(t, cmd)
		_, ok := cmd().(tea.QuitMsg)
		assert.True(t, ok, "cmd should return tea.QuitMsg")
		_ = next
	})

	t.Run("other key is no-op", func(t *testing.T) {
		m := newEnrollModel()
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		assert.Nil(t, cmd)
		assert.False(t, next.(enrollModel).done)
	})
}

func TestEnrollModelView(t *testing.T) {
	t.Run("waiting view shows spinner and pointer to the printed URL", func(t *testing.T) {
		m := newEnrollModel()
		view := m.View()
		assert.Contains(t, view, "Waiting for passkey registration")
		assert.Contains(t, view, "Press q to cancel")
	})

	t.Run("waiting view never renders the token-bearing URL", func(t *testing.T) {
		// bubbletea truncates every view line wider than the terminal with no
		// ellipsis. A clicked or copied truncated URL carries a truncated
		// one-time token that the Gateway rejects as not found, so the secret
		// URL must only reach the user through the unabridged Out sink.
		view := newEnrollModel().View()
		assert.NotContains(t, view, "token=")
		assert.NotContains(t, view, "#enroll")
		assert.Contains(t, view, "enrollment URL printed above")
	})

	t.Run("done view shows success", func(t *testing.T) {
		m := newEnrollModel()
		m.done = true
		view := m.View()
		assert.Contains(t, view, "Passkey registered successfully")
	})

	t.Run("error view shows error message", func(t *testing.T) {
		m := newEnrollModel()
		m.err = fmt.Errorf("connection failed")
		view := m.View()
		assert.Contains(t, view, "Enrollment failed")
		assert.Contains(t, view, "connection failed")
	})
}

func TestSpinnerChar(t *testing.T) {
	chars := []string{"|", "/", "-", "\\"}
	for i := 0; i < 8; i++ {
		assert.Equal(t, chars[i%4], spinnerChar(i))
	}
}

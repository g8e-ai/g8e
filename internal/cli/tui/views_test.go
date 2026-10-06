// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

var keyEsc = tea.KeyMsg{Type: tea.KeyEsc}

func enrollmentsFixture() EnrollmentsMsg {
	now := time.Now()
	return EnrollmentsMsg{
		Pending: []models.PlatformEnrollmentPendingRequest{
			{RequestID: "req-pending-1", ComponentKind: models.PlatformComponentOperator, Hostname: "web-01", InstanceID: "i-1", ExpiresAt: now.Add(time.Hour),
				Fingerprints: models.PlatformEnrollmentCSRFingerprints{Operator: "SHA256:operator-fp"}},
			{RequestID: "req-pending-2", ComponentKind: models.PlatformComponentEnsemble, Hostname: "web-02", InstanceID: "i-2", ExpiresAt: now.Add(time.Hour)},
		},
		Enrolled: []models.PlatformEnrollmentEnrolledRequest{
			{RequestID: "req-done-1", ComponentKind: models.PlatformComponentOperator, Hostname: "db-01", State: "completed", OperatorID: "op-1"},
			{RequestID: "req-done-2", ComponentKind: models.PlatformComponentOperator, Hostname: "db-02", State: "revoked", RevokedAt: &now},
		},
	}
}

func TestViewSwitching(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, viewOverview, m.view)

	for _, tt := range []struct {
		key  rune
		want viewID
	}{{'2', viewApprovals}, {'3', viewOperators}, {'6', viewEnrollments}, {'1', viewOverview}} {
		m, _ = press(t, m, runeKey(tt.key))
		assert.Equal(t, tt.want, m.view, "key %c", tt.key)
	}

	m, _ = press(t, m, runeKey('4'))
	assert.Equal(t, viewOverview, m.view, "a reserved key does not switch views")
}

func TestHelpOverlay(t *testing.T) {
	m := NewModel(Options{})
	m, _ = press(t, m, runeKey('?'))
	require.True(t, m.showHelp)
	assert.Contains(t, m.View(), "KEYS")

	m, _ = press(t, m, runeKey('2'))
	assert.Equal(t, viewOverview, m.view, "keys other than ?/esc/q are ignored under help")

	m, _ = press(t, m, keyEsc)
	assert.False(t, m.showHelp)
}

func TestApprovalsViewKeys(t *testing.T) {
	m := NewModel(Options{
		ApprovalURL: func(tx string) string { return "https://gw/approve/" + tx },
		OpenBrowser: func(string) error { return nil },
	}).applyPendingApprovalsMsg(pendingTxs("tx-1", "tx-2"))
	m, _ = press(t, m, runeKey('2'))

	m, _ = press(t, m, runeKey('j'))
	assert.Equal(t, 1, m.pendingSelected, "j moves without focusing a pane first")

	_, cmd := press(t, m, keyEnter)
	require.NotNil(t, cmd)
	assert.Equal(t, ApprovalOpenedMsg{TxHash: "tx-2", URL: "https://gw/approve/tx-2"}, cmd())

	out := m.View()
	assert.Contains(t, out, "PENDING L3 APPROVALS (2)")
	assert.Contains(t, out, "TRANSACTION")
}

func TestOperatorsViewKeys(t *testing.T) {
	m := NewModel(Options{Identity: Identity{OperatorID: "op-2"}}).applyOperatorsMsg(OperatorsMsg{Operators: []models.OperatorDocumentGo{
		{ID: "op-1", CurrentHostname: "alpha", Status: constants.OperatorStatusActive},
		{ID: "op-2", CurrentHostname: "bravo", Status: constants.OperatorStatusActive},
	}})
	m, _ = press(t, m, runeKey('3'))
	m, _ = press(t, m, runeKey('j'))
	assert.Equal(t, 1, m.operatorsSelected)

	out := m.View()
	assert.Contains(t, out, "bravo")
	assert.Contains(t, out, "yes (this CLI session)")
}

func TestEnrollmentsView(t *testing.T) {
	t.Run("new pending requests are announced after the first listing", func(t *testing.T) {
		m := NewModel(Options{}).applyEnrollmentsMsg(EnrollmentsMsg{})
		before := len(m.ledger)
		m = m.applyEnrollmentsMsg(enrollmentsFixture())
		require.Len(t, m.ledger, before+2)
		assert.Contains(t, m.ledger[before].message, "ENROLLMENT REQUEST: operator from web-01")
	})

	t.Run("the first listing is not announced", func(t *testing.T) {
		m := NewModel(Options{}).applyEnrollmentsMsg(enrollmentsFixture())
		assert.Empty(t, m.ledger)
		assert.Equal(t, " (2)", m.viewCount(viewEnrollments))
	})

	t.Run("selection follows the selected request across refreshes", func(t *testing.T) {
		m := NewModel(Options{}).applyEnrollmentsMsg(enrollmentsFixture())
		m.enrollPendingSelected = 1
		fixture := enrollmentsFixture()
		fixture.Pending = fixture.Pending[1:]
		m = m.applyEnrollmentsMsg(fixture)
		assert.Equal(t, 0, m.enrollPendingSelected)
		assert.Equal(t, "req-pending-2", m.enrollPending[0].RequestID)
	})

	t.Run("a listing error is shown in place of the lists", func(t *testing.T) {
		m := NewModel(Options{}).applyEnrollmentsMsg(EnrollmentsMsg{Err: &api.StatusError{StatusCode: http.StatusForbidden, Body: "owner only"}})
		m, _ = press(t, m, runeKey('6'))
		assert.Contains(t, m.View(), "unavailable:")
		assert.Equal(t, ConnIdle, m.connStatus, "a 403 is not a session expiry")
	})

	t.Run("detail shows the selected request's fingerprints", func(t *testing.T) {
		m := NewModel(Options{}).applyEnrollmentsMsg(enrollmentsFixture())
		m, _ = press(t, m, runeKey('6'))
		out := m.View()
		assert.Contains(t, out, "ENROLLMENT REQUEST")
		assert.Contains(t, out, "SHA256:operator-fp")
	})
}

func TestEnrollmentDecision(t *testing.T) {
	setup := func(t *testing.T) (Model, *testSession) {
		t.Helper()
		session := &testSession{}
		m := NewModel(Options{Session: session}).applyEnrollmentsMsg(enrollmentsFixture())
		m, _ = press(t, m, runeKey('6'))
		return m, session
	}

	t.Run("approve asks first and posts only on y", func(t *testing.T) {
		m, session := setup(t)
		m, cmd := press(t, m, runeKey('a'))
		assert.Nil(t, cmd)
		require.NotNil(t, m.confirm)
		assert.Contains(t, m.confirm.prompt, "Approve the operator enrollment request from web-01")
		assert.Contains(t, m.View(), "CONFIRM")
		assert.Empty(t, session.posted, "nothing is posted before confirming")

		m, cmd = press(t, m, runeKey('y'))
		assert.Nil(t, m.confirm)
		require.NotNil(t, cmd)
		msg := cmd()
		decided, ok := msg.(EnrollmentDecidedMsg)
		require.True(t, ok)
		require.NoError(t, decided.Err)
		require.Len(t, session.posted, 1)
		assert.Equal(t, models.PlatformEnrollmentDecisionRequest{RequestID: "req-pending-1", Decision: models.PlatformEnrollmentDecisionApprove}, session.posted[0])

		model, cmd := m.Update(msg)
		m = model.(Model)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "approved")
		require.NotNil(t, cmd, "a decision re-lists enrollments")
		_, ok = cmd().(EnrollmentsMsg)
		assert.True(t, ok)
	})

	t.Run("deny targets the selected request", func(t *testing.T) {
		m, session := setup(t)
		m, _ = press(t, m, runeKey('j'))
		m, _ = press(t, m, runeKey('d'))
		require.NotNil(t, m.confirm)
		assert.Contains(t, m.confirm.prompt, "Deny the ensemble enrollment request from web-02")
		_, cmd := press(t, m, runeKey('y'))
		cmd()
		require.Len(t, session.posted, 1)
		assert.Equal(t, models.PlatformEnrollmentDecisionDeny, session.posted[0].(models.PlatformEnrollmentDecisionRequest).Decision)
	})

	t.Run("n and esc cancel without posting", func(t *testing.T) {
		for _, key := range []tea.KeyMsg{runeKey('n'), keyEsc} {
			m, session := setup(t)
			m, _ = press(t, m, runeKey('a'))
			m, cmd := press(t, m, key)
			assert.Nil(t, cmd)
			assert.Nil(t, m.confirm)
			assert.Empty(t, session.posted)
			assert.Contains(t, m.ledger[len(m.ledger)-1].message, "Cancelled")
		}
	})

	t.Run("other keys are swallowed while confirming", func(t *testing.T) {
		m, _ := setup(t)
		m, _ = press(t, m, runeKey('a'))
		m, cmd := press(t, m, runeKey('q'))
		assert.Nil(t, cmd, "q does not quit past an open confirmation")
		assert.NotNil(t, m.confirm)
		assert.False(t, m.quitting)
	})

	t.Run("a failed decision is reported and re-lists", func(t *testing.T) {
		m, session := setup(t)
		session.postErr = &api.StatusError{StatusCode: http.StatusUnauthorized}
		m, _ = press(t, m, runeKey('a'))
		_, cmd := press(t, m, runeKey('y'))
		model, cmd := m.Update(cmd())
		m = model.(Model)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "failed")
		assert.Equal(t, ConnFailed, m.connStatus, "a 401 marks the CLI session expired")
		assert.NotNil(t, cmd)
	})

	t.Run("approve needs a selected pending request", func(t *testing.T) {
		m, _ := setup(t)
		m, _ = press(t, m, keyTab) // enrolled section
		m, _ = press(t, m, runeKey('a'))
		assert.Nil(t, m.confirm)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "Select a pending enrollment request")
	})
}

func TestEnrollmentRevoke(t *testing.T) {
	session := &testSession{}
	m := NewModel(Options{Session: session}).applyEnrollmentsMsg(enrollmentsFixture())
	m, _ = press(t, m, runeKey('6'))
	m, _ = press(t, m, keyTab)
	require.Equal(t, enrollSectionEnrolled, m.enrollSection)

	assert.Contains(t, m.View(), "operator db-01 op-1 completed", "enrolled rows name the issued identity")
	m, _ = press(t, m, runeKey('x'))
	require.NotNil(t, m.confirm)
	assert.Contains(t, m.confirm.prompt, "Revoke the operator enrollment of db-01")
	m.width = 40
	assert.Contains(t, m.View(), "immediately.", "the prompt wraps instead of being cut")
	m, cmd := press(t, m, runeKey('y'))
	revoked, ok := cmd().(EnrollmentRevokedMsg)
	require.True(t, ok)
	require.NoError(t, revoked.Err)
	assert.Equal(t, models.PlatformEnrollmentRevokeRequest{RequestID: "req-done-1"}, session.posted[0])

	t.Run("an already revoked enrollment is not offered", func(t *testing.T) {
		m, _ := press(t, m, runeKey('j'))
		m, _ = press(t, m, runeKey('x'))
		assert.Nil(t, m.confirm)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "already revoked")
	})
}

func TestOperatorStop(t *testing.T) {
	setup := func(t *testing.T) (Model, *testSession) {
		t.Helper()
		session := &testSession{}
		m := NewModel(Options{Session: session}).applyOperatorsMsg(OperatorsMsg{Operators: []models.OperatorDocumentGo{
			{ID: "op-embedded", CurrentHostname: "gateway", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorSessionID: "sess-embedded"},
			{ID: "op-remote", CurrentHostname: "web-01", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, OperatorSessionID: "sess-remote"},
		}})
		m, _ = press(t, m, runeKey('3'))
		return m, session
	}

	t.Run("s asks first and posts the shutdown only on y", func(t *testing.T) {
		m, session := setup(t)
		m, _ = press(t, m, runeKey('j'))
		m, cmd := press(t, m, runeKey('s'))
		assert.Nil(t, cmd)
		require.NotNil(t, m.confirm)
		assert.Contains(t, m.confirm.prompt, "Stop Operator web-01")
		assert.Empty(t, session.posted, "nothing is posted before confirming")

		m, cmd = press(t, m, runeKey('y'))
		require.NotNil(t, cmd)
		msg := cmd()
		stopped, ok := msg.(OperatorStopMsg)
		require.True(t, ok)
		require.NoError(t, stopped.Err)
		require.Len(t, session.posted, 1)
		assert.Equal(t, models.StopOperatorRequest{OperatorSessionID: "sess-remote", Reason: stopReason}, session.posted[0])

		model, cmd := m.Update(msg)
		m = model.(Model)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "Stop requested")
		require.NotNil(t, cmd, "a stop re-lists Operators")
		_, ok = cmd().(OperatorsMsg)
		assert.True(t, ok)
	})

	t.Run("the embedded Operator is refused without asking", func(t *testing.T) {
		m, session := setup(t)
		m, cmd := press(t, m, runeKey('s'))
		assert.Nil(t, cmd)
		assert.Nil(t, m.confirm)
		assert.Empty(t, session.posted)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, constants.ErrOperatorStopEmbedded.Error())
	})

	t.Run("n cancels without posting", func(t *testing.T) {
		m, session := setup(t)
		m, _ = press(t, m, runeKey('j'))
		m, _ = press(t, m, runeKey('s'))
		m, _ = press(t, m, runeKey('n'))
		assert.Nil(t, m.confirm)
		assert.Empty(t, session.posted)
	})

	t.Run("nothing selected", func(t *testing.T) {
		m := NewModel(Options{Session: &testSession{}})
		m, _ = press(t, m, runeKey('3'))
		m, _ = press(t, m, runeKey('s'))
		assert.Nil(t, m.confirm)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "Select an Operator")
	})

	t.Run("a failed stop is reported and a 401 marks the session expired", func(t *testing.T) {
		m, session := setup(t)
		session.postErr = &api.StatusError{StatusCode: http.StatusUnauthorized}
		m, _ = press(t, m, runeKey('j'))
		m, _ = press(t, m, runeKey('s'))
		_, cmd := press(t, m, runeKey('y'))
		model, _ := m.Update(cmd())
		m = model.(Model)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "failed")
		assert.Equal(t, ConnFailed, m.connStatus)
	})

	t.Run("an unsuccessful response is an error", func(t *testing.T) {
		m, session := setup(t)
		session.stopJSON = `{"success":false}`
		m, _ = press(t, m, runeKey('j'))
		m, _ = press(t, m, runeKey('s'))
		_, cmd := press(t, m, runeKey('y'))
		stopped := cmd().(OperatorStopMsg)
		assert.Error(t, stopped.Err)
	})
}

// TestViews_FitTerminalExactly checks every view and overlay fills the
// terminal exactly, as TestView_FitsTerminalExactly does for the overview.
func TestViews_FitTerminalExactly(t *testing.T) {
	base := NewModel(Options{Session: &testSession{}, Version: "v2.3.1", Identity: Identity{UserID: strings.Repeat("u", 36), CLISessionID: "session", OperatorID: "op-1"}}).
		applyPendingApprovalsMsg(pendingTxs(strings.Repeat("a", 64), "tx-2")).
		applyOperatorsMsg(OperatorsMsg{Operators: []models.OperatorDocumentGo{{ID: "op-1", CurrentHostname: strings.Repeat("h", 80), Status: constants.OperatorStatusActive, LocalDir: strings.Repeat("/d", 60)}}}).
		applyEnrollmentsMsg(enrollmentsFixture())

	confirming := base
	confirming.view = viewEnrollments
	confirming = confirming.confirmEnrollmentDecision(models.PlatformEnrollmentDecisionApprove)
	require.NotNil(t, confirming.confirm)
	help := base
	help.showHelp = true

	cases := map[string]Model{"confirm": confirming, "help": help}
	for _, spec := range viewSpecs {
		m := base
		m.view = spec.id
		cases[spec.title] = m
	}
	for name, m := range cases {
		for _, size := range []struct{ width, height int }{{160, 40}, {80, 24}} {
			m.width, m.height = size.width, size.height
			lines := strings.Split(m.View(), "\n")
			assert.Len(t, lines, size.height, "%s %dx%d", name, size.width, size.height)
			for i, line := range lines {
				assert.LessOrEqual(t, lipgloss.Width(line), size.width, "%s %dx%d line %d", name, size.width, size.height, i)
			}
		}
	}
}

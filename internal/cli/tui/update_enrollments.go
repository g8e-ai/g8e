// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// enrollSection is the selectable section of the enrollments view.
type enrollSection int

const (
	enrollSectionPending enrollSection = iota
	enrollSectionEnrolled
)

// handleEnrollmentsKey applies a key on the enrollments view: tab switches
// section, a/d decide the selected pending request, and x revokes the
// selected enrollment. Every decision asks y/N first, as 'g8e auth enroll'
// does without --yes.
func (m Model) handleEnrollmentsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab", "shift+tab":
		if m.enrollSection == enrollSectionPending {
			m.enrollSection = enrollSectionEnrolled
		} else {
			m.enrollSection = enrollSectionPending
		}
	case "up", "k":
		m = m.moveEnrollSelection(-1)
	case "down", "j":
		m = m.moveEnrollSelection(1)
	case "a":
		return m.confirmEnrollmentDecision(models.PlatformEnrollmentDecisionApprove), nil
	case "d":
		return m.confirmEnrollmentDecision(models.PlatformEnrollmentDecisionDeny), nil
	case "x":
		return m.confirmEnrollmentRevoke(), nil
	}
	return m, nil
}

func (m Model) moveEnrollSelection(delta int) Model {
	if m.enrollSection == enrollSectionPending {
		m.enrollPendingSelected = clamp(m.enrollPendingSelected+delta, max(len(m.enrollPending)-1, 0))
	} else {
		m.enrollEnrolledSelected = clamp(m.enrollEnrolledSelected+delta, max(len(m.enrollEnrolled)-1, 0))
	}
	return m
}

// selectedPendingEnrollment returns the selected pending request when the
// pending section is active.
func (m Model) selectedPendingEnrollment() (models.PlatformEnrollmentPendingRequest, bool) {
	if m.enrollSection != enrollSectionPending || m.enrollPendingSelected >= len(m.enrollPending) {
		return models.PlatformEnrollmentPendingRequest{}, false
	}
	return m.enrollPending[m.enrollPendingSelected], true
}

// selectedEnrolled returns the selected enrollment when the enrolled section
// is active.
func (m Model) selectedEnrolled() (models.PlatformEnrollmentEnrolledRequest, bool) {
	if m.enrollSection != enrollSectionEnrolled || m.enrollEnrolledSelected >= len(m.enrollEnrolled) {
		return models.PlatformEnrollmentEnrolledRequest{}, false
	}
	return m.enrollEnrolled[m.enrollEnrolledSelected], true
}

// confirmEnrollmentDecision asks to approve or deny the selected pending
// request.
func (m Model) confirmEnrollmentDecision(decision models.PlatformEnrollmentDecision) Model {
	req, ok := m.selectedPendingEnrollment()
	if !ok {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Select a pending enrollment request first (tab switches section)"})
	}
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	verb := "Approve"
	if decision == models.PlatformEnrollmentDecisionDeny {
		verb = "Deny"
	}
	gw := m.gw
	decisionReq := models.PlatformEnrollmentDecisionRequest{RequestID: req.RequestID, Decision: decision}
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("%s the %s enrollment request from %s (instance %s, request %s)? Compare the key fingerprints with the workload's output first.",
			verb, req.ComponentKind, req.Hostname, req.InstanceID, shortHash(req.RequestID)),
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.decideEnrollment(ctx, decisionReq)
		},
	}
	return m
}

// confirmEnrollmentRevoke asks to revoke the selected enrollment.
func (m Model) confirmEnrollmentRevoke() Model {
	enrollment, ok := m.selectedEnrolled()
	if !ok {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Select an enrollment first (tab switches section)"})
	}
	if enrollment.RevokedAt != nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Enrollment %s is already revoked", shortHash(enrollment.RequestID))})
	}
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	gw := m.gw
	revokeReq := models.PlatformEnrollmentRevokeRequest{RequestID: enrollment.RequestID}
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("Revoke the %s enrollment of %s (request %s)? Its certificates, policy, and sessions are disabled immediately.",
			enrollment.ComponentKind, enrollment.Hostname, shortHash(enrollment.RequestID)),
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.revokeEnrollment(ctx, revokeReq)
		},
	}
	return m
}

// applyEnrollmentsMsg records the listed enrollments, keeping each section's
// selection on the same request when it is still listed.
func (m Model) applyEnrollmentsMsg(msg EnrollmentsMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.enrollmentsErr = msg.Err.Error()
		return m
	}
	pendingID := ""
	if m.enrollPendingSelected < len(m.enrollPending) {
		pendingID = m.enrollPending[m.enrollPendingSelected].RequestID
	}
	enrolledID := ""
	if m.enrollEnrolledSelected < len(m.enrollEnrolled) {
		enrolledID = m.enrollEnrolled[m.enrollEnrolledSelected].RequestID
	}

	known := make(map[string]struct{}, len(m.enrollPending))
	for _, req := range m.enrollPending {
		known[req.RequestID] = struct{}{}
	}
	m.enrollPendingSelected = 0
	for i, req := range msg.Pending {
		if req.RequestID == pendingID {
			m.enrollPendingSelected = i
		}
		if _, seen := known[req.RequestID]; !seen && m.enrollmentsLoaded {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("ENROLLMENT REQUEST: %s from %s (request %s) — press 6 to review", req.ComponentKind, req.Hostname, shortHash(req.RequestID))})
		}
	}
	m.enrollEnrolledSelected = 0
	for i, enrollment := range msg.Enrolled {
		if enrollment.RequestID == enrolledID {
			m.enrollEnrolledSelected = i
		}
	}

	m.enrollPending = msg.Pending
	m.enrollEnrolled = msg.Enrolled
	m.enrollmentsLoaded = true
	m.enrollmentsErr = ""
	return m
}

// applyEnrollmentDecidedMsg reports a decision and re-lists enrollments.
func (m Model) applyEnrollmentDecidedMsg(msg EnrollmentDecidedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Platform enrollment request %s %s failed: %s", shortHash(msg.RequestID), msg.Decision, msg.Err)})
	} else {
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Platform enrollment request %s %s.", shortHash(msg.RequestID), msg.Response.State)})
	}
	return m, m.fetchEnrollmentsCmd()
}

// applyEnrollmentRevokedMsg reports a revocation and re-lists enrollments.
func (m Model) applyEnrollmentRevokedMsg(msg EnrollmentRevokedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Revoking platform enrollment %s failed: %s", shortHash(msg.RequestID), msg.Err)})
	} else {
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Platform enrollment %s revoked (%s).", shortHash(msg.Response.RequestID), msg.Response.ComponentKind)})
	}
	return m, m.fetchEnrollmentsCmd()
}

func (m Model) fetchEnrollmentsCmd() tea.Cmd {
	if m.gw == nil {
		return nil
	}
	return withTimeout(m.gw.fetchEnrollments)
}

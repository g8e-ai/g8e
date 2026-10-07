// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
)

// renderEnrollmentsView renders the pending platform enrollment requests over
// the completed enrollments, beside the selected item's details.
func (m Model) renderEnrollmentsView(width, height int) string {
	leftWidth, rightWidth := splitWidth(width)
	pendingHeight := height / 2
	enrolledHeight := height - pendingHeight

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left,
			m.renderPendingEnrollments(leftWidth, pendingHeight),
			m.renderEnrolled(leftWidth, enrolledHeight),
		),
		box(borderPane, rightWidth, height, m.enrollmentDetail(rightWidth)),
	)
}

// enrollSectionBorder highlights the active section.
func (m Model) enrollSectionBorder(section enrollSection) lipgloss.Style {
	if m.enrollSection == section {
		return borderFocused
	}
	return borderPane
}

// enrollmentStatusLine is the placeholder line shown before a list loads or
// when it cannot be listed (only the platform owner may list enrollments).
func (m Model) enrollmentStatusLine() (string, bool) {
	switch {
	case m.enrollmentsErr != "":
		return ledgerWarnStyle.Render("unavailable: " + m.enrollmentsErr), true
	case !m.enrollmentsLoaded:
		return detailStyle.Render("(loading...)"), true
	}
	return "", false
}

func (m Model) renderPendingEnrollments(width, height int) string {
	lines := []string{paneHeaderStyle.Render(fmt.Sprintf("PENDING ENROLLMENT REQUESTS (%d)", len(m.enrollPending)))}
	if status, ok := m.enrollmentStatusLine(); ok {
		lines = append(lines, status)
	} else if len(m.enrollPending) == 0 {
		lines = append(lines, detailStyle.Render("(no pending requests)"))
	}
	rows := height - borderPane.GetVerticalFrameSize() - 1
	start, end := windowFor(len(m.enrollPending), m.enrollPendingSelected, rows)
	active := m.enrollSection == enrollSectionPending
	for i := start; i < end; i++ {
		req := m.enrollPending[i]
		row := fmt.Sprintf("%s %s expires %s", req.ComponentKind, req.Hostname, formatRemaining(req.ExpiresAt.Sub(timeNow())))
		lines = append(lines, selectableRow(active && i == m.enrollPendingSelected, row))
	}
	return box(m.enrollSectionBorder(enrollSectionPending), width, height, lines)
}

func (m Model) renderEnrolled(width, height int) string {
	lines := []string{paneHeaderStyle.Render(fmt.Sprintf("ENROLLMENTS (%d)", len(m.enrollEnrolled)))}
	if status, ok := m.enrollmentStatusLine(); ok {
		lines = append(lines, status)
	} else if len(m.enrollEnrolled) == 0 {
		lines = append(lines, detailStyle.Render("(no completed enrollments)"))
	}
	rows := height - borderPane.GetVerticalFrameSize() - 1
	start, end := windowFor(len(m.enrollEnrolled), m.enrollEnrolledSelected, rows)
	active := m.enrollSection == enrollSectionEnrolled
	for i := start; i < end; i++ {
		e := m.enrollEnrolled[i]
		row := fmt.Sprintf("%s %s", e.ComponentKind, e.Hostname)
		if identity := auth.PlatformEnrollmentIssuedIdentity(e); identity != "" {
			row += " " + shortHash(identity)
		}
		row += " " + string(e.State)
		lines = append(lines, selectableRow(active && i == m.enrollEnrolledSelected, row))
	}
	return box(m.enrollSectionBorder(enrollSectionEnrolled), width, height, lines)
}

// enrollmentDetail shows the selected request's or enrollment's details,
// the same fields 'g8e auth enroll pending' and 'g8e auth enroll list' print.
func (m Model) enrollmentDetail(width int) []string {
	if req, ok := m.selectedPendingEnrollment(); ok {
		lines := []string{paneHeaderStyle.Render("ENROLLMENT REQUEST"), "",
			detailRow("Request ID", req.RequestID),
			detailRow("Component", fmt.Sprintf("%s (%s)", req.ComponentKind, req.ComponentName)),
			detailRow("Instance ID", req.InstanceID),
			detailRow("Hostname", req.Hostname),
			detailRow("System FP", shortHash(req.SystemFingerprint)),
			detailRow("State", string(req.State)),
			detailRow("Created", req.CreatedAt.Format(detailTimeFormat)),
			detailRow("Expires", req.ExpiresAt.Format(detailTimeFormat)),
		}
		if fingerprints := auth.PlatformEnrollmentFingerprints(req.Fingerprints); len(fingerprints) > 0 {
			lines = append(lines, "", detailStyle.Render("Key fingerprints (compare with the workload output):"))
			for _, fp := range fingerprints {
				lines = append(lines, detailStyle.Render(fp.Label+":"))
				lines = append(lines, wrapText(fp.Value, innerWidth(width))...)
			}
		}
		return append(lines, "", detailStyle.Render("a: approve | d: deny"))
	}
	if e, ok := m.selectedEnrolled(); ok {
		lines := []string{paneHeaderStyle.Render("ENROLLMENT"), "",
			detailRow("Request ID", e.RequestID),
			detailRow("Component", fmt.Sprintf("%s (%s)", e.ComponentKind, e.ComponentName)),
			detailRow("Instance ID", e.InstanceID),
			detailRow("Hostname", e.Hostname),
			detailRow("State", string(e.State)),
			detailRow("Identity", auth.PlatformEnrollmentIssuedIdentity(e)),
			detailRow("Completed", formatOptionalTime(e.CompletedAt)),
		}
		if e.RevokedAt != nil {
			return append(lines, detailRow("Revoked", formatOptionalTime(e.RevokedAt)))
		}
		return append(lines, "", detailStyle.Render("x: revoke"))
	}
	return []string{paneHeaderStyle.Render("DETAILS"), "", detailStyle.Render("(nothing selected — tab switches section)")}
}

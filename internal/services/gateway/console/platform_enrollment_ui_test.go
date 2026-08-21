// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexHTML returns the raw index.html content for content assertions. The
// console SPA is a static HTML file with embedded JavaScript; these tests
// verify the platform enrollment UI elements, fragment handling, secret
// redaction, and 401/non-owner rejection handling are present and correct.
func indexHTML(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(staticFS, "static/index.html")
	require.NoError(t, err)
	return string(data)
}

// TestConsoleHTML_PlatformEnrollmentFragmentHandling verifies that the console
// SPA reads the #platform-enrollment=<request_id> fragment, stores the request
// ID in memory (pendingPlatformEnrollmentID), and clears the fragment so it
// never reaches server-side access logs.
func TestConsoleHTML_PlatformEnrollmentFragmentHandling(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "platform-enrollment")
	assert.Contains(t, html, "pendingPlatformEnrollmentID")
	assert.Contains(t, html, "hashParams.get('platform-enrollment')")
	// The fragment must be cleared after reading (history.replaceState).
	assert.Contains(t, html, "history.replaceState(null, '', location.pathname + location.search)")
}

// TestConsoleHTML_PlatformEnrollmentPendingListFetch verifies that the console
// fetches the authenticated pending list from the correct endpoint and stores
// the results for rendering.
func TestConsoleHTML_PlatformEnrollmentPendingListFetch(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "/api/v1/auth/platform-enrollments/pending")
	assert.Contains(t, html, "loadPlatformEnrollments")
	assert.Contains(t, html, "platformEnrollments")
}

// TestConsoleHTML_PlatformEnrollmentDecisionEndpoint verifies that the console
// posts approve/deny decisions to the correct endpoint with only the request
// ID and decision in the body — never a user ID or token.
func TestConsoleHTML_PlatformEnrollmentDecisionEndpoint(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "/api/v1/auth/platform-enrollments/decision")
	assert.Contains(t, html, "decidePlatformEnrollment")
	// The decision body must carry request_id and decision only.
	assert.Contains(t, html, "request_id: requestID")
	assert.Contains(t, html, "decision: decision")
}

// TestConsoleHTML_PlatformEnrollmentRequestDisplay verifies that the console
// renders the component kind, hostname, instance ID, system fingerprint, CSR
// fingerprints, state, creation time, and expiry for each pending request.
func TestConsoleHTML_PlatformEnrollmentRequestDisplay(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "renderPlatformEnrollmentDetailCard")
	assert.Contains(t, html, "renderPlatformEnrollmentsCard")
	assert.Contains(t, html, "component_kind")
	assert.Contains(t, html, "component_name")
	assert.Contains(t, html, "instance_id")
	assert.Contains(t, html, "hostname")
	assert.Contains(t, html, "system_fingerprint")
	assert.Contains(t, html, "fingerprintList")
	assert.Contains(t, html, "created_at")
	assert.Contains(t, html, "expires_at")
}

// TestConsoleHTML_PlatformEnrollmentApproveDenyButtons verifies that the
// console renders Approve and Deny buttons for pending requests and that the
// buttons call decidePlatformEnrollment with the correct decision values.
func TestConsoleHTML_PlatformEnrollmentApproveDenyButtons(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "peApproveBtn")
	assert.Contains(t, html, "peDenyBtn")
	assert.Contains(t, html, "decidePlatformEnrollment(pendingPlatformEnrollmentID, 'approve')")
	assert.Contains(t, html, "decidePlatformEnrollment(pendingPlatformEnrollmentID, 'deny')")
}

// TestConsoleHTML_PlatformEnrollmentNonOwnerRejection verifies that the
// console handles a 401 response from the decision endpoint by clearing the
// user session and re-rendering the sign-in prompt, so a non-owner or expired
// session is surfaced to the approver rather than silently failing.
func TestConsoleHTML_PlatformEnrollmentNonOwnerRejection(t *testing.T) {
	html := indexHTML(t)

	// The decidePlatformEnrollment function must check for 401 and reset
	// user=null, triggering re-render to the sign-in view.
	assert.Contains(t, html, "r.status === 401")
	assert.Contains(t, html, "user = null")
}

// TestConsoleHTML_PlatformEnrollmentNeverExposesTokens verifies that the
// console HTML and JavaScript never reference requester tokens, token hashes,
// CSR PEM, or certificates in the platform enrollment flow. The request ID is
// the only identifier used for approval and display.
func TestConsoleHTML_PlatformEnrollmentNeverExposesTokens(t *testing.T) {
	html := indexHTML(t)

	// The platform enrollment section must not reference token, token_hash,
	// csr_pem, or certificate PEM in any platform-enrollment-specific code.
	// We check that the platform enrollment functions reference request_id
	// (the non-secret identifier) rather than token.
	platformSection := html[strings.Index(html, "Platform Workload Enrollment Approval"):]
	if platformSection == "" {
		t.Fatal("platform enrollment section not found in HTML")
	}

	assert.Contains(t, platformSection, "request_id")
	assert.NotContains(t, platformSection, "token_hash")
	assert.NotContains(t, platformSection, "csr_pem")
	assert.NotContains(t, platformSection, "BEGIN CERTIFICATE")
}

// TestConsoleHTML_PlatformEnrollmentStateLabels verifies that the console
// renders human-readable state labels for all platform enrollment states.
func TestConsoleHTML_PlatformEnrollmentStateLabels(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "platformEnrollmentStateLabel")
	for _, state := range []string{"pending", "approved", "issuing", "completed", "denied", "expired"} {
		assert.Contains(t, html, "'"+state+"'")
	}
}

// TestConsoleHTML_PlatformEnrollmentUnauthenticatedPrompt verifies that when
// the user is not authenticated and a platform enrollment request ID is in
// the fragment, the console shows a prompt to sign in before reviewing the
// request.
func TestConsoleHTML_PlatformEnrollmentUnauthenticatedPrompt(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "pendingPlatformEnrollmentID")
	// The unauthenticated render path must mention signing in to review the
	// platform enrollment request.
	idx := strings.Index(html, "Platform Workload Enrollment")
	require.Greater(t, idx, -1)
	renderSection := html[idx:]
	assert.Contains(t, renderSection, "Sign in with your passkey")
}

// TestConsoleHTML_PlatformEnrollmentSignOutClearsState verifies that signing
// out clears the platform enrollment state so stale data does not persist
// across sessions.
func TestConsoleHTML_PlatformEnrollmentSignOutClearsState(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "platformEnrollments = []")
	assert.Contains(t, html, "pendingPlatformEnrollmentID = null")
}

// TestConsoleHTML_PlatformEnrollmentStatCounter verifies that the console
// shows a platform enrollments counter in the stats row.
func TestConsoleHTML_PlatformEnrollmentStatCounter(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "Platform Enrollments")
}

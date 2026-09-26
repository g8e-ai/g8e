// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// newRenderCmd creates a cobra.Command with a bytes.Buffer for output, used
// by render tests to capture printed text.
func newRenderCmd() (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	return cmd, &buf
}

// TestRenderConnectReview_PrintsOriginRPIDAndAPIURL verifies that the review
// output contains the normalized origin, derived RP ID, RP name, Gateway API
// URL, and Gateway state.
func TestRenderConnectReview_PrintsOriginRPIDAndAPIURL(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	cmd, buf := newRenderCmd()
	renderConnectReview(cmd, origin, "your-app.lovable.app", "g8e", "https://localhost:8443", gatewayStateStopped)

	output := buf.String()
	assert.Contains(t, output, "https://your-app.lovable.app")
	assert.Contains(t, output, "your-app.lovable.app")
	assert.Contains(t, output, "g8e")
	assert.Contains(t, output, "https://localhost:8443")
	assert.Contains(t, output, "stopped")
}

// TestRenderConnectReview_PrintsMatchingState verifies that the matching
// state label is printed when the Gateway is already correctly configured.
func TestRenderConnectReview_PrintsMatchingState(t *testing.T) {
	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	assert.NoError(t, err)

	cmd, buf := newRenderCmd()
	renderConnectReview(cmd, origin, "your-app.lovable.app", "g8e", "https://localhost:8443", gatewayStateMatching)

	assert.Contains(t, buf.String(), "running (configuration matches)")
}

// TestRenderConfigDelta_PrintsChangedFields verifies that config deltas are
// printed with the field name, current value, and proposed value.
func TestRenderConfigDelta_PrintsChangedFields(t *testing.T) {
	cmd, buf := newRenderCmd()
	deltas := []configDelta{
		{Field: "CORS origin (--cors-origin)", Current: "https://old.lovable.app", Proposed: "https://new.lovable.app"},
		{Field: "Passkey RP ID (--passkey-rp-id)", Current: "old.lovable.app", Proposed: "new.lovable.app"},
	}
	renderConfigDelta(cmd, deltas)

	output := buf.String()
	assert.Contains(t, output, "CORS origin (--cors-origin)")
	assert.Contains(t, output, "https://old.lovable.app")
	assert.Contains(t, output, "https://new.lovable.app")
	assert.Contains(t, output, "Passkey RP ID (--passkey-rp-id)")
	assert.Contains(t, output, "old.lovable.app")
	assert.Contains(t, output, "new.lovable.app")
}

// TestRenderConfigDelta_EmptyDeltasPrintsNothing verifies that an empty delta
// slice produces no output.
func TestRenderConfigDelta_EmptyDeltasPrintsNothing(t *testing.T) {
	cmd, buf := newRenderCmd()
	renderConfigDelta(cmd, nil)
	assert.Empty(t, buf.String())
}

// TestRenderConfigDelta_UnsetCurrentPrintsUnsetLabel verifies that an empty
// current value is displayed as "(unset)".
func TestRenderConfigDelta_UnsetCurrentPrintsUnsetLabel(t *testing.T) {
	cmd, buf := newRenderCmd()
	deltas := []configDelta{
		{Field: "CORS origin (--cors-origin)", Current: "", Proposed: "https://new.lovable.app"},
	}
	renderConfigDelta(cmd, deltas)
	assert.Contains(t, buf.String(), "(unset)")
}

// TestRenderVerificationReport_PrintsPassAndFailChecks verifies that the
// report output contains PASS/FAIL markers for each check with its detail.
func TestRenderVerificationReport_PrintsPassAndFailChecks(t *testing.T) {
	cmd, buf := newRenderCmd()
	report := frontendverify.Report{
		Checks: []frontendverify.CheckResult{
			{Name: frontendverify.CheckHTTPSHealth, Status: frontendverify.CheckPass, Detail: "Gateway healthy"},
			{Name: frontendverify.CheckCORSOrigin, Status: frontendverify.CheckFail, Detail: "origin mismatch"},
		},
		AllPassed:    false,
		FirstFailure: &frontendverify.CheckResult{Name: frontendverify.CheckCORSOrigin, Status: frontendverify.CheckFail, Detail: "origin mismatch"},
	}
	renderVerificationReport(cmd, report)

	output := buf.String()
	assert.Contains(t, output, "[PASS] https_health: Gateway healthy")
	assert.Contains(t, output, "[FAIL] cors_origin: origin mismatch")
}

// TestRenderFrontendPrompt_PrintsPromptAndNextAction verifies that the
// frontend prompt contains the Gateway API URL, the credentials instruction,
// and the next-action line about opening the app in a new tab.
func TestRenderFrontendPrompt_PrintsPromptAndNextAction(t *testing.T) {
	cmd, buf := newRenderCmd()
	renderFrontendPrompt(cmd, "https://localhost:8443")

	output := buf.String()
	assert.Contains(t, output, "https://localhost:8443")
	assert.Contains(t, output, "Include credentials")
	assert.Contains(t, output, "withCredentials: true")
	assert.Contains(t, output, "open the app in a new browser tab")
}

// TestRenderManualTrustInstructions_PrintsURLAndFingerprint verifies that
// manual trust instructions contain the Gateway HTTPS URL and root CA
// fingerprint, and never recommend disabling TLS.
func TestRenderManualTrustInstructions_PrintsURLAndFingerprint(t *testing.T) {
	cmd, buf := newRenderCmd()
	renderManualTrustInstructions(cmd, "https://localhost:8443", "abc123fingerprint")

	output := buf.String()
	assert.Contains(t, output, "https://localhost:8443")
	assert.Contains(t, output, "abc123fingerprint")
	assert.Contains(t, output, "Do not disable TLS verification")
}

// TestConnectAPIBaseURL_DefaultsToOperatorHttpsPort verifies that a zero
// httpsPort defaults to the configured Operator HTTPS port.
func TestConnectAPIBaseURL_DefaultsToOperatorHttpsPort(t *testing.T) {
	url := connectAPIBaseURL(0)
	assert.Contains(t, url, "https://localhost:")
	assert.Contains(t, url, "8443")
}

// TestConnectAPIBaseURL_UsesProvidedPort verifies that a non-zero port is
// used in the URL.
func TestConnectAPIBaseURL_UsesProvidedPort(t *testing.T) {
	url := connectAPIBaseURL(9000)
	assert.Contains(t, url, "9000")
}

// TestConnectHealthURL_AppendsHealthPath verifies that the health URL
// appends the health API path constant to the base URL.
func TestConnectHealthURL_AppendsHealthPath(t *testing.T) {
	url := connectHealthURL(8443)
	assert.Contains(t, url, "https://localhost:8443")
	assert.Contains(t, url, constants.APIPaths.Health)
}

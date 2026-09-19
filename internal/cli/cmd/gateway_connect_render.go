// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
)

// configDelta describes a single browser-relevant field that differs between
// the persisted launch profile and the proposed connect configuration. It is
// the output of browserConfigMatches and the input to renderConfigDelta.
type configDelta struct {
	Field    string
	Current  string
	Proposed string
}

// gatewayStateLabel is a string-typed summary of the Gateway process state
// discovered at the start of a connect run. It drives the review header.
type gatewayStateLabel string

const (
	gatewayStateStopped  gatewayStateLabel = "stopped"
	gatewayStateRunning  gatewayStateLabel = "running"
	gatewayStateMatching gatewayStateLabel = "running (configuration matches)"
)

// renderConnectReview prints the normalized origin, derived RP ID, Gateway API
// URL, and current Gateway state before any state-changing action. It is a
// pure write to the command's output sink and performs no I/O or prompts.
func renderConnectReview(cmd *cobra.Command, origin browserorigin.Origin, rpID, rpName, apiURL string, state gatewayStateLabel) {
	cmd.Println("Frontend connection review")
	cmd.Println()
	cmd.Printf("  Frontend origin:  %s\n", origin.URL)
	cmd.Printf("  Passkey RP ID:    %s\n", rpID)
	cmd.Printf("  Passkey RP name:  %s\n", rpName)
	cmd.Printf("  Gateway API:      %s\n", apiURL)
	cmd.Printf("  Gateway state:    %s\n", state)
	cmd.Println()
}

// renderConfigDelta prints only the browser-relevant fields that differ between
// the persisted launch profile and the proposed connect configuration. When
// deltas is empty, it prints nothing. It is a pure write to the command's
// output sink.
func renderConfigDelta(cmd *cobra.Command, deltas []configDelta) {
	if len(deltas) == 0 {
		return
	}
	cmd.Println("Configuration changes required:")
	cmd.Println()
	for _, d := range deltas {
		cmd.Printf("  %s\n", d.Field)
		if d.Current != "" {
			cmd.Printf("    current:  %s\n", d.Current)
		} else {
			cmd.Printf("    current:  (unset)\n")
		}
		cmd.Printf("    proposed: %s\n", d.Proposed)
	}
	cmd.Println()
}

// renderVerificationReport prints each check result in deterministic order with
// a pass/fail marker and the human-readable detail. It is a pure write to the
// command's output sink and performs no network I/O.
func renderVerificationReport(cmd *cobra.Command, report frontendverify.Report) {
	for _, check := range report.Checks {
		marker := "PASS"
		if check.Status == frontendverify.CheckFail {
			marker = "FAIL"
		}
		cmd.Printf("[%s] %s: %s\n", marker, check.Name, check.Detail)
	}
	cmd.Println()
}

// renderFrontendPrompt prints the concise prompt the user pastes into their
// frontend builder, followed by the next-action line. It is called only after
// system verification succeeds. It is a pure write to the command's output
// sink and contains no secrets.
func renderFrontendPrompt(cmd *cobra.Command, apiURL string) {
	prompt := fmt.Sprintf(
		"Connect this app to the g8e Gateway at %s. Make all Gateway requests directly from the browser, not from an edge function or server. Include credentials: \"include\" on every fetch request and withCredentials: true on every EventSource connection.",
		apiURL,
	)
	cmd.Println("Paste this into your frontend builder:")
	cmd.Println()
	cmd.Println(prompt)
	cmd.Println()
	cmd.Println("Next: open the app in a new browser tab and select Allow if the browser requests local-network access.")
}

// renderManualTrustInstructions prints the manual browser-trust path when the
// user declined OS trust installation or selected --no-system-trust. It
// provides the local health URL and fingerprint for manual validation and
// never recommends disabling TLS checks. It is a pure write to the command's
// output sink.
func renderManualTrustInstructions(cmd *cobra.Command, apiURL, fingerprint string) {
	cmd.Println("Manual browser trust is required.")
	cmd.Println()
	cmd.Printf("  Gateway HTTPS URL: %s\n", apiURL)
	if fingerprint != "" {
		cmd.Printf("  Root CA fingerprint (SHA-256): %s\n", fingerprint)
	}
	cmd.Println()
	cmd.Println("Open the Gateway HTTPS URL in your browser and accept the certificate warning after verifying the fingerprint matches.")
	cmd.Println("Do not disable TLS verification in your frontend code.")
}

// containsString reports whether s is present in list. Used by
// browserConfigMatches to check whether the frontend origin is already in the
// persisted AllowedOrigins or PasskeyRpOrigins slices.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// joinOrigins joins a slice of origin strings for display, returning "(none)"
// when the slice is empty. Used by renderConfigDelta to display current values.
func joinOrigins(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return strings.Join(origins, ", ")
}

// connectAPIBaseURL returns the HTTPS base URL of the Gateway (e.g.
// https://localhost:8443) used for display, the browser handoff, and manual
// trust instructions. It is constructed from the Gateway HTTPS port — no
// hardcoded URL strings.
func connectAPIBaseURL(httpsPort int) string {
	port := httpsPort
	if port == 0 {
		port = constants.Ports.OperatorHttps
	}
	return network.LocalhostHTTPSURL(port)
}

// connectHealthURL returns the full HTTPS health endpoint URL (e.g.
// https://localhost:8443/api/v1/health) used by the verifier. It appends the
// health API path constant to the base URL — no hardcoded path strings.
func connectHealthURL(httpsPort int) string {
	return connectAPIBaseURL(httpsPort) + constants.APIPaths.Health
}

func connectDiscoveryURL(httpPort int) string {
	port := httpPort
	if port == 0 {
		port = constants.Ports.OperatorHttp
	}
	return network.LocalhostHTTPURL(port) + constants.APIPaths.WellKnownPKICABundle
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsoleHTML_OperatorBinaryDownloadSectionPresent verifies that the console
// SPA includes the Operator Binary Download section with proper heading and description.
func TestConsoleHTML_OperatorBinaryDownloadSectionPresent(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "Operator Binary Download")
	assert.Contains(t, html, "renderOperatorDownloadsCard")
	assert.Contains(t, html, "operatorBinaries")
	assert.Contains(t, html, "Policy Execution Point")
}

// TestConsoleHTML_OperatorBinaryDownloadLinks verifies that the console
// renders download links for all supported platform binaries under the canonical
// /.well-known/g8e/bin/ path.
func TestConsoleHTML_OperatorBinaryDownloadLinks(t *testing.T) {
	html := indexHTML(t)

	expectedBinaries := []string{
		"g8e-linux-amd64",
		"g8e-linux-arm64",
		"g8e-linux-386",
		"g8e-darwin-arm64",
		"g8e-darwin-amd64",
		"g8e-windows-amd64.exe",
		"g8e-windows-arm64.exe",
	}

	assert.Contains(t, html, "/.well-known/g8e/bin/")
	for _, bin := range expectedBinaries {
		assert.Contains(t, html, bin, "expected binary %s to be listed in console SPA", bin)
	}
}

// TestConsoleHTML_OperatorBinaryCommandStrings verifies that the console
// displays the command strings for users to download, make executable, and start
// the operator on their workstation.
func TestConsoleHTML_OperatorBinaryCommandStrings(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "operator start -e")
	assert.Contains(t, html, "curl -fsSL")
	assert.Contains(t, html, "chmod +x")
	assert.Contains(t, html, "Invoke-WebRequest")
	assert.Contains(t, html, "/g8e-deploy.sh")
	assert.Contains(t, html, "/g8e-deploy.ps1")
}

// TestConsoleHTML_OperatorBinaryDownloadUsesHTTPOrigin verifies that the
// console constructs an HTTP origin (not HTTPS) for operator binary download
// URLs and deploy script commands. The binary download and deploy script
// endpoints are served on the plain HTTP port (default 8080), not the HTTPS
// console port (default 8443), so displayed and copied URLs must use HTTP to
// resolve to a live endpoint.
func TestConsoleHTML_OperatorBinaryDownloadUsesHTTPOrigin(t *testing.T) {
	html := indexHTML(t)

	// The card must construct an httpOrigin variable backed by http://.
	assert.Contains(t, html, "httpOrigin")
	assert.Contains(t, html, "'http://' + gwHost + ':8080'")

	// The deploy script and binary download commands must reference httpOrigin,
	// not the HTTPS location.origin.
	assert.Contains(t, html, "httpOrigin + '/g8e-deploy.sh")
	assert.Contains(t, html, "httpOrigin + '/g8e-deploy.ps1")
	assert.Contains(t, html, "httpOrigin + downloadPath")

	// The download anchor href must use the full HTTP URL, not a relative path
	// that would resolve against the HTTPS console origin.
	assert.Contains(t, html, "href: fullDownloadUrl")

	// The old HTTPS-based origin variable must not be used for download URLs.
	assert.NotContains(t, html, "origin + '/g8e-deploy.sh")
	assert.NotContains(t, html, "origin + '/g8e-deploy.ps1")
	assert.NotContains(t, html, "origin + downloadPath")
}

// TestConsoleHTML_OperatorBinaryDownloadCopyButton verifies that the console
// provides clipboard copy support for workstation commands.
func TestConsoleHTML_OperatorBinaryDownloadCopyButton(t *testing.T) {
	html := indexHTML(t)

	assert.Contains(t, html, "copyToClipboard")
	assert.Contains(t, html, "buildCodeBox")
	assert.Contains(t, html, "code-box")
	assert.Contains(t, html, "Copy")
	assert.Contains(t, html, "Copied!")
}

// TestConsoleHTML_OperatorBinaryDownloadRenderedInBothStates verifies that the
// Operator Binary Download card is rendered in both authenticated and unauthenticated
// views so users can access operator binaries at any time.
func TestConsoleHTML_OperatorBinaryDownloadRenderedInBothStates(t *testing.T) {
	html := indexHTML(t)

	// Count occurrences of renderOperatorDownloadsCard in the HTML.
	// It must be defined once and appended in both render() paths (unauthenticated and authenticated).
	count := strings.Count(html, "renderOperatorDownloadsCard()")
	require.GreaterOrEqual(t, count, 2, "renderOperatorDownloadsCard() should be called in both unauthenticated and authenticated render branches")
}

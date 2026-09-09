// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spf13/cobra"
)

// These regression tests lock in the current broken behavior of the GUI
// enrollment origin validator and RP ID derivation so that the fixes in
// Phase 1 of the v2.1.8 browser frontend connection UX rollout can be
// verified against a captured baseline. See plan Finding 0.1.
//
// Issue tracked: gui-rpid-derives-from-host-not-hostname
// The GUI validator accepts paths, queries, fragments, and user info, and
// printEnrollConfig derives the default RP ID from url.URL.Host (which
// includes the port) instead of url.URL.Hostname(), producing invalid
// WebAuthn RP IDs for any ported origin.

// TestValidateOrigin_AcceptsPathsAndQueries_CurrentBrokenBehavior asserts
// that the current validateOrigin implementation accepts origins that carry
// paths, query strings, fragments, and user information. A correct origin
// parser rejects all of these because a browser origin is scheme + host +
// port only. This test documents the broken behavior so the Phase 1 fix
// can flip each row to a rejection.
func TestValidateOrigin_AcceptsPathsAndQueries_CurrentBrokenBehavior(t *testing.T) {
	tests := []struct {
		name   string
		origin string
	}{
		{"path component accepted", "http://localhost:3003/app"},
		{"query and fragment accepted", "https://app.lovable.app/path?q=1#frag"},
		{"user information accepted", "https://user:pass@app.lovable.app"},
		{"trailing slash accepted", "https://app.lovable.app/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrigin(tt.origin)
			require.NoError(t, err, RegressionMarkerBeforeFix)
		})
	}

	// Regression marker: validateOrigin does not canonicalize or reject
	// path/query/fragment/userinfo components of a browser origin.
	_ = RegressionMarkerIssue
}

// TestPrintEnrollConfig_DerivesPortBearingRPID_CurrentBrokenBehavior
// asserts that printEnrollConfig derives the default passkey RP ID from
// url.URL.Host, which includes the port. For a ported origin such as
// https://your-app.lovable.app:8443 the emitted snippet contains
// PASSKEY_RP_ID = 'your-app.lovable.app:8443', which is an invalid
// WebAuthn RP ID. A correct derivation uses url.URL.Hostname() so the port
// is never included.
func TestPrintEnrollConfig_DerivesPortBearingRPID_CurrentBrokenBehavior(t *testing.T) {
	const portedOrigin = "https://your-app.lovable.app:8443"
	const wantRPID = "PASSKEY_RP_ID = 'your-app.lovable.app:8443'"

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	printEnrollConfig(cmd, portedOrigin, "", "", "")

	output := buf.String()
	assert.Contains(t, output, wantRPID, RegressionMarkerBeforeFix)
	// The correct (hostname-only) RP ID must NOT be present while the bug
	// is active. Once the fix lands this assertion flips.
	assert.NotContains(t, output, "PASSKEY_RP_ID = 'your-app.lovable.app'", RegressionMarkerBeforeFix)

	// Regression marker: printEnrollConfig derives the default RP ID from
	// url.URL.Host (host:port) instead of url.URL.Hostname().
	_ = RegressionMarkerIssue
}

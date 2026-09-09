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

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// These regression tests verify the fixed behavior of the GUI enrollment
// origin validator and RP ID derivation after Phase 1 of the v2.1.8
// browser frontend connection UX rollout centralized origin parsing in
// internal/cli/browserorigin. See plan Finding 0.1.
//
// Issue tracked: gui-rpid-derives-from-host-not-hostname
// The GUI validator previously accepted paths, queries, fragments, and
// user info, and printEnrollConfig derived the default RP ID from
// url.URL.Host (which includes the port) instead of url.URL.Hostname(),
// producing invalid WebAuthn RP IDs for any ported origin. Both paths now
// delegate to browserorigin.Parse / browserorigin.ValidateRPID, so the
// centralized parser rejects path/query/fragment/userinfo components and
// derives the RP ID from the hostname only.

// TestValidateOrigin_RejectsPathsQueriesFragmentsUserinfo_AfterFix asserts
// that the centralized validateOrigin implementation rejects origins that
// carry paths, query strings, fragments, and user information. A browser
// origin is scheme + host + port only. This is the flipped assertion from
// the Phase 0 baseline, which captured that the old validator accepted all
// of these.
func TestValidateOrigin_RejectsPathsQueriesFragmentsUserinfo_AfterFix(t *testing.T) {
	tests := []struct {
		name   string
		origin string
	}{
		{"path component rejected", "http://localhost:3003/app"},
		{"query and fragment rejected", "https://app.lovable.app/path?q=1#frag"},
		{"user information rejected", "https://user:pass@app.lovable.app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrigin(tt.origin)
			require.Error(t, err, RegressionMarkerAfterFix)
			assert.ErrorIs(t, err, constants.ErrValidationFailed, RegressionMarkerAfterFix)
		})
	}

	// Regression marker: validateOrigin now delegates to browserorigin.Parse,
	// which canonicalizes and rejects path/query/fragment/userinfo components
	// of a browser origin.
	_ = RegressionMarkerIssue
}

// TestValidateOrigin_AcceptsTrailingSlashRootPath_AfterFix asserts that the
// centralized parser still accepts a root path "/" (the only path a browser
// origin may carry) and strips it during canonicalization. This preserves
// compatibility with origins supplied as "https://app.lovable.app/".
func TestValidateOrigin_AcceptsTrailingSlashRootPath_AfterFix(t *testing.T) {
	err := validateOrigin("https://app.lovable.app/")
	require.NoError(t, err, RegressionMarkerAfterFix)
	_ = RegressionMarkerIssue
}

// TestPrintEnrollConfig_DerivesHostnameOnlyRPID_AfterFix asserts that
// printEnrollConfig derives the default passkey RP ID from the origin
// hostname only, never the host:port pair. For a ported origin such as
// https://your-app.lovable.app:8443 the emitted snippet contains
// PASSKEY_RP_ID = 'your-app.lovable.app', which is a valid WebAuthn RP ID.
// The port-bearing form the Phase 0 baseline captured is no longer present.
func TestPrintEnrollConfig_DerivesHostnameOnlyRPID_AfterFix(t *testing.T) {
	const portedOrigin = "https://your-app.lovable.app:8443"
	const wantRPID = "PASSKEY_RP_ID = 'your-app.lovable.app'"

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	printEnrollConfig(cmd, portedOrigin, "", "", "")

	output := buf.String()
	assert.Contains(t, output, wantRPID, RegressionMarkerAfterFix)
	// The port-bearing RP ID the Phase 0 baseline captured must no longer
	// be present now that derivation uses browserorigin.Parse(origin).RPID.
	assert.NotContains(t, output, "PASSKEY_RP_ID = 'your-app.lovable.app:8443'", RegressionMarkerAfterFix)

	// Regression marker: printEnrollConfig derives the default RP ID from
	// browserorigin.Parse(origin).RPID (hostname only), not url.URL.Host.
	_ = RegressionMarkerIssue
}

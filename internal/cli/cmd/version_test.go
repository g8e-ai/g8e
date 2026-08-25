// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"crypto/fips140"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
)

func TestVersionCmd_RegisteredOnRoot(t *testing.T) {
	rootCmd := NewRootCmd("dev", serve.VersionInfo{})
	for _, c := range rootCmd.Commands() {
		if c.Use == "version" {
			return
		}
	}
	t.Fatalf("version subcommand not registered on root")
}

func TestRunVersion_PlainPrintsBuildInfo(t *testing.T) {
	vi := serve.VersionInfo{
		Version:   "1.2.3",
		BuildID:   "abc123",
		BuildTime: "2026-07-31T00:00:00Z",
		Platform:  "linux/amd64",
	}
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, vi, false))

	out := buf.String()
	assert.Contains(t, out, "g8e version 1.2.3")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "2026-07-31T00:00:00Z")
	assert.Contains(t, out, "linux/amd64")
	assert.NotContains(t, out, "FIPS 140-3 mode")
}

func TestRunVersion_PlainOmitsEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, serve.VersionInfo{}, false))

	out := buf.String()
	assert.Contains(t, out, "g8e version ")
	assert.NotContains(t, out, "build id:")
	assert.NotContains(t, out, "build time:")
	assert.NotContains(t, out, "platform:")
}

func TestRunVersion_FIPSReportsModuleStatus(t *testing.T) {
	vi := serve.VersionInfo{Version: "1.2.3", Platform: "linux/amd64"}
	var buf bytes.Buffer
	err := runVersion(&buf, vi, true)

	out := buf.String()
	assert.Contains(t, out, "FIPS 140-3 mode:")
	assert.Contains(t, out, "FIPS enforcement:")
	assert.Contains(t, out, "FIPS module version:")

	// In the default (non-FIPS) test build, FIPS mode is not active, so the
	// self-check must surface that and return an error so auditors/scripts can
	// detect a non-compliant binary.
	if !fips140.Enabled() {
		require.Error(t, err)
		assert.Contains(t, out, "NOT active")
		assert.Contains(t, strings.ToLower(out), "gofips140=v1.0.0")
		return
	}
	// When the test binary itself was built with GOFIPS140, approved mode is on.
	// Enforcement off is the common production posture (e.g. when SSH streaming
	// needs non-approved primitives); the command must warn but exit 0 so
	// operators get a status report, not a false alarm. CI/release gates that
	// require the strict posture run under GODEBUG=fips140=only (see `make
	// verify-fips`).
	if !fips140.Enforced() {
		require.NoError(t, err)
		assert.Contains(t, out, "FIPS 140-3 mode:     enabled")
		assert.Contains(t, out, "FIPS enforcement:    disabled")
		assert.Contains(t, out, "WARNING: FIPS 140-3 approved mode is active but enforcement is OFF")
		return
	}
	require.NoError(t, err)
	assert.Contains(t, out, "FIPS 140-3 mode:     enabled")
	assert.Contains(t, out, "FIPS enforcement:    enabled")
	assert.NotContains(t, out, "WARNING:")
}

func TestVersionCmd_HasFipsFlag(t *testing.T) {
	cmd := versionCmd()
	f := cmd.Flags().Lookup("fips")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

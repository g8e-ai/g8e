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
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
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
	require.NoError(t, runVersion(&buf, vi, false, false))

	out := buf.String()
	assert.Contains(t, out, "g8e version 1.2.3")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "2026-07-31T00:00:00Z")
	assert.Contains(t, out, "linux/amd64")
	assert.NotContains(t, out, "FIPS 140-3 mode")
}

func TestRunVersion_PlainOmitsEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, serve.VersionInfo{}, false, false))

	out := buf.String()
	assert.Contains(t, out, "g8e version ")
	assert.NotContains(t, out, "build id:")
	assert.NotContains(t, out, "build time:")
	assert.NotContains(t, out, "platform:")
}

func TestRunVersion_FIPSReportsModuleStatus(t *testing.T) {
	vi := serve.VersionInfo{Version: "1.2.3", Platform: "linux/amd64"}
	var buf bytes.Buffer
	err := runVersion(&buf, vi, true, false)

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

func TestVersionCmd_HasJSONFlag(t *testing.T) {
	rootCmd := NewRootCmd("dev", serve.VersionInfo{})
	f := rootCmd.PersistentFlags().Lookup("json")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

func TestRunVersion_JSONEmitsProvenanceFields(t *testing.T) {
	vi := serve.VersionInfo{
		Version:             "2.1.8",
		BuildID:             "abc123",
		BuildTime:           "2026-09-12T00:00:00Z",
		Platform:            "linux_amd64",
		SourceRevision:      "deadbeef" + strings.Repeat("0", 32),
		SourceTreeStateHash: "a" + strings.Repeat("1", 63),
	}
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, vi, false, true))

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "2.1.8", out["version"])
	assert.Equal(t, "abc123", out["build_id"])
	assert.Equal(t, "deadbeef"+strings.Repeat("0", 32), out["source_revision"])
	assert.Equal(t, "a"+strings.Repeat("1", 63), out["source_tree_state_hash"])
	_, hasFIPS := out["fips140"]
	assert.False(t, hasFIPS, "fips140 must be omitted unless --fips is passed")
}

func TestRunVersion_JSONOmitsUnstampedSentinels(t *testing.T) {
	vi := serve.VersionInfo{
		Version:             "2.1.8",
		SourceRevision:      string(constants.SystemHealthUnknown),
		SourceTreeStateHash: "not-a-hash",
	}
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, vi, false, true))

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	_, hasRev := out["source_revision"]
	_, hasHash := out["source_tree_state_hash"]
	// A test binary carries no ldflags stamp; source_revision may still be
	// populated from toolchain VCS info, but a sentinel or malformed value
	// must never be emitted.
	if hasRev {
		assert.NotEqual(t, string(constants.SystemHealthUnknown), out["source_revision"])
	}
	assert.False(t, hasHash, "malformed tree-state hash must be omitted, not emitted")
}

func TestRunVersion_JSONPrefersStampedRevisionOverVCS(t *testing.T) {
	vi := serve.VersionInfo{Version: "2.1.8", SourceRevision: "stamped-revision"}
	var buf bytes.Buffer
	require.NoError(t, runVersion(&buf, vi, false, true))

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "stamped-revision", out["source_revision"])
}

func TestEffectiveSourceRevision_FallsBackForUnstampedValues(t *testing.T) {
	vcs := buildinfo.VCSStamp{Revision: "vcs-revision", Present: true}
	for _, value := range []string{"", string(constants.SystemHealthUnknown), constants.BuildMetadataUnavailable} {
		t.Run(value, func(t *testing.T) {
			assert.Equal(t, "vcs-revision", effectiveSourceRevision(serve.VersionInfo{SourceRevision: value}, vcs))
		})
	}
}

func TestEffectiveBuildID_FallsBackToSourceTreeHash(t *testing.T) {
	hash := "a" + strings.Repeat("1", 63)
	vi := serve.VersionInfo{BuildID: constants.BuildMetadataUnavailable, SourceTreeStateHash: hash}

	assert.Equal(t, hash, effectiveBuildID(vi))
}

func TestRunVersion_JSONFIPSIncludesModuleBlock(t *testing.T) {
	vi := serve.VersionInfo{Version: "1.2.3"}
	var buf bytes.Buffer
	err := runVersion(&buf, vi, true, true)

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	fipsBlock, hasFIPS := out["fips140"].(map[string]any)
	require.True(t, hasFIPS, "--json --fips must include the fips140 block")
	assert.Contains(t, fipsBlock, "enabled")
	assert.Contains(t, fipsBlock, "module_version")

	if !fips140.Enabled() {
		require.Error(t, err)
		return
	}
	require.NoError(t, err)
}

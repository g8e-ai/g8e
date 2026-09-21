// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"crypto/fips140"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// versionCmd reports g8e build metadata. With --fips it additionally runs a
// FIPS 140-3 self-check against the Go Cryptographic Module via the native
// crypto/fips140 package, giving operators and auditors a verifiable signal
// that the deployed binary is within the validated boundary (CMVP Cert #5247).
//
// The check inspects the module's own state — it does NOT probe environment
// variables. FIPS mode is activated at build time via GOFIPS140; the binary
// enters approved mode by default and runs its integrity/CAST self-tests at
// init, so no runtime env var is required.
func versionCmd() *cobra.Command {
	var fips bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print g8e build version information",
		Long: `Print g8e build version information (version, build ID, build time, platform).

With --json, the same fields are emitted as a JSON object along with the
source provenance stamp (source_revision, source_tree_state_hash, and
source_tree_modified when the toolchain recorded VCS state). This is the
machine-readable surface the evals provenance bridge consumes instead of
environment variables.

With --fips, also report the FIPS 140-3 status of the running binary by
querying the Go Cryptographic Module (crypto/fips140). A binary built with
GOFIPS140=v1.0.0 enters FIPS approved mode by default; this flag confirms
that mode is active and prints the validated module version.

The command exits non-zero only if FIPS approved mode is NOT active. If
approved mode is active but enforcement is off (the common production
posture when non-approved primitives such as ChaCha20-Poly1305 SSH are
required), the command prints a warning and exits 0 — this is informational
for operators, not a failure. CI/release gates that require the strict
posture should run the binary under GODEBUG=fips140=only (see 'make
verify-fips').`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(cmd.OutOrStdout(), versionInfoFromCmd(cmd), fips, output.JSONEnabled(cmd))
		},
	}
	cmd.Flags().BoolVar(&fips, "fips", false, "report FIPS 140-3 module status; exit non-zero only if approved mode is not active")
	return cmd
}

// fipsStatusJSON is the FIPS 140-3 block of `version --json` output.
type fipsStatusJSON struct {
	Enabled       bool   `json:"enabled"`
	Enforced      bool   `json:"enforced"`
	ModuleVersion string `json:"module_version"`
}

// versionJSON is the `version --json` output contract. Snake_case field
// names match the canonical JSON convention used by `auth context` and are
// consumed by the evals provenance bridge. Unstamped fields are omitted
// rather than emitted as sentinel strings.
type versionJSON struct {
	Version             string          `json:"version"`
	BuildID             string          `json:"build_id,omitempty"`
	BuildTime           string          `json:"build_time,omitempty"`
	Platform            string          `json:"platform,omitempty"`
	SourceRevision      string          `json:"source_revision,omitempty"`
	SourceTreeStateHash string          `json:"source_tree_state_hash,omitempty"`
	SourceTreeModified  *bool           `json:"source_tree_modified,omitempty"`
	FIPS140             *fipsStatusJSON `json:"fips140,omitempty"`
}

func isUnstampedMetadata(value string) bool {
	switch strings.TrimSpace(value) {
	case "", string(constants.SystemHealthUnknown), constants.BuildMetadataUnavailable:
		return true
	default:
		return false
	}
}

// effectiveBuildID prefers the explicit build identity and falls back to the
// source-tree state hash for local builds without release metadata.
func effectiveBuildID(vi serve.VersionInfo) string {
	if buildID := strings.TrimSpace(vi.BuildID); !isUnstampedMetadata(buildID) {
		return buildID
	}
	if isHex64(vi.SourceTreeStateHash) {
		return vi.SourceTreeStateHash
	}
	return ""
}

// effectiveSourceRevision prefers the ldflags stamp and falls back to the
// toolchain-embedded VCS revision so a plain `go build` still carries a
// source identity.
func effectiveSourceRevision(vi serve.VersionInfo, vcs buildinfo.VCSStamp) string {
	if sourceRevision := strings.TrimSpace(vi.SourceRevision); !isUnstampedMetadata(sourceRevision) {
		return sourceRevision
	}
	return vcs.Revision
}

func runVersion(w io.Writer, vi serve.VersionInfo, fips bool, asJSON bool) error {
	vcs := buildinfo.ReadVCSStamp()
	if asJSON {
		return writeVersionJSON(w, vi, vcs, fips)
	}

	fmt.Fprintf(w, "g8e version %s\n", vi.Version)
	if buildID := effectiveBuildID(vi); buildID != "" {
		fmt.Fprintf(w, "build id:    %s\n", buildID)
	}
	if vi.BuildTime != "" {
		fmt.Fprintf(w, "build time:  %s\n", vi.BuildTime)
	}
	if vi.Platform != "" {
		fmt.Fprintf(w, "platform:    %s\n", vi.Platform)
	}
	if rev := effectiveSourceRevision(vi, vcs); !isUnstampedMetadata(rev) {
		fmt.Fprintf(w, "source rev:  %s\n", rev)
	}
	if isHex64(vi.SourceTreeStateHash) {
		fmt.Fprintf(w, "source tree: %s\n", vi.SourceTreeStateHash)
	}
	if vcs.Present && vcs.Modified {
		fmt.Fprintln(w, "source state: modified (uncommitted changes at build time)")
	}

	if !fips {
		return nil
	}

	fmt.Fprintln(w)
	enabled := fips140.Enabled()
	enforced := fips140.Enforced()
	moduleVersion := fips140.Version()

	fmt.Fprintf(w, "FIPS 140-3 mode:     %s\n", fipsBoolStr(enabled))
	fmt.Fprintf(w, "FIPS enforcement:    %s\n", fipsBoolStr(enforced))
	fmt.Fprintf(w, "FIPS module version: %s\n", moduleVersion)

	if !enabled {
		fmt.Fprintln(w)
		fmt.Fprint(w, "FIPS 140-3 approved mode is NOT active. Build with GOFIPS140=v1.0.0 to link\n")
		fmt.Fprint(w, "the Go Cryptographic Module (CMVP Cert #5247) and enable approved mode by\n")
		fmt.Fprint(w, "default (e.g. `make build-fips` or the Dockerfile builder stage).\n")
		return constants.ErrFIPSModeNotActive
	}
	if !enforced {
		// Approved mode is active but enforcement is off. This is the common
		// production posture when non-approved primitives (e.g. ChaCha20-Poly1305
		// for SSH streaming) are required. Warn but do not fail — operators get a
		// status report, not a false alarm. CI/release gates that need the strict
		// posture run under GODEBUG=fips140=only (see `make verify-fips`).
		fmt.Fprintln(w)
		fmt.Fprint(w, "WARNING: FIPS 140-3 approved mode is active but enforcement is OFF.\n")
		fmt.Fprint(w, "Non-approved cryptographic primitives are not rejected at runtime.\n")
		fmt.Fprint(w, "Set GODEBUG=fips140=only in the process environment to enable enforcement\n")
		fmt.Fprint(w, "(e.g. `GODEBUG=fips140=only ./g8e version --fips`).\n")
	}
	return nil
}

// writeVersionJSON emits the machine-readable version record. The FIPS
// block is included only when requested; the same exit contract applies —
// approved-mode-inactive still returns an error after the JSON is written.
func writeVersionJSON(w io.Writer, vi serve.VersionInfo, vcs buildinfo.VCSStamp, fips bool) error {
	out := versionJSON{
		Version:   vi.Version,
		BuildID:   effectiveBuildID(vi),
		BuildTime: vi.BuildTime,
		Platform:  vi.Platform,
	}
	if rev := effectiveSourceRevision(vi, vcs); !isUnstampedMetadata(rev) {
		out.SourceRevision = rev
	}
	if isHex64(vi.SourceTreeStateHash) {
		out.SourceTreeStateHash = vi.SourceTreeStateHash
	}
	if vcs.Present {
		modified := vcs.Modified
		out.SourceTreeModified = &modified
	}
	var fipsErr error
	if fips {
		enabled := fips140.Enabled()
		out.FIPS140 = &fipsStatusJSON{
			Enabled:       enabled,
			Enforced:      fips140.Enforced(),
			ModuleVersion: fips140.Version(),
		}
		if !enabled {
			fipsErr = constants.ErrFIPSModeNotActive
		}
	}
	if err := output.WriteJSON(w, out); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return fipsErr
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func fipsBoolStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

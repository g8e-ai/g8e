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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
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
			return runVersion(cmd.OutOrStdout(), versionInfoFromCmd(cmd), fips)
		},
	}
	cmd.Flags().BoolVar(&fips, "fips", false, "report FIPS 140-3 module status; exit non-zero only if approved mode is not active")
	return cmd
}

func runVersion(w io.Writer, vi serve.VersionInfo, fips bool) error {
	fmt.Fprintf(w, "g8e version %s\n", vi.Version)
	if vi.BuildID != "" {
		fmt.Fprintf(w, "build id:    %s\n", vi.BuildID)
	}
	if vi.BuildTime != "" {
		fmt.Fprintf(w, "build time:  %s\n", vi.BuildTime)
	}
	if vi.Platform != "" {
		fmt.Fprintf(w, "platform:    %s\n", vi.Platform)
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
		return fmt.Errorf("fips 140-3 mode is not active")
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

func fipsBoolStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

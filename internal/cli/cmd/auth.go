// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// Enroller is the interface satisfied by *auth.EnrollmentCoordinator. The
// command layer depends on this interface rather than the concrete type so
// tests can inject a mock coordinator that records calls and returns canned
// EnrollmentResults without network I/O, sudo, or browser launches.
type Enroller interface {
	Enroll(ctx context.Context, opts auth.EnrollmentOptions) (*auth.EnrollmentResult, error)
}

// enrollerFactory builds an Enroller from an output function, file service,
// and config. It is injected through *WithConfig constructors (mirroring
// fileSvcFactory) so production wires newDefaultEnrollmentCoordinator and
// tests wire a stub — no package-level mutable state.
type enrollerFactory func(out auth.OutputFunc, fileSvc fs.RuntimeFileService, cfg *config.Config) Enroller

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and session management",
		Long:  `Manage mTLS enrollment and CLI/web/operator sessions via CSR-based authentication.`,
	}

	cmd.AddCommand(
		enrollCmd(),
		logoutCmd(),
		approveCmd(),
		approveRecoveryCmd(),
	)

	return cmd
}

// newDefaultEnrollmentCoordinator is the production coordinator factory. It
// injects production defaults (real gateway client, file-backed key provider,
// real system-trust installer, real browser opener, hardened passkey
// registrar, stdin-reading confirm and continue functions) and an OutputFunc
// that writes to the provided output sink.
func newDefaultEnrollmentCoordinator(out auth.OutputFunc, fileSvc fs.RuntimeFileService, cfg *config.Config) Enroller {
	return auth.NewEnrollmentCoordinator(auth.EnrollmentCoordinatorDeps{
		FileSvc:  fileSvc,
		Cfg:      cfg,
		Out:      out,
		Confirm:  stdinConfirm,
		Continue: stdinContinue,
		Logger:   slog.Default(),
	})
}

// stdinConfirm prints the prompt to stdout and reads a y/N response from
// stdin. Returns true only for "y" or "Y". Used by the coordinator to confirm
// stale trust anchor removal before proceeding.
func stdinConfirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(response)
	return response == "y" || response == "Y"
}

// stdinContinue prints the prompt to stdout and blocks until the user presses
// Enter. Returns true on Enter (the user confirmed they closed their browser),
// false on read error. Used by the coordinator to gate the passkey ceremony
// behind a blocking browser-restart prompt after the trust store changed.
func stdinContinue(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	if _, err := reader.ReadString('\n'); err != nil {
		return false
	}
	return true
}

// enrollCmd is the parent command for enrollment. It has no RunE, so cobra
// prints help and exits non-zero when invoked without a subcommand, forcing
// explicit session-type selection (user vs operator).
func enrollCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll a CLI user or remote operator with the running Gateway",
		Long: `Enroll a CLI user or remote operator with the running Gateway.

Two distinct enrollment paths exist as subcommands:

  user      Local human CLI/user enrollment. Drives the EnrollmentCoordinator
            state machine (bootstrap, recovery, rotation, reuse), installs the
            gateway root CA into the OS trust store, and runs the browser-based
            WebAuthn passkey ceremony. Produces a CLI session bound to a user
            identity.

  operator  Remote operator/device enrollment. Generates an operator CSR and
            enrolls with the gateway to obtain Operator mTLS certificates.
            Headless and operator-only: no OS trust installation, no passkey
            ceremony, no CLI session.

Bare ` + "`auth enroll`" + ` (no subcommand) prints this help and exits non-zero.`,
	}
	cmd.AddCommand(
		enrollUserCmd(),
		enrollOperatorCmd(),
		guiCmd(),
	)
	return cmd
}

func enrollUserCmd() *cobra.Command {
	return enrollUserCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
}

func enrollUserCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	checkOperatorRunning func(*config.Config) error,
	enrollerFactory enrollerFactory,
) *cobra.Command {
	var (
		noSystemTrust bool
		rotateCLI     bool
		headless      bool
	)
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Enroll a local CLI user session with the running Gateway and register a passkey",
		Long: `Enroll a CLI session with the running Gateway via CSR-based enrollment, then register a passkey for secure authentication.

The coordinator inspects the local CLI identity and chooses the correct action:
  - No local identity on an unbootstrapped gateway: bootstrap (creates the first user/session).
  - No local identity on a bootstrapped gateway: human-approved CLI recovery (a one-time
    approval in the gateway console with an existing passkey).
  - Complete, valid identity: reuse it (no new certificate is issued).
  - Complete, expiring identity: rotate via the mTLS rotation endpoint exactly once.
  - Partial or corrupt local state: human-approved recovery (never silently overwrite one file).

OS trust installation runs BEFORE the browser-based passkey ceremony by default. If
system trust installation fails, the browser phase is not started. Use --no-system-trust
to skip the installer when an administrator has pre-installed the gateway root CA; the
passkey ceremony still runs and runtime mTLS/trust-bundle errors still fail enrollment.

For the remote operator/device enrollment path (CSR-only, no passkey, no CLI session),
use ` + "`auth enroll operator`" + ` instead.

The Gateway must already be running (use './g8e gw start' first).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			if err := checkOperatorRunning(cfg); err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			coordinator := enrollerFactory(func(format string, args ...any) {
				cmd.Printf(format+"\n", args...)
			}, fileSvc, cfg)
			result, err := coordinator.Enroll(cmd.Context(), auth.EnrollmentOptions{
				NoSystemTrust: noSystemTrust || headless,
				RotateCLI:     rotateCLI,
				Headless:      headless,
			})
			if err != nil {
				return err
			}

			// Progress lines for the user-visible identity. The coordinator
			// already prints intermediate progress via OutputFunc; these are
			// the final summary lines so the user sees the bound identity.
			if result.Reused {
				cmd.Printf("Reusing existing CLI identity (no new certificate issued).\n")
			} else {
				cmd.Printf("\nCLI session %s complete\n", result.Source)
			}
			cmd.Printf("User ID: %s\n", result.UserID)
			cmd.Printf("CLI Session ID: %s\n", result.CLISessionID)
			if result.SystemTrustInstalled {
				cmd.Println("System trust: installed gateway root CA.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noSystemTrust, "no-system-trust", false,
		"Skip OS trust installation (administrator must have pre-installed the gateway root CA). The passkey ceremony still runs.")
	cmd.Flags().BoolVar(&rotateCLI, "rotate-cli", false,
		"Force an mTLS CLI rotation even when the local identity is complete and not expiring.")
	cmd.Flags().BoolVar(&headless, "headless", false,
		"Enroll a CLI-only identity without a browser. Skips passkey registration and OS trust installation; recovery approval is delegated to an already-enrolled CLI via 'g8e auth approve-recovery <token>'. The resulting identity is mTLS-only and cannot authenticate to the Console SPA.")
	return cmd
}

func logoutCmd() *cobra.Command {
	return logoutCmdWithConfig(loadConfig, newFileSvc)
}

func logoutCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear local Operator session and credentials",
		Long: `Clear the local Operator session by deleting stored credentials from disk. This does not revoke the session on the gateway side — it only removes the local credential files so the CLI can no longer authenticate.

The shared OS root CA (runtime trust bundle) is NOT removed. System trust is
shared and may be used by another runtime or gateway; logout only clears the
local CLI credential material (credentials JSON, CLI cert, CLI key).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			store := auth.NewCredentialStore(fileSvc, cfg)
			creds, err := store.LoadCredentials(cmd.Context())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
			}

			if creds == nil {
				cmd.Println("No active session found")
				return nil
			}

			if err := store.Clear(cmd.Context()); err != nil {
				return err
			}

			cmd.Println("Logged out successfully")
			return nil
		},
	}
	return cmd
}

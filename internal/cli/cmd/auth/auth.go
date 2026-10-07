// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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
type EnrollerFactory func(out auth.OutputFunc, fileSvc fs.RuntimeFileService, cfg *config.Config) (Enroller, error)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and session management",
		Long: `Manage mTLS enrollment and CLI/web/operator sessions via CSR-based authentication.

Use 'g8e login' and 'g8e logout' to sign in and out.`,
	}

	cmd.AddCommand(
		enrollCmd(),
		approveCmd(),
		approveRecoveryCmd(),
		refreshCmd(),
		authContextCmd(),
	)

	return cmd
}

// newDefaultEnrollmentCoordinator is the production coordinator factory. It
// injects production defaults (real gateway client, file-backed key provider,
// real system-trust installer, real browser opener, hardened passkey
// registrar, stdin-reading confirm and continue functions) and an OutputFunc
// that writes to the provided output sink.
func NewDefaultEnrollmentCoordinator(out auth.OutputFunc, fileSvc fs.RuntimeFileService, cfg *config.Config) (Enroller, error) {
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
// explicit session-type selection (user vs platform workload approval).
func enrollCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll CLI users and manage platform workload enrollment",
		Long: `Enroll CLI users and manage platform workload enrollment with the running Gateway.

  user      Local human CLI/user enrollment. Drives the EnrollmentCoordinator
            state machine (bootstrap, recovery, rotation, reuse), installs the
            gateway root CA into the OS trust store, and runs the browser-based
            WebAuthn passkey ceremony. Produces a CLI session bound to a user
            identity.

  app       Application platform workload enrollment. Generates a private key
            and CSR, submits an enrollment request to the Gateway, and polls
            until approved by an owner.

  pending   List pending platform workload enrollment requests (dashboard,
            ensemble, or operator) awaiting an owner decision.

  list      List completed or revoked platform workload enrollments with
            enrollment request IDs for revocation.

  approve   Approve pending platform workload enrollment requests by request
            ID, instance ID, or hostname (several at once), or all with --all.

  deny      Deny pending platform workload enrollment requests (same
            selectors as approve).

  revoke    Revoke a completed platform workload enrollment.

Bare ` + "`auth enroll`" + ` (no subcommand) prints this help and exits non-zero.`,
	}
	cmd.AddCommand(
		enrollUserCmd(),
		enrollAppCmd(),
		pendingPlatformEnrollmentCmd(),
		listPlatformEnrollmentCmd(),
		approvePlatformEnrollmentCmd(),
		denyPlatformEnrollmentCmd(),
		revokePlatformEnrollmentCmd(),
	)
	return cmd
}

func enrollUserCmd() *cobra.Command {
	return enrollUserCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, auth.CheckOperatorRunning, NewDefaultEnrollmentCoordinator)
}

func enrollUserCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	checkOperatorRunning func(*config.Config) error,
	EnrollerFactory EnrollerFactory,
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

For the remote operator enrollment path (CSR-only, no passkey, no CLI session),
use ` + "`auth enroll operator`" + ` instead.

The Gateway must already be running (use './g8e gw start' first).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserEnrollment(cmd, userEnrollmentDeps{
				configLoader:         configLoader,
				fileSvcFactory:       fileSvcFactory,
				checkOperatorRunning: checkOperatorRunning,
				enrollerFactory:      EnrollerFactory,
			}, auth.EnrollmentOptions{
				NoSystemTrust: noSystemTrust || headless,
				RotateCLI:     rotateCLI,
				Headless:      headless,
			})
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

// userEnrollmentDeps groups the injectable collaborators shared by
// 'auth enroll user' and 'login'.
type userEnrollmentDeps struct {
	configLoader         func(string) (*config.Config, error)
	fileSvcFactory       func(string, *slog.Logger) (fs.RuntimeFileService, error)
	checkOperatorRunning func(*config.Config) error
	enrollerFactory      EnrollerFactory
}

// runUserEnrollment drives the EnrollmentCoordinator for a local CLI user and
// prints the bound identity. It is the single implementation behind
// 'auth enroll user' and every 'login' form, which differ only in opts.
func runUserEnrollment(cmd *cobra.Command, deps userEnrollmentDeps, opts auth.EnrollmentOptions) error {
	cfg, err := deps.configLoader("")
	if err != nil {
		return err
	}

	if err := deps.checkOperatorRunning(cfg); err != nil {
		return err
	}

	fileSvc, err := deps.fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	coordinator, err := deps.enrollerFactory(func(format string, args ...any) {
		cmd.Printf(format+"\n", args...)
	}, fileSvc, cfg)
	if err != nil {
		return err
	}
	result, err := coordinator.Enroll(cmd.Context(), opts)
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
}

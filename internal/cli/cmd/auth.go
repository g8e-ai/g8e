// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

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
	)

	return cmd
}

// enrollCoordinatorFactory builds an EnrollmentCoordinator from an output
// function, file service, and config. It is a package-level var so tests can
// swap it for a mock coordinator that avoids network I/O, sudo, and browser
// launches.
var enrollCoordinatorFactory = newDefaultEnrollmentCoordinator

// newDefaultEnrollmentCoordinator is the production coordinator factory. It
// injects production defaults (real gateway client, file-backed key provider,
// real system-trust installer, real browser opener, hardened passkey
// registrar) and an OutputFunc that writes to the provided output sink.
func newDefaultEnrollmentCoordinator(out auth.OutputFunc, fileSvc fs.RuntimeFileService, cfg *config.Config) *auth.EnrollmentCoordinator {
	return auth.NewEnrollmentCoordinator(auth.EnrollmentCoordinatorDeps{
		FileSvc: fileSvc,
		Cfg:     cfg,
		Out:     out,
		Logger:  slog.Default(),
	})
}

func enrollCmd() *cobra.Command {
	return enrollCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning)
}

func enrollCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	checkOperatorRunning func(*config.Config) error,
) *cobra.Command {
	var (
		noSystemTrust bool
		rotateCLI     bool
	)
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll CLI session with the running Gateway and register a passkey",
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

			coordinator := enrollCoordinatorFactory(func(format string, args ...any) {
				cmd.Printf(format+"\n", args...)
			}, fileSvc, cfg)
			result, err := coordinator.Enroll(cmd.Context(), auth.EnrollmentOptions{
				NoSystemTrust: noSystemTrust,
				RotateCLI:     rotateCLI,
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
				cmd.Println("System trust: installed gateway root CA. Restart any open browsers so they pick up the new trust anchor.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noSystemTrust, "no-system-trust", false,
		"Skip OS trust installation (administrator must have pre-installed the gateway root CA). The passkey ceremony still runs.")
	cmd.Flags().BoolVar(&rotateCLI, "rotate-cli", false,
		"Force an mTLS CLI rotation even when the local identity is complete and not expiring.")
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

The shared OS root CA is NOT removed. System trust is shared and may be used by
another runtime or gateway; logout only clears the local CLI credential material
(credentials JSON, CLI cert, CLI key, and the runtime trust bundle).`,
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

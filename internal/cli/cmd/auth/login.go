// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// LoginCmd returns the top-level 'login' command.
func LoginCmd() *cobra.Command {
	return loginCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, auth.CheckOperatorRunning, NewDefaultEnrollmentCoordinator)
}

// loginCmdWithConfig builds 'login' with injectable collaborators. Every form
// runs the same EnrollmentCoordinator as 'auth enroll user'; they differ only
// in the options passed. Bare 'login' and 'login web' are identical and are
// never headless.
func loginCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	checkOperatorRunning func(*config.Config) error,
	enrollerFactory EnrollerFactory,
) *cobra.Command {
	deps := userEnrollmentDeps{
		configLoader:         configLoader,
		fileSvcFactory:       fileSvcFactory,
		checkOperatorRunning: checkOperatorRunning,
		enrollerFactory:      enrollerFactory,
	}

	cmd := newWebLoginCmd("login", deps)
	cmd.Short = "Log in to the Gateway (browser passkey by default; 'login cli' for headless)"
	cmd.Long = `Log in to the running Gateway. Bare 'login' and 'login web' sign in through the
browser: they enroll a CLI session, install the gateway root CA into the OS trust
store, and register or confirm a passkey. The resulting identity can use both the
CLI and the Console.

  web              Browser login (the default). Same as bare 'login'.
  cli (headless)   Headless CLI-only login with no browser. The identity is mTLS-only
                   and cannot authenticate to the Console. On a bootstrapped gateway
                   recovery is approved from an already-enrolled CLI with
                   'g8e auth approve-recovery <token>'.

The coordinator inspects the local CLI identity and chooses bootstrap, recovery,
rotation, or reuse exactly as 'auth enroll user' does. The Gateway must already be
running (use './g8e gw start' first). To sign out, run 'g8e logout'.`
	cmd.AddCommand(
		newWebLoginCmd("web", deps),
		loginCLICmd(deps),
	)
	return cmd
}

// newWebLoginCmd builds the browser login form under the given name.
func newWebLoginCmd(use string, deps userEnrollmentDeps) *cobra.Command {
	var (
		noSystemTrust bool
		rotateCLI     bool
	)
	cmd := &cobra.Command{
		Use:   use,
		Short: "Log in through the browser with a passkey",
		Long: `Log in through the browser: enroll a CLI session, install the gateway root CA into
the OS trust store, and register or confirm a passkey. This is the default login.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserEnrollment(cmd, deps, auth.EnrollmentOptions{
				NoSystemTrust: noSystemTrust,
				RotateCLI:     rotateCLI,
			})
		},
	}
	cmd.Flags().BoolVar(&noSystemTrust, "no-system-trust", false,
		"Skip OS trust installation (administrator must have pre-installed the gateway root CA). The passkey ceremony still runs.")
	cmd.Flags().BoolVar(&rotateCLI, "rotate-cli", false,
		"Force an mTLS CLI rotation even when the local identity is complete and not expiring.")
	return cmd
}

// loginCLICmd builds the headless login form.
func loginCLICmd(deps userEnrollmentDeps) *cobra.Command {
	var rotateCLI bool
	cmd := &cobra.Command{
		Use:     "cli",
		Aliases: []string{"headless"},
		Short:   "Log in headless, without a browser (CLI-only identity)",
		Long: `Log in without a browser. Enrolls a CLI-only identity: passkey registration and OS
trust installation are skipped, and recovery approval is delegated to an
already-enrolled CLI via 'g8e auth approve-recovery <token>'. The resulting
identity is mTLS-only and cannot authenticate to the Console.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserEnrollment(cmd, deps, auth.EnrollmentOptions{
				NoSystemTrust: true,
				RotateCLI:     rotateCLI,
				Headless:      true,
			})
		},
	}
	cmd.Flags().BoolVar(&rotateCLI, "rotate-cli", false,
		"Force an mTLS CLI rotation even when the local identity is complete and not expiring.")
	return cmd
}

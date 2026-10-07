// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// LogoutClient is the Gateway call logout depends on. The command layer takes
// this interface instead of *auth.EnrollmentClient so tests can record the
// requested scope without network I/O.
type LogoutClient interface {
	Logout(ctx context.Context, fileSvc fs.RuntimeFileService, scope constants.LogoutScope) (auth.CLISessionLogout, error)
}

// LogoutClientFactory builds a LogoutClient for a loaded config.
type LogoutClientFactory func(cfg *config.Config) LogoutClient

func defaultLogoutClientFactory(cfg *config.Config) LogoutClient {
	return auth.NewEnrollmentClient(cfg, nil)
}

// logoutOutput is the --json shape of a logout.
type logoutOutput struct {
	Success                   bool                  `json:"success"`
	Scope                     constants.LogoutScope `json:"scope"`
	UserID                    string                `json:"user_id,omitempty"`
	LocalOnly                 bool                  `json:"local_only"`
	LocalCredentialsCleared   bool                  `json:"local_credentials_cleared"`
	WebSessionsTerminated     int                   `json:"web_sessions_terminated"`
	CLISessionsTerminated     int                   `json:"cli_sessions_terminated"`
	CLICertificatesRevoked    int                   `json:"cli_certificates_revoked"`
	UnboundOperatorSessionIDs []string              `json:"unbound_operator_session_ids"`
}

// LogoutCmd returns the top-level 'logout' command.
func LogoutCmd() *cobra.Command {
	return logoutCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, defaultLogoutClientFactory)
}

type logoutDeps struct {
	configLoader   func(string) (*config.Config, error)
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)
	clientFactory  LogoutClientFactory
}

// logoutCmdWithConfig builds 'logout' with injectable collaborators. Bare
// 'logout' ends every web and CLI session of the user; 'logout web' and
// 'logout cli' end one kind.
func logoutCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	clientFactory LogoutClientFactory,
) *cobra.Command {
	deps := logoutDeps{configLoader: configLoader, fileSvcFactory: fileSvcFactory, clientFactory: clientFactory}

	cmd := newLogoutScopeCmd("logout", constants.LogoutScopeAll, deps)
	cmd.Short = "Log out: unbind operators and end all web and CLI sessions"
	cmd.Long = `Log the current user out everywhere. The Gateway:

  - unbinds every operator bound to the user's web or CLI sessions (the
    operators themselves keep running; only the bindings are removed),
  - ends every web session and every CLI session of the user, on every machine,
  - revokes the CLI certificates behind those CLI sessions.

Local CLI credentials (credentials JSON, CLI certificate, CLI key) are then
deleted from this machine. The shared OS root CA (runtime trust bundle) is NOT
removed: system trust is shared and may be used by another runtime or gateway.

Because the certificates are revoked, other machines that were logged in must
clear their stale local credentials ('g8e logout --local-only') before they can
'g8e login' again. Logging back in on a bootstrapped gateway uses the recovery
flow, approved with an existing passkey in the Console.

  web   End only the web sessions (and unbind operators bound to them). The CLI
        session, its certificate, and local credentials are kept.
  cli   End only the CLI sessions and revoke their certificates (and unbind
        operators bound to them). Web sessions are kept.

Local credentials are cleared only after the Gateway confirms the logout. If the
Gateway cannot be reached, nothing is changed; --local-only discards the local
credentials without contacting it and does NOT end any server-side session.`
	cmd.AddCommand(
		newLogoutScopeCmd("web", constants.LogoutScopeWeb, deps),
		newLogoutScopeCmd("cli", constants.LogoutScopeCLI, deps),
	)
	return cmd
}

// newLogoutScopeCmd builds the logout form for one scope. --local-only exists
// only where local credentials are cleared (scopes all and cli).
func newLogoutScopeCmd(use string, scope constants.LogoutScope, deps logoutDeps) *cobra.Command {
	var localOnly bool
	cmd := &cobra.Command{
		Use:  use,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout(cmd, deps, scope, localOnly)
		},
	}
	switch scope {
	case constants.LogoutScopeWeb:
		cmd.Short = "Log out of all web sessions"
		cmd.Long = `End every web session of the user and unbind the operators bound to them. The CLI
session, its certificate, and local credentials are kept.`
	case constants.LogoutScopeCLI:
		cmd.Short = "Log out of all CLI sessions and revoke their certificates"
		cmd.Long = `End every CLI session of the user, on every machine, unbind the operators bound to
them, and revoke the CLI certificates behind them. Local credentials are deleted
after the Gateway confirms. Web sessions are kept.`
	}
	if scope.IncludesCLI() {
		cmd.Flags().BoolVar(&localOnly, "local-only", false,
			"Delete local CLI credentials without contacting the Gateway. Server-side sessions, operator bindings, and certificates are NOT changed.")
	}
	return cmd
}

func runLogout(cmd *cobra.Command, deps logoutDeps, scope constants.LogoutScope, localOnly bool) error {
	cfg, err := deps.configLoader("")
	if err != nil {
		return err
	}

	fileSvc, err := deps.fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	store := auth.NewCredentialStore(fileSvc, cfg)
	creds, err := store.LoadCredentials(cmd.Context())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}

	result := logoutOutput{Success: true, Scope: scope, LocalOnly: localOnly, UnboundOperatorSessionIDs: []string{}}
	if creds == nil {
		return writeLogoutResult(cmd, result, "No active session found")
	}
	result.UserID = creds.UserID

	if !localOnly {
		res, err := deps.clientFactory(cfg).Logout(cmd.Context(), fileSvc, scope)
		if err != nil {
			return fmt.Errorf("logout: the Gateway did not confirm the logout, so nothing was changed locally "+
				"(retry, or use --local-only to discard local credentials without ending server-side sessions): %w", err)
		}
		result.UserID = res.UserID
		result.WebSessionsTerminated = res.WebSessionsTerminated
		result.CLISessionsTerminated = res.CLISessionsTerminated
		result.CLICertificatesRevoked = res.CLICertificatesRevoked
		if res.UnboundOperatorSessionIDs != nil {
			result.UnboundOperatorSessionIDs = res.UnboundOperatorSessionIDs
		}
	}

	if scope.IncludesCLI() {
		if err := store.Clear(cmd.Context()); err != nil {
			return err
		}
		result.LocalCredentialsCleared = true
	}

	return writeLogoutResult(cmd, result, logoutHeadline(scope, localOnly))
}

func logoutHeadline(scope constants.LogoutScope, localOnly bool) string {
	if localOnly {
		return "Local CLI credentials removed. Server-side sessions, operator bindings, and certificates were not changed."
	}
	switch scope {
	case constants.LogoutScopeWeb:
		return "Logged out of all web sessions"
	case constants.LogoutScopeCLI:
		return "Logged out of all CLI sessions"
	default:
		return "Logged out of all web and CLI sessions"
	}
}

func writeLogoutResult(cmd *cobra.Command, result logoutOutput, headline string) error {
	if output.JSONEnabled(cmd) {
		return output.WriteJSON(cmd.OutOrStdout(), result)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, headline)
	if result.LocalOnly || !result.Success || result.UserID == "" {
		return nil
	}
	if result.Scope.IncludesWeb() {
		fmt.Fprintf(out, "  Web sessions ended:        %d\n", result.WebSessionsTerminated)
	}
	if result.Scope.IncludesCLI() {
		fmt.Fprintf(out, "  CLI sessions ended:        %d\n", result.CLISessionsTerminated)
		fmt.Fprintf(out, "  CLI certificates revoked:  %d\n", result.CLICertificatesRevoked)
	}
	fmt.Fprintf(out, "  Operators unbound:         %d\n", len(result.UnboundOperatorSessionIDs))
	if result.LocalCredentialsCleared {
		fmt.Fprintln(out, "  Local credentials:         cleared")
	}
	return nil
}

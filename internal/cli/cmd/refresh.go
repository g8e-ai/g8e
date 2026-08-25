// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// refreshClient is the interface satisfied by *auth.EnrollmentClient for
// the refresh operation. The command layer depends on this interface
// rather than the concrete type so tests can inject a mock that returns
// canned results without network I/O.
type refreshClient interface {
	Refresh(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionRefresh, error)
}

// refreshClientFactory builds an auth.EnrollmentClient for the refresh
// operation. It is injected through *WithConfig constructors (mirroring
// fileSvcFactory) so production wires the real client and tests wire a
// stub.
type refreshClientFactory func(cfg *config.Config) refreshClient

func defaultRefreshClientFactory(cfg *config.Config) refreshClient {
	return auth.NewEnrollmentClient(cfg, nil)
}

func refreshCmd() *cobra.Command {
	return refreshCmdWithConfig(loadConfig, newFileSvc, defaultRefreshClientFactory)
}

func refreshCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	clientFactory refreshClientFactory,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh an expired CLI session using the still-valid mTLS certificate",
		Long: `Refresh an expired CLI session by obtaining a new session from the gateway.

This command is the recovery path for an expired CLI session with a still-valid
mTLS certificate. The gateway derives the user identity from the verified
certificate and issues a new CLI session bound to the same user. The certificate
is NOT rotated — the cert is the proof of identity.

When the certificate itself has expired, this command cannot help (an expired
cert cannot authenticate via mTLS). Use 'auth enroll user --headless' instead to
initiate the human-approved CLI recovery flow, which issues a new certificate.

The gateway must already be running.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			// Load the current credentials to get the old session ID for
			// the deactivation side of the refresh. If credentials are
			// missing, proceed — the gateway handles a missing old
			// session by only persisting the new one.
			store := auth.NewCredentialStore(fileSvc, cfg)
			creds, err := store.LoadCredentials(ctx)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
			}
			if creds == nil {
				cmd.Println("No local CLI identity found. Use 'auth enroll user' to enroll first.")
				return nil
			}
			cmd.Printf("Refreshing CLI session for user %s (old session %s).\n", creds.UserID, creds.CLISessionID)

			client := clientFactory(cfg)
			refresh, err := client.Refresh(ctx, fileSvc)
			if err != nil {
				return fmt.Errorf("auth refresh: %w", err)
			}

			// Update the local credentials with the new session ID. The
			// cert, user ID, and operator binding are unchanged.
			creds.CLISessionID = refresh.CLISessionID
			if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
				return fmt.Errorf("auth refresh: save credentials: %w", err)
			}

			cmd.Printf("CLI session refreshed successfully.\n")
			cmd.Printf("User ID: %s\n", refresh.UserID)
			cmd.Printf("New CLI Session ID: %s\n", refresh.CLISessionID)
			return nil
		},
	}
	return cmd
}

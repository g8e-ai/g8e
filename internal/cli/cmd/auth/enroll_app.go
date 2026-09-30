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
	"io"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// AppEnroller is the interface satisfied by *auth.AppPlatformEnrollmentClient.
type AppEnroller interface {
	Enroll(ctx context.Context, out io.Writer) (*models.PlatformEnrollmentCompleteResponse, error)
}

// AppEnrollerFactory creates an AppEnroller instance.
type AppEnrollerFactory func(appName string, fileSvc fs.RuntimeFileService, cfg *config.Config, logger *slog.Logger) (AppEnroller, error)

// DefaultAppEnrollerFactory creates a production AppPlatformEnrollmentClient.
func DefaultAppEnrollerFactory(appName string, fileSvc fs.RuntimeFileService, cfg *config.Config, logger *slog.Logger) (AppEnroller, error) {
	return auth.NewAppPlatformEnrollmentClient(appName, fileSvc, cfg, logger)
}

func enrollAppCmd() *cobra.Command {
	return enrollAppCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, DefaultAppEnrollerFactory)
}

func enrollAppCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	enrollerFactory AppEnrollerFactory,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app <name>",
		Short: "Enroll an application workload with the running Gateway via owner-approved platform enrollment",
		Long: `Enroll an application workload with the running Gateway via owner-approved platform enrollment.

Submits a platform enrollment request for the specified application name, prints the approval command,
and polls until an owner approves the request (via 'g8e auth enroll approve <request-id>'). Upon approval,
the certificate and private key are securely stored in the managed runtime PKI directory.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := strings.TrimSpace(args[0])

			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			enroller, err := enrollerFactory(appName, fileSvc, cfg, slog.Default())
			if err != nil {
				return err
			}

			_, err = enroller.Enroll(cmd.Context(), cmd.OutOrStdout())
			return err
		},
	}

	return cmd
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type publicRestoreRunJSON struct {
	RunID            string `json:"run_id"`
	PublishedRecords int    `json:"published_records"`
	Force            bool   `json:"force,omitempty"`
}

func PublicRestoreCmdWithConfig(configLoader func(string) (*config.Config, error), fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var queue bool
	var runID string
	var force bool
	var runTimeout time.Duration
	var projectRoot string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore missing campaign datasets to the public mirror",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !queue && runID == "" {
				return fmt.Errorf("public restore: specify --queue or --run-id")
			}
			if queue && runID != "" {
				return fmt.Errorf("public restore: --queue and --run-id are mutually exclusive")
			}
			cfg, err := configLoader(projectRoot)
			if err != nil {
				return fmt.Errorf("public restore: load config: %w", err)
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("public restore: %w", err)
			}
			if err := fileSvc.CreateRuntimeTree(shared.CommandContext(cmd)); err != nil {
				return fmt.Errorf("public restore: create runtime tree: %w", err)
			}
			publication, err := NewCampaignPublicationCoordinator(cmd, fileSvc)
			if err != nil {
				return fmt.Errorf("public restore: %w", err)
			}
			mirrorProbe := gwremote.NewHTTPCampaignMirrorProbe(cmd.Context())
			publication.WithMirrorProbe(mirrorProbe)
			reconciler := evaluation.NewCampaignMirrorReconciler(
				publication,
				evaluation.NewStore(fileSvc),
				mirrorProbe,
			)
			if queue {
				result, err := ReconcileVerifiedCampaignMirrorQueue(cmd.Context(), cfg.ProjectRoot, reconciler, runTimeout, force, func(progress evaluation.CampaignMirrorReconcileProgress) {
					WriteCampaignMirrorRestoreProgress(cmd.ErrOrStderr(), progress)
				})
				if err != nil {
					return fmt.Errorf("public restore: %w", err)
				}
				if output.JSONEnabled(cmd) {
					payload, err := json.MarshalIndent(result, "", "  ")
					if err != nil {
						return err
					}
					if _, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload)); err != nil {
						return err
					}
				} else {
					WriteCampaignMirrorRestoreQueueResult(cmd.OutOrStdout(), cmd.ErrOrStderr(), result, force)
				}
				if len(result.FailedRuns) > 0 {
					return fmt.Errorf("public restore: %d run(s) failed", len(result.FailedRuns))
				}
				return nil
			}
			published, err := reconciler.ReconcileRun(cmd.Context(), runID, force)
			if err != nil {
				return fmt.Errorf("public restore: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(publicRestoreRunJSON{
					RunID:            runID,
					PublishedRecords: published,
					Force:            force,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			WriteCampaignMirrorRestoreRunResult(cmd.OutOrStdout(), runID, published, force)
			return nil
		},
	}
	cmd.Flags().BoolVar(&queue, "queue", false, "Restore every verified run listed in .g8e/eval/init-campaign-queue.json")
	cmd.Flags().BoolVar(&force, "force", false, "Clear host publication idempotency before restoring missing datasets; with --run-id, also republish when already present")
	cmd.Flags().StringVar(&runID, "run-id", "", "Restore one canonical campaign run")
	cmd.Flags().DurationVar(&runTimeout, "run-timeout", evaluation.CampaignMirrorDefaultRunTimeout, "Base per-run timeout; scales up with assignment count during restore")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Override the repository root (defaults to cwd)")
	return cmd
}

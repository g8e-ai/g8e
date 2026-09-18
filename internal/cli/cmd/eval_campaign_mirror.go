// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func campaignEvalMirrorCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Reconcile campaign datasets with the gateway-owned public mirror",
	}
	cmd.AddCommand(campaignEvalMirrorRestoreCmd(deps))
	return cmd
}

func campaignEvalMirrorRestoreCmd(deps nativeEvalDeps) *cobra.Command {
	var queue bool
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore missing campaign datasets to the public mirror",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !queue && runID == "" {
				return fmt.Errorf("evaluation: campaign mirror restore: specify --queue or --run-id")
			}
			if queue && runID != "" {
				return fmt.Errorf("evaluation: campaign mirror restore: --queue and --run-id are mutually exclusive")
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: campaign mirror restore: %w", err)
			}
			reconciler := evaluation.NewCampaignMirrorReconciler(
				publication,
				evaluation.NewStore(fileSvc),
				newHTTPCampaignMirrorProbe(cmd.Context()),
			)
			if queue {
				result, err := reconcileVerifiedCampaignMirrorQueue(cmd.Context(), cfg.ProjectRoot, reconciler)
				if err != nil {
					return fmt.Errorf("evaluation: campaign mirror restore: %w", err)
				}
				if jsonOutput {
					payload, err := json.MarshalIndent(result, "", "  ")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Restored %d verified dataset(s) to public mirror (%d record(s))\n", len(result.RestoredRunIDs), result.PublishedRecords)
				if len(result.SkippedRunIDs) > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skipped %d run(s) already present in mirror\n", len(result.SkippedRunIDs))
				}
				if len(result.HostAbsentRunIDs) > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Host canonical store missing for %d verified run(s) after .g8e wipe — re-execute to repopulate: %v\n", len(result.HostAbsentRunIDs), result.HostAbsentRunIDs)
				}
				for runID, reason := range result.FailedRuns {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: mirror restore failed for %s: %s\n", runID, reason)
				}
				return err
			}
			published, err := reconciler.ReconcileRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign mirror restore: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":            runID,
					"published_records": published,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if published == 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run %s already present in public mirror\n", runID)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Restored run %s to public mirror (%d record(s))\n", runID, published)
			return err
		},
	}
	cmd.Flags().BoolVar(&queue, "queue", false, "Restore every verified run listed in .local.dev/init-campaign-queue.json")
	cmd.Flags().StringVar(&runID, "run-id", "", "Restore one canonical campaign run")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func reconcileVerifiedCampaignMirrorQueue(ctx context.Context, projectRoot string, reconciler *evaluation.CampaignMirrorReconciler) (*evaluation.CampaignMirrorReconcileResult, error) {
	queuePath := filepath.Join(projectRoot, evaluation.DefaultInitCampaignQueueRelPath)
	queue, err := evaluation.LoadInitCampaignQueue(queuePath)
	if err != nil {
		return nil, err
	}
	return reconciler.ReconcileVerifiedQueue(ctx, queue)
}

func reconcileVerifiedCampaignMirrorFromDockerInit(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignMirrorReconcileResult, error) {
	if !isGatewayHealthy() {
		return nil, fmt.Errorf("gateway is not healthy")
	}
	publication, err := newCampaignPublicationCoordinatorFromConfig(ctx, fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	reconciler := evaluation.NewCampaignMirrorReconciler(
		publication,
		evaluation.NewStore(fileSvc),
		newHTTPCampaignMirrorProbe(ctx),
	)
	return reconcileVerifiedCampaignMirrorQueue(ctx, cfg.ProjectRoot, reconciler)
}

func newCampaignPublicationCoordinatorFromConfig(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignPublicationCoordinator, error) {
	exporter, err := newCampaignFeedExporter(ctx, fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: %w", err)
	}
	remote, err := newProviderObservationRemote(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: provider observation remote: %w", err)
	}
	publicationState, err := newGatewayCampaignPublicationStateStoreFromConfig(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	return evaluation.NewCampaignPublicationCoordinator(
		evaluation.NewStore(fileSvc),
		fileSvc,
		publicationState,
		exporter,
		remote,
	).WithMirrorProbe(newHTTPCampaignMirrorProbe(ctx)), nil
}

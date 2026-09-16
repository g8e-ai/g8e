// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func campaignEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Initialize, schedule, and resume North Star model campaigns",
	}
	cmd.AddCommand(
		campaignEvalInitCmd(deps),
		campaignEvalScheduleCmd(deps),
		campaignEvalStatusCmd(deps),
	)
	return cmd
}

func campaignEvalInitCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignID string
	var runID string
	var inventoryFile string
	var inferenceSessionID string
	var dataSessionID string
	var repetitionCount uint32
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a North Star campaign run with frozen catalog and model registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" || runID == "" {
				return fmt.Errorf("evaluation: campaign init: %w", constants.ErrMissingRequiredField)
			}
			if inventoryFile == "" {
				return fmt.Errorf("evaluation: campaign init: --inventory-file is required")
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: load CLI identity: %w", err)
			}
			inventory, err := evaluation.LoadModelInventoryFreezeFile(inventoryFile)
			if err != nil {
				return err
			}
			if inventory.CampaignID != "" && inventory.CampaignID != campaignID {
				return fmt.Errorf("evaluation: campaign init: inventory campaign_id mismatch")
			}
			inventory.CampaignID = campaignID
			catalog, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: %w", err)
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			run, err := controller.InitializeCampaign(cmd.Context(), evaluation.CampaignInitRequest{
				CampaignID:                 campaignID,
				RunID:                      runID,
				Catalog:                    catalog,
				Inventory:                  inventory,
				ScenarioArtifacts:          artifacts,
				RepetitionCount:            repetitionCount,
				InferenceOperatorSessionID: inferenceSessionID,
				DataOperatorSessionID:      dataSessionID,
				Deployment:                 nativeEvalDeployment(cfg, authContext, runID, ""),
			})
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":           campaignID,
					"run_id":                runID,
					"catalog_digest":        catalog.GetCatalogDigest(),
					"model_registry_digest": inventory.RegistryDigest,
					"model_count":           len(inventory.Variants),
					"scenario_count":        len(catalog.GetScenarios()),
					"project_root":          cfg.ProjectRoot,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Campaign initialized\nCampaign: %s\nRun: %s\nCatalog digest: %s\nModel registry digest: %s\nModels: %d\nScenarios: %d\nStarted: %s\n",
				campaignID,
				run.GetRunId(),
				catalog.GetCatalogDigest(),
				inventory.RegistryDigest,
				len(inventory.Variants),
				len(catalog.GetScenarios()),
				run.GetStartedAt().AsTime().Format("2006-01-02T15:04:05Z"),
			)
			return err
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().StringVar(&inventoryFile, "inventory-file", "", "Phase 3 inventory freeze JSON path")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Exact inference Operator session ID")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Exact data Operator session ID")
	cmd.Flags().Uint32Var(&repetitionCount, "repetition-count", 1, "Homogeneous repetition count")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalScheduleCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Materialize and persist the homogeneous assignment matrix for one run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign schedule: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			count, err := controller.ScheduleHomogeneousRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign schedule: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"run_id": runID, "assignment_count": count}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d homogeneous assignments for run %s\n", count, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalStatusCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report resumable campaign run status from canonical assignment records",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign status: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			summary, err := controller.RunSummary(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign status: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":               runID,
					"campaign_id":          summary.Run.GetCampaignBinding().GetCampaignId(),
					"expected_assignments": summary.ExpectedAssignment,
					"queued":               summary.QueuedCount,
					"running":              summary.RunningCount,
					"terminal":             summary.TerminalCount,
					"next_assignment_id":   summary.NextAssignmentID,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nCampaign: %s\nExpected assignments: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nNext assignment: %s\n",
				runID,
				summary.Run.GetCampaignBinding().GetCampaignId(),
				summary.ExpectedAssignment,
				summary.QueuedCount,
				summary.RunningCount,
				summary.TerminalCount,
				summary.NextAssignmentID,
			)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

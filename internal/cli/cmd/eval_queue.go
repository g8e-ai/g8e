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
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func rolloutEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Manage the init-campaign rollout queue",
	}
	cmd.AddCommand(
		rolloutEvalInitCmd(deps),
		rolloutEvalRunCmd(deps),
		rolloutEvalMarkCmd(deps),
		rolloutEvalListCmd(deps),
		rolloutEvalNextCmd(deps),
	)
	return cmd
}

func rolloutEvalInitCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	var outputPath string
	var inventoryDir string
	var tags string
	var materialize bool
	var mergeExisting bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Build the init-campaign rollout queue from a frozen inventory",
		Long: `Initialize .g8e/eval/init-campaign-queue.json from a provider inventory freeze.

Examples:
  g8e eval rollout init --materialize --merge
  g8e eval rollout init --from .g8e/eval/model-inventory.json --tags qwen3:4b,gemma3:4b
  g8e eval rollout init --materialize --merge`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			runtimeInventoryPath, externalInventoryPath, err := resolveEvaluationInventorySource(fromPath, cfg.ProjectRoot)
			if err != nil {
				return err
			}
			result, err := evaluation.InitCampaignQueue(evaluation.InitCampaignQueueRequest{
				Context:                     cmd.Context(),
				FileService:                 fileSvc,
				RuntimeInventoryPath:        runtimeInventoryPath,
				ExternalSourceInventoryPath: externalInventoryPath,
				InventoryRelDir:             normalizeRuntimeEvalPath(inventoryDir),
				OutputQueuePath:             normalizeRuntimeEvalPath(outputPath),
				Tags:                        splitCSVModelTags(tags),
				Materialize:                 materialize,
				MergeExisting:               mergeExisting,
			})
			if err != nil {
				return fmt.Errorf("evaluation: queue init: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (%d models", result.QueuePath, result.ModelCount)
			if err != nil {
				return err
			}
			if result.Materialized > 0 {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), ", %d inventories materialized", result.Materialized); err != nil {
					return err
				}
			}
			if result.Preserved > 0 {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), ", %d verified entries preserved", result.Preserved); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), ")")
			return err
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "Source inventory JSON path (default: eval/base-model-inventory.json; runtime freeze filters to base tags when passed explicitly)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	cmd.Flags().StringVar(&inventoryDir, "inventory-dir", "", "Per-model inventory directory (default: .g8e/eval/inventories)")
	cmd.Flags().StringVar(&tags, "tags", "", "Include only these served model tags (comma-separated)")
	cmd.Flags().BoolVar(&materialize, "materialize", false, "Write per-model inventory files before building the queue")
	cmd.Flags().BoolVar(&mergeExisting, "merge", false, "Preserve verified status from an existing queue file")
	return cmd
}

func rolloutEvalMarkCmd(deps nativeEvalDeps) *cobra.Command {
	var queuePath string
	var variantID string
	var servedModelTag string
	var status string
	var verifiedRunID string
	var notes string
	cmd := &cobra.Command{
		Use:   "mark",
		Short: "Update one init-campaign queue entry",
		Long: `Record verification progress for one queue entry.

Example:
  g8e eval rollout mark --tag qwen3:4b --status verified --run-id eval-init-qwen3-4b-1789657337`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			entry, err := evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
				Context:        cmd.Context(),
				FileService:    fileSvc,
				QueuePath:      normalizeRuntimeEvalPath(queuePath),
				VariantID:      variantID,
				ServedModelTag: servedModelTag,
				Status:         status,
				VerifiedRunID:  verifiedRunID,
				Notes:          notes,
			})
			if err != nil {
				return fmt.Errorf("evaluation: queue mark: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(entry, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated %s (%s) status=%s run_id=%s\n",
				entry.ServedModelTag, entry.VariantID, entry.Status, entry.VerifiedRunID)
			return err
		},
	}
	cmd.Flags().StringVar(&queuePath, "queue-file", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	cmd.Flags().StringVar(&variantID, "variant-id", "", "Variant ID to update")
	cmd.Flags().StringVar(&servedModelTag, "tag", "", "Served model tag to update")
	cmd.Flags().StringVar(&status, "status", "verified", "Queue status to set")
	cmd.Flags().StringVar(&verifiedRunID, "run-id", "", "Verified campaign run ID")
	cmd.Flags().StringVar(&notes, "notes", "", "Optional operator notes")
	return cmd
}

func normalizeRuntimeEvalPath(rawPath string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rawPath)), constants.RuntimeDirname+"/")
}

func loadInitCampaignQueue(ctx context.Context, fileSvc fs.RuntimeFileService, queueFile string) (*evaluation.CampaignQueue, string, error) {
	queuePath := queueFile
	if queuePath == "" {
		queuePath = evaluation.DefaultInitCampaignQueueRelPath
	}
	queuePath = normalizeRuntimeEvalPath(queuePath)
	if filepath.IsAbs(queuePath) || queuePath == ".." || strings.HasPrefix(queuePath, "../") {
		return nil, queueFile, fmt.Errorf("evaluation: queue path must be runtime-relative")
	}
	queue, err := evaluation.LoadInitCampaignQueueFromRuntime(ctx, fileSvc, queuePath)
	if err != nil {
		return nil, queuePath, err
	}
	return queue, queuePath, nil
}

type campaignQueueListJSON struct {
	Models []evaluation.CampaignQueueModel `json:"models"`
}

func rolloutEvalListCmd(deps nativeEvalDeps) *cobra.Command {
	var status string
	var queueFile string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List init-campaign queue entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, _, err := loadInitCampaignQueue(cmd.Context(), fileSvc, queueFile)
			if err != nil {
				return fmt.Errorf("evaluation: queue list: %w", err)
			}
			entries := queue.FilterByStatus(status)
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignQueueListJSON{Models: entries}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if len(entries) == 0 {
				cmd.Println("No queue entries found")
				return nil
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Init campaign queue (%d entries)\n", len(entries))
			if err != nil {
				return err
			}
			for _, entry := range entries {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "- %s  campaign=%s  status=%s  cells=%d\n",
					entry.ServedModelTag,
					entry.CampaignID,
					entry.Status,
					entry.HomogeneousCellCount,
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "all", "Filter by status: all, pending, verified, completed")
	cmd.Flags().StringVar(&queueFile, "queue-file", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	return cmd
}

func rolloutEvalNextCmd(deps nativeEvalDeps) *cobra.Command {
	var queueFile string
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next pending init-campaign queue entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, _, err := loadInitCampaignQueue(cmd.Context(), fileSvc, queueFile)
			if err != nil {
				return fmt.Errorf("evaluation: queue next: %w", err)
			}
			entry, err := queue.NextPending()
			if err != nil {
				return fmt.Errorf("evaluation: queue next: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(entry, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Next pending model\nTag: %s\nVariant: %s\nCampaign: %s\nInventory: %s\nRegistry digest: %s\nCells: %d\n\nRecommended flow:\n  ./g8e eval campaign start --queue next --publish --daemon --require-witness\n  ./g8e eval rollout run --require-witness --skip-verified\n",
				entry.ServedModelTag,
				entry.VariantID,
				entry.CampaignID,
				entry.InventoryFile,
				entry.ModelRegistryDigest,
				entry.HomogeneousCellCount,
			)
			return err
		},
	}
	cmd.Flags().StringVar(&queueFile, "queue-file", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	return cmd
}

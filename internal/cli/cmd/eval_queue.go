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
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func queueEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Inspect the init-campaign rollout queue",
	}
	cmd.AddCommand(
		queueEvalListCmd(deps),
		queueEvalNextCmd(deps),
	)
	return cmd
}

func queueEvalListCmd(deps nativeEvalDeps) *cobra.Command {
	var status string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List init-campaign queue entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, err := evaluation.LoadInitCampaignQueue(filepath.Join(cfg.ProjectRoot, evaluation.DefaultInitCampaignQueueRelPath))
			if err != nil {
				return fmt.Errorf("evaluation: queue list: %w", err)
			}
			entries := queue.FilterByStatus(status)
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"models": entries}, "", "  ")
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func queueEvalNextCmd(deps nativeEvalDeps) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next pending init-campaign queue entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, err := evaluation.LoadInitCampaignQueue(filepath.Join(cfg.ProjectRoot, evaluation.DefaultInitCampaignQueueRelPath))
			if err != nil {
				return fmt.Errorf("evaluation: queue next: %w", err)
			}
			entry, err := queue.NextPending()
			if err != nil {
				return fmt.Errorf("evaluation: queue next: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(entry, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Next pending model\nTag: %s\nVariant: %s\nCampaign: %s\nInventory: %s\nRegistry digest: %s\nCells: %d\n\nRecommended flow:\n  ./g8e eval campaign start --queue next --publish --daemon --verify --require-provider-observation\n",
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

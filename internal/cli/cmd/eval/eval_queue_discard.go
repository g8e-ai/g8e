// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func rolloutEvalDiscardCmd(deps nativeEvalDeps) *cobra.Command {
	var queueFile string
	var runID string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "discard",
		Short: "Remove abandoned rollout runs that never made execution progress",
		Long: `Discard init-campaign runs that were scheduled but never started executing.

This removes host evidence under .g8e/data/eval/runs/, clears failed queue
references back to pending, deletes gateway publication idempotency, and hides
the run from the public mirror. Verified queue entries are never discarded.

Examples:
  g8e eval rollout discard --dry-run
  g8e eval rollout discard
  g8e eval rollout discard --run-id eval-init-glm-5-3-air-1790258676`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, queuePath, err := loadInitCampaignQueue(cmd.Context(), fileSvc, queueFile)
			if err != nil {
				return fmt.Errorf("evaluation: rollout discard: %w", err)
			}
			store := evaluation.NewStore(fileSvc)
			controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			abandoned, err := evaluation.ListAbandonedCampaignRuns(evaluation.ListAbandonedCampaignRunsRequest{
				Context:      cmd.Context(),
				Store:        store,
				Controller:   controller,
				Queue:        queue,
				IncludeRunID: runID,
			})
			if err != nil {
				return fmt.Errorf("evaluation: rollout discard: %w", err)
			}
			if len(abandoned) == 0 {
				if output.JSONEnabled(cmd) {
					payload, err := json.MarshalIndent(evaluation.DiscardAbandonedCampaignRunsResult{}, "", "  ")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return err
				}
				cmd.Println("No abandoned rollout runs to discard")
				return nil
			}
			if dryRun {
				if output.JSONEnabled(cmd) {
					payload, err := json.MarshalIndent(abandoned, "", "  ")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Would discard %d abandoned rollout run(s)\n", len(abandoned))
				if err != nil {
					return err
				}
				for _, run := range abandoned {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "- %s  scheduled=%d  queued=%d  queue=%s\n",
						run.RunID, run.Scheduled, run.Queued, run.QueueStatus)
					if err != nil {
						return err
					}
				}
				return nil
			}
			publicationState, err := newGatewayCampaignPublicationStateStoreFromConfig(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: rollout discard: %w", err)
			}
			withdrawMirror := func(ctx context.Context, discardedRunID string) error {
				return publicationState.Delete(ctx, discardedRunID)
			}
			result, err := evaluation.DiscardAbandonedCampaignRuns(evaluation.DiscardAbandonedCampaignRunsRequest{
				Context:        cmd.Context(),
				FileService:    fileSvc,
				Store:          store,
				Queue:          queue,
				QueuePath:      queuePath,
				Runs:           abandoned,
				WithdrawMirror: withdrawMirror,
			})
			if err != nil {
				return fmt.Errorf("evaluation: rollout discard: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Discarded %d abandoned rollout run(s)\n", len(result.DiscardedRunIDs))
			if err != nil {
				return err
			}
			for _, discardedRunID := range result.DiscardedRunIDs {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", discardedRunID)
				if err != nil {
					return err
				}
			}
			if result.QueueUpdates > 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reset %d queue entr(y/ies) to pending\n", result.QueueUpdates)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&queueFile, "queue-file", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Discard one specific abandoned run")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "List abandoned runs without deleting anything")
	return cmd
}

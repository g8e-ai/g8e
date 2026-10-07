// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func rolloutEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Qualify registry models one at a time through the rollout queue",
		Long: `The rollout queue orders registry models for unattended qualification. Each
entry is created as a campaign, run with strict witness verification, and marked
verified only when a full run passes.

  add      queue registry models
  remove   drop models from the queue
  retry    return models to pending
  skip     exclude models from rollout runs
  list     show the queue
  next     show the next pending model
  run      qualify pending models`,
	}
	addLeaves(cmd,
		rolloutListCmd(deps),
		rolloutAddCmd(deps),
		rolloutRemoveCmd(deps),
		rolloutNextCmd(deps),
		rolloutRetryCmd(deps),
		rolloutSkipCmd(deps),
		rolloutRunCmd(deps),
	)
	return cmd
}

type rolloutListJSON struct {
	Models []evaluation.CampaignQueueModel `json:"models"`
}

type rolloutChangeJSON struct {
	Action    string                          `json:"action"`
	Models    []evaluation.CampaignQueueModel `json:"models"`
	Remaining int                             `json:"remaining"`
}

func rolloutListCmd(deps nativeEvalDeps) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List rollout queue entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, err := evaluation.LoadRolloutQueue(cmd.Context(), fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: rollout list: %w", err)
			}
			entries := queue.FilterByStatus(status)
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), rolloutListJSON{Models: entries})
			}
			if len(entries) == 0 {
				cmd.Println("No queue entries found")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "MODEL\tCAMPAIGN\tSTATUS\tCELLS\tRUN")
			for _, entry := range entries {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", entry.ServedModelTag, entry.CampaignID, entry.Status, entry.HomogeneousCellCount, entry.VerifiedRunID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&status, "status", "all", "Filter by status: all, pending, verified, failed, skipped")
	return cmd
}

func rolloutAddCmd(deps nativeEvalDeps) *cobra.Command {
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   "add <selector>",
		Short: "Queue registry models for rollout",
		Long: `Queue the selected registry models. Models already queued keep their place and
status. New models are ordered by the checked-in intake priority
(eval/rollout-intake-priority.json), then smallest first.

Examples:
  g8e eval rollout add qwen3:0.6b
  g8e eval rollout add --max-params 4b
  g8e eval rollout add --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			registry, err := newModelInventories(fileSvc, cfg.ProjectRoot).variants(cmd.Context(), modelScopeRegistry)
			if err != nil {
				return fmt.Errorf("evaluation: rollout add: %w", err)
			}
			if len(registry) == 0 {
				return fmt.Errorf("evaluation: rollout add: the model registry is empty (run `g8e eval models freeze` or `g8e eval models import`): %w", constants.ErrEvaluationSelectionEmpty)
			}
			selected, err := selector.withArgs(args).Resolve(registry)
			if err != nil {
				return fmt.Errorf("evaluation: rollout add: %w", err)
			}
			queue, err := evaluation.LoadRolloutQueue(cmd.Context(), fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: rollout add: %w", err)
			}
			priority, err := evaluation.LoadRolloutIntakePriorityIDs(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("evaluation: rollout add: %w", err)
			}
			added, err := queue.AddVariants(selected, priority)
			if err != nil {
				return fmt.Errorf("evaluation: rollout add: %w", err)
			}
			if len(added) > 0 {
				if err := evaluation.SaveRolloutQueue(cmd.Context(), fileSvc, queue); err != nil {
					return fmt.Errorf("evaluation: rollout add: %w", err)
				}
			}
			note := ""
			if already := len(selected) - len(added); already > 0 {
				note = fmt.Sprintf("%d selected model(s) were already queued", already)
			}
			return writeRolloutChange(cmd, "add", added, len(queue.Models), func(entry evaluation.CampaignQueueModel) string {
				return fmt.Sprintf("Queued %s (%s)", entry.ServedModelTag, entry.VariantID)
			}, note)
		},
	}
	selector.bindFlags(cmd)
	return cmd
}

func rolloutRemoveCmd(deps nativeEvalDeps) *cobra.Command {
	return rolloutSelectorCmd(deps, rolloutSelectorVerb{
		use:   "remove <selector>",
		name:  "remove",
		short: "Drop models from the rollout queue",
		long: `Drop the selected models from the queue. Their campaigns and runs are not
touched; use g8e eval campaigns archive for those.`,
		line: func(entry evaluation.CampaignQueueModel) string {
			return fmt.Sprintf("Removed %s (%s)", entry.ServedModelTag, entry.VariantID)
		},
		apply: func(queue *evaluation.CampaignQueue, entries []evaluation.CampaignQueueModel) {
			queue.RemoveEntries(variantIDsOf(entries))
		},
	})
}

func rolloutRetryCmd(deps nativeEvalDeps) *cobra.Command {
	return rolloutSelectorCmd(deps, rolloutSelectorVerb{
		use:   "retry <selector>",
		name:  "retry",
		short: "Return models to pending so the next rollout run picks them up",
		long: `Set the selected models back to pending, whatever their status. A verified model
that is retried qualifies again on the next rollout run.`,
		line: func(entry evaluation.CampaignQueueModel) string {
			return fmt.Sprintf("Pending %s (%s)", entry.ServedModelTag, entry.VariantID)
		},
		apply: func(queue *evaluation.CampaignQueue, entries []evaluation.CampaignQueueModel) {
			queue.SetEntryStatus(variantIDsOf(entries), evaluation.QueueStatusPending, "")
		},
	})
}

func rolloutSkipCmd(deps nativeEvalDeps) *cobra.Command {
	return rolloutSelectorCmd(deps, rolloutSelectorVerb{
		use:   "skip <selector>",
		name:  "skip",
		short: "Exclude models from rollout runs without removing them",
		long: `Mark the selected models skipped. Rollout runs never execute skipped models;
retry returns them to pending.`,
		line: func(entry evaluation.CampaignQueueModel) string {
			return fmt.Sprintf("Skipped %s (%s)", entry.ServedModelTag, entry.VariantID)
		},
		apply: func(queue *evaluation.CampaignQueue, entries []evaluation.CampaignQueueModel) {
			queue.SetEntryStatus(variantIDsOf(entries), evaluation.QueueStatusSkipped, "skipped by operator")
		},
	})
}

// rolloutSelectorVerb describes one verb that edits the queue entries a model
// selector picks out.
type rolloutSelectorVerb struct {
	use   string
	name  string
	short string
	long  string
	line  func(evaluation.CampaignQueueModel) string
	apply func(*evaluation.CampaignQueue, []evaluation.CampaignQueueModel)
}

func rolloutSelectorCmd(deps nativeEvalDeps, verb rolloutSelectorVerb) *cobra.Command {
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   verb.use,
		Short: verb.short,
		Long:  verb.long,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, err := evaluation.LoadRolloutQueue(cmd.Context(), fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: rollout %s: %w", verb.name, err)
			}
			entries, err := selectQueueEntries(cmd.Context(), fileSvc, cfg.ProjectRoot, queue, selector.withArgs(args))
			if err != nil {
				return fmt.Errorf("evaluation: rollout %s: %w", verb.name, err)
			}
			verb.apply(queue, entries)
			if err := evaluation.SaveRolloutQueue(cmd.Context(), fileSvc, queue); err != nil {
				return fmt.Errorf("evaluation: rollout %s: %w", verb.name, err)
			}
			return writeRolloutChange(cmd, verb.name, entries, len(queue.Models), verb.line, "")
		},
	}
	selector.bindFlags(cmd)
	return cmd
}

// selectQueueEntries applies a selector to the queue. Positional models name
// queue entries directly, so an entry whose model has left the registry can
// still be removed. Filter flags resolve against the registry.
func selectQueueEntries(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot string, queue *evaluation.CampaignQueue, selector *ModelSelector) ([]evaluation.CampaignQueueModel, error) {
	if err := selector.validate(); err != nil {
		return nil, err
	}
	if !selector.IsSet() {
		return nil, fmt.Errorf("name models or pass --family, --max-params, or --all: %w", constants.ErrEvaluationSelectionEmpty)
	}
	if len(selector.IDs) > 0 {
		return queue.SelectEntries(selector.IDs)
	}
	registry, err := newModelInventories(fileSvc, projectRoot).variants(ctx, modelScopeRegistry)
	if err != nil {
		return nil, err
	}
	resolved, err := selector.Resolve(registry)
	if err != nil {
		return nil, err
	}
	entries := queue.EntriesForVariants(resolved)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no queued models matched: %w", constants.ErrEvaluationSelectionEmpty)
	}
	return entries, nil
}

func variantIDsOf(entries []evaluation.CampaignQueueModel) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.VariantID)
	}
	return ids
}

func writeRolloutChange(cmd *cobra.Command, action string, entries []evaluation.CampaignQueueModel, remaining int, line func(evaluation.CampaignQueueModel) string, note string) error {
	if entries == nil {
		entries = []evaluation.CampaignQueueModel{}
	}
	if output.JSONEnabled(cmd) {
		return output.WriteJSON(cmd.OutOrStdout(), rolloutChangeJSON{Action: action, Models: entries, Remaining: remaining})
	}
	out := cmd.OutOrStdout()
	for _, entry := range entries {
		_, _ = fmt.Fprintln(out, line(entry))
	}
	if note != "" {
		_, _ = fmt.Fprintln(out, note)
	}
	_, err := fmt.Fprintf(out, "%d model(s) in the rollout queue\n", remaining)
	return err
}

func rolloutNextCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Show the next pending rollout model",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, err := evaluation.LoadRolloutQueue(cmd.Context(), fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: rollout next: %w", err)
			}
			entry, err := queue.NextPending()
			if err != nil {
				return fmt.Errorf("evaluation: rollout next: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), entry)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Next pending model\nTag: %s\nVariant: %s\nCampaign: %s\nRegistry digest: %s\nCells: %d\n\nQualify it with:\n  g8e eval rollout run --until 1\n",
				entry.ServedModelTag, entry.VariantID, entry.CampaignID, entry.ModelRegistryDigest, entry.HomogeneousCellCount)
			return err
		},
	}
}

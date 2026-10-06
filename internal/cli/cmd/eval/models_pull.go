// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// intakeVariants presents pull metadata as model variants so the shared
// selector can choose among them.
func intakeVariants(models []evaluation.RolloutIntakeModel) []*evalv1.ModelVariant {
	variants := make([]*evalv1.ModelVariant, 0, len(models))
	for _, model := range models {
		parameterCount, err := evaluation.ParseParameterCount(model.ParameterCount)
		if err != nil {
			parameterCount = 0
		}
		variants = append(variants, &evalv1.ModelVariant{
			VariantId:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			ModelFamily:    model.ModelFamily,
			ParameterCount: parameterCount,
		})
	}
	return variants
}

func modelsPullCmd(deps nativeEvalDeps) *cobra.Command {
	var timeout time.Duration
	var dryRun bool
	var formations bool
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   "pull <selector>",
		Short: "Pull models onto the inference provider through the governed Inference Operator",
		Long: `Pull models through the governed Inference Operator and apply their canonical aliases.

Pull metadata comes from eval/rollout-intake-hf.json. With --formations the
sovereign ExecutionTopologies served tags are pulled instead.

Examples:
  g8e eval models pull qwen3:4b
  g8e eval models pull --family glm53 --dry-run
  g8e eval models pull --formations --timeout 24h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			selector = *selector.withArgs(args)
			if formations && selector.IsSet() {
				return fmt.Errorf("evaluation: models pull: --formations cannot be combined with a model selector: %w", constants.ErrEvaluationFlagsInvalid)
			}
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			catalogPath := filepath.Join(cfg.ProjectRoot, evaluation.DefaultRolloutIntakeCatalogRelPath)
			var variantIDs []string
			if !formations {
				catalog, err := evaluation.LoadRolloutIntakeCatalog(catalogPath)
				if err != nil {
					return fmt.Errorf("evaluation: models pull: %w", err)
				}
				selected, err := selector.Resolve(intakeVariants(catalog.Models))
				if err != nil {
					return fmt.Errorf("evaluation: models pull: %w", err)
				}
				for _, variant := range selected {
					variantIDs = append(variantIDs, variant.GetVariantId())
				}
			}
			env, err := resolveGovernedModelMaintenance(cmd, deps)
			if err != nil {
				return fmt.Errorf("evaluation: models pull: %w", err)
			}
			env.Maintenance.Timeout = timeout
			req := evaluation.RolloutIntakeStageRequest{
				Context:            cmd.Context(),
				CatalogPath:        catalogPath,
				PullTimeout:        timeout,
				VariantIDs:         variantIDs,
				DryRun:             dryRun,
				Dispatcher:         env.ModelDispatcher,
				InferenceSessionID: env.Maintenance.TargetOperatorSessionID,
				Environment:        env.Maintenance.Environment,
				NewID:              env.Maintenance.NewID,
				CaseID:             "eval-models-pull",
				Progress: func(event evaluation.RolloutIntakeStageEvent) {
					if output.JSONEnabled(cmd) {
						return
					}
					target := event.ServedModelTag
					if target == "" {
						target = event.VariantID
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s", target, event.Action, event.Status)
					if strings.TrimSpace(event.Detail) != "" && event.Action != "pull" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (%s)", event.Detail)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				},
			}
			var result *evaluation.RolloutIntakeStageResult
			if formations {
				result, err = evaluation.StageFormationCatalogIntake(req)
			} else {
				result, err = evaluation.StageRolloutIntake(req)
			}
			if err != nil {
				return fmt.Errorf("evaluation: models pull: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), result)
			}
			out := cmd.OutOrStdout()
			for _, tag := range result.Pulled {
				_, _ = fmt.Fprintf(out, "Pulled %s\n", tag)
			}
			for _, tag := range result.Aliased {
				_, _ = fmt.Fprintf(out, "Aliased %s\n", tag)
			}
			for _, skip := range result.Skipped {
				_, _ = fmt.Fprintf(out, "Skipped %s: %s\n", skip.ServedModelTag, skip.Reason)
			}
			for _, failure := range result.Failed {
				_, _ = fmt.Fprintf(out, "Failed %s: %s\n", failure.ServedModelTag, failure.Err)
			}
			if len(result.Failed) > 0 {
				return fmt.Errorf("evaluation: models pull: %d model(s) failed", len(result.Failed))
			}
			_, err = fmt.Fprintln(out, "\nNext: `g8e eval models freeze` to bind the registry to the pulled digests.")
			return err
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", evaluation.DefaultRolloutIntakePullTimeout(), "Maximum time to wait for each pull")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the pull plan without pulling")
	cmd.Flags().BoolVar(&formations, "formations", false, "Pull the sovereign ExecutionTopologies served tags")
	selector.bindFlags(cmd)
	return cmd
}

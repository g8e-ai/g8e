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
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func modelsEvalStageCmd() *cobra.Command {
	var catalogPath string
	var ollamaEndpoint string
	var pullTimeout time.Duration
	var variantIDs []string
	var dryRun bool
	var formationCatalog bool

	cmd := &cobra.Command{
		Use:   "stage",
		Short: "Stage rollout-intake models from Hugging Face through Ollama",
		Long: `Pull rollout-intake models from Hugging Face via Ollama deep HF compatibility.

Reads eval/rollout-intake-hf.json, uses the official Ollama Go client for /api/pull
and /api/copy, and applies canonical served-model aliases for the rollout queue.

Use --formation-catalog to pull the nine sovereign ExecutionTopologies served tags
to the approved remote Ollama provider through the same staging path.

Examples:
  g8e eval models stage
  g8e eval models stage --variant-id qwen3-8-27b --variant-id gpt-oss-20b
  g8e eval models stage --formation-catalog --ollama-endpoint http://192.168.1.2:11434
  g8e eval models stage --pull-timeout 24h --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ollamaEndpoint == "" {
				ollamaEndpoint = os.Getenv("G8E_OLLAMA_ENDPOINT")
			}
			stageReq := evaluation.RolloutIntakeStageRequest{
				Context:        cmd.Context(),
				CatalogPath:    catalogPath,
				OllamaEndpoint: ollamaEndpoint,
				PullTimeout:    pullTimeout,
				VariantIDs:     variantIDs,
				DryRun:         dryRun,
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
			var (
				result *evaluation.RolloutIntakeStageResult
				err    error
			)
			if formationCatalog {
				result, err = evaluation.StageFormationCatalogIntake(stageReq)
			} else {
				result, err = evaluation.StageRolloutIntake(stageReq)
			}
			if err != nil {
				return fmt.Errorf("evaluation: models stage: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			for _, tag := range result.Pulled {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s\n", tag); err != nil {
					return err
				}
			}
			for _, tag := range result.Aliased {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Aliased %s\n", tag); err != nil {
					return err
				}
			}
			for _, skip := range result.Skipped {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s: %s\n", skip.ServedModelTag, skip.Reason); err != nil {
					return err
				}
			}
			for _, failure := range result.Failed {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Failed %s: %s\n", failure.ServedModelTag, failure.Err); err != nil {
					return err
				}
			}
			if len(result.Failed) > 0 {
				return fmt.Errorf("evaluation: models stage: %d model(s) failed", len(result.Failed))
			}
			if formationCatalog {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "\nNext: freeze and materialize the formation benchmark inventory:")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "  G8E_OLLAMA_ENDPOINT=%s g8e eval models freeze --campaign-id eval-formations-benchmark --output .g8e/eval/inventories/eval-formations-provider-freeze.json\n", ollamaEndpoint)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "  g8e eval models materialize --from .g8e/eval/inventories/eval-formations-provider-freeze.json --formation-catalog --campaign-id eval-formations-benchmark --output .g8e/eval/inventories/eval-formations-benchmark.json")
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "\nNext: re-freeze provider inventory, then rebuild the rollout queue:")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "  g8e eval models freeze --campaign-id eval-genesis-homogeneous --ollama-endpoint %s --output .g8e/eval/model-inventory.json\n", ollamaEndpoint)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "  g8e eval rollout init --from eval/base-model-inventory.json --materialize --merge")
			return err
		},
	}

	cmd.Flags().StringVar(&catalogPath, "catalog", evaluation.DefaultRolloutIntakeCatalogRelPath, "Rollout intake catalog JSON path")
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint (default: G8E_OLLAMA_ENDPOINT)")
	cmd.Flags().DurationVar(&pullTimeout, "pull-timeout", evaluation.DefaultRolloutIntakePullTimeout(), "Maximum time to wait for each Hugging Face pull")
	cmd.Flags().StringArrayVar(&variantIDs, "variant-id", nil, "Stage only these catalog variant IDs (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the staging plan without pulling")
	cmd.Flags().BoolVar(&formationCatalog, "formation-catalog", false, "Pull sovereign ExecutionTopologies served tags to the remote provider")
	return cmd
}

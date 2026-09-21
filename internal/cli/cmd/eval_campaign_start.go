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
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func campaignEvalStartCmd(deps nativeEvalDeps) *cobra.Command {
	var modelTag string
	var modelTags string
	var queueRef string
	var campaignID string
	var inventoryFile string
	var runID string
	var inferenceSessionID string
	var dataSessionID string
	var ensembleURL string
	var ollamaEndpoint string
	var dryRun bool
	var prepareOnly bool
	var publish bool
	var daemon bool
	var verify bool
	var requireProviderObservation bool
	var requireModelProvenance bool
	var noAutoRefresh bool
	var tierA bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize, schedule, and execute one homogeneous campaign run",
		Long: `Resolve model selection (or the init-campaign queue), bind operator sessions,
initialize the campaign, schedule assignments, and execute them in one flow.

Examples:
  g8e eval campaign start --model gemma3:4b --publish --daemon
  g8e eval campaign start --models qwen3:0.6b,qwen3:4b,gemma3:4b --publish --daemon
  g8e eval campaign start --queue next --publish --daemon --verify --tier-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tierA {
				requireProviderObservation = true
				requireModelProvenance = true
			}
			flowOpts := campaignStartFlowOptions{
				ModelTag:                   modelTag,
				ModelTags:                  splitCSVModelTags(modelTags),
				QueueRef:                   queueRef,
				CampaignID:                 campaignID,
				InventoryFile:              inventoryFile,
				RunID:                      runID,
				InferenceSessionID:         inferenceSessionID,
				DataSessionID:              dataSessionID,
				EnsembleURL:                ensembleURL,
				OllamaEndpoint:             ollamaEndpoint,
				DryRun:                     dryRun,
				PrintPlan:                  !dryRun,
				PrepareOnly:                prepareOnly,
				Publish:                    publish,
				Daemon:                     daemon,
				Verify:                     verify,
				RequireProviderObservation: requireProviderObservation,
				RequireModelProvenance:     requireModelProvenance,
				NoAutoRefresh:              noAutoRefresh,
				JSONOutput:                 output.JSONEnabled(cmd),
			}
			result, err := runCampaignStartFlow(cmd, deps, flowOpts)
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) && result != nil && !dryRun {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":     result.Plan.CampaignID,
					"run_id":          result.Plan.RunID,
					"executed":        result.Executed,
					"verified":        result.Report != nil,
					"verify_status":   verificationStatusString(result.Report),
					"failure_count":   verificationFailureCount(result.Report),
					"failure_reasons": verificationFailureReasons(result.Report),
					"prepared":        prepareOnly,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if prepareOnly && result != nil {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Prepared run %s. Run:\n  ./g8e eval campaign execute --run-id %s --publish --daemon\n", result.Plan.RunID, result.Plan.RunID)
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modelTag, "model", "", "Single served model tag (for example gemma3:4b)")
	cmd.Flags().StringVar(&modelTags, "models", "", "Comma-separated served model tags")
	cmd.Flags().StringVar(&queueRef, "queue", "", "Init-campaign queue selector: next, served tag, or variant_id")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Override campaign ID")
	cmd.Flags().StringVar(&inventoryFile, "inventory-file", "", "Override inventory freeze JSON path")
	cmd.Flags().StringVar(&runID, "run-id", "", "Override run ID")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Pin the inference Operator session ID")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Pin the data Operator session ID")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint for model maintenance (default: active inference operator runtime_config, then G8E_OLLAMA_ENDPOINT, then loopback)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the resolved plan without running")
	cmd.Flags().BoolVar(&prepareOnly, "prepare-only", false, "Initialize and schedule only; stop before execute")
	cmd.Flags().BoolVar(&publish, "publish", true, "Publish lifecycle projections to the public mirror during schedule and execute")
	cmd.Flags().BoolVar(&daemon, "daemon", true, "Execute continuously until the queued matrix is exhausted")
	cmd.Flags().BoolVar(&verify, "verify", false, "Run campaign verify after execute completes")
	cmd.Flags().BoolVar(&requireProviderObservation, "require-provider-observation", false, "Fail verify when provider-boundary observation windows are missing")
	cmd.Flags().BoolVar(&requireModelProvenance, "require-model-provenance", false, "Fail verify when model provenance attestation windows are missing or digest_match is false")
	cmd.Flags().BoolVar(&tierA, "tier-a", false, "Tier-A verify preset: require provider observation and model provenance")
	cmd.Flags().BoolVar(&noAutoRefresh, "no-auto-refresh", false, "Do not refresh stale CLI operator bindings before execution")
	return cmd
}

func splitCSVModelTags(raw string) []string {
	if raw == "" {
		return nil
	}
	tags := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

func verificationStatusString(report *evalv1.EvaluationVerificationReport) string {
	if report == nil {
		return ""
	}
	return report.GetStatus().String()
}

func verificationFailureCount(report *evalv1.EvaluationVerificationReport) int {
	if report == nil {
		return 0
	}
	return int(report.GetFailureCount())
}

func verificationFailureReasons(report *evalv1.EvaluationVerificationReport) []string {
	if report == nil {
		return nil
	}
	return report.GetFailureReasons()
}

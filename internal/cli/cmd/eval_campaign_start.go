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
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
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
	var waitForProviderIdle bool
	var providerIdlePoll time.Duration
	var providerSettle time.Duration
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize, schedule, and execute one homogeneous campaign run",
		Long: `Resolve model selection (or the init-campaign queue), bind operator sessions,
initialize the campaign, schedule assignments, and execute them in one flow.

Examples:
  g8e eval campaign start --model gemma3:4b --publish --daemon
  g8e eval campaign start --models qwen3:0.6b,qwen3:4b,gemma3:4b --publish --daemon
  g8e eval campaign start --queue next --publish --daemon --verify`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			plan, err := evaluation.ResolveCampaignStartPlan(evaluation.CampaignStartPlanRequest{
				ProjectRoot:   cfg.ProjectRoot,
				ModelTag:      modelTag,
				ModelTags:     splitCSVModelTags(modelTags),
				QueueRef:      queueRef,
				CampaignID:    campaignID,
				InventoryFile: inventoryFile,
				RunID:         runID,
				Now:           deps.now().UTC(),
			})
			if err != nil {
				return err
			}
			sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, inferenceSessionID, dataSessionID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign start: %w", err)
			}
			if dryRun {
				return writeCampaignStartPlan(cmd.OutOrStdout(), plan, sessions, output.JSONEnabled(cmd))
			}
			if err := writeCampaignStartPlan(cmd.OutOrStdout(), plan, sessions, output.JSONEnabled(cmd)); err != nil {
				return err
			}
			startedAt := deps.now().UTC()
			if err := initializeCampaignRun(cmd, deps, plan, sessions); err != nil {
				return err
			}
			if !output.JSONEnabled(cmd) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized campaign %s run %s\n", plan.CampaignID, plan.RunID)
			}
			assignmentCount, err := scheduleHomogeneousCampaignRun(cmd, deps, plan.RunID, publish)
			if err != nil {
				return err
			}
			if !output.JSONEnabled(cmd) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d assignments for run %s\n", assignmentCount, plan.RunID)
			}
			if err := persistActiveCampaignRun(cfg.ProjectRoot, plan, startedAt); err != nil {
				return err
			}
			if prepareOnly {
				if output.JSONEnabled(cmd) {
					payload, err := json.MarshalIndent(map[string]any{
						"campaign_id":      plan.CampaignID,
						"run_id":           plan.RunID,
						"assignment_count": assignmentCount,
						"prepared":         true,
					}, "", "  ")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Prepared run %s (%d assignments). Run:\n  ./g8e eval campaign execute --run-id %s --publish --daemon\n", plan.RunID, assignmentCount, plan.RunID)
				return err
			}
			executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
				RunID:               plan.RunID,
				Publish:             publish,
				Daemon:              daemon,
				InferenceSessionID:  sessions.InferenceSessionID,
				DataSessionID:       sessions.DataSessionID,
				EnsembleURL:         ensembleURL,
				OllamaEndpoint:      ollamaEndpoint,
				NoAutoRefresh:       noAutoRefresh,
				WaitForProviderIdle: waitForProviderIdle,
				ProviderIdlePoll:    providerIdlePoll,
				ProviderSettle:      providerSettle,
				JSONOutput:          output.JSONEnabled(cmd),
			})
			if err != nil {
				return err
			}
			if !output.JSONEnabled(cmd) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %d assignment(s) for run %s\n", executed, plan.RunID)
			}
			if !verify {
				if output.JSONEnabled(cmd) {
					payload, err := json.MarshalIndent(map[string]any{
						"campaign_id": plan.CampaignID,
						"run_id":      plan.RunID,
						"executed":    executed,
						"verified":    false,
					}, "", "  ")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
					return err
				}
				return nil
			}
			report, err := verifyCampaignRun(cmd, deps, plan.RunID, requireProviderObservation, requireModelProvenance, output.JSONEnabled(cmd))
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":     plan.CampaignID,
					"run_id":          plan.RunID,
					"executed":        executed,
					"verified":        true,
					"verify_status":   report.GetStatus().String(),
					"failure_count":   report.GetFailureCount(),
					"failure_reasons": report.GetFailureReasons(),
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run %s verification: %s (%d failure(s))\n", plan.RunID, report.GetStatus().String(), report.GetFailureCount())
				if report.GetFailureCount() > 0 {
					for _, reason := range report.GetFailureReasons() {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
					}
				}
			}
			if report.GetFailureCount() > 0 {
				return constants.ErrEvalRunVerificationFailed
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
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint for provider-idle gating (default: active inference operator runtime_config, then G8E_OLLAMA_ENDPOINT, then loopback)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the resolved plan without running")
	cmd.Flags().BoolVar(&prepareOnly, "prepare-only", false, "Initialize and schedule only; stop before execute")
	cmd.Flags().BoolVar(&publish, "publish", true, "Publish lifecycle projections to the public mirror during schedule and execute")
	cmd.Flags().BoolVar(&daemon, "daemon", true, "Execute continuously until the queued matrix is exhausted")
	cmd.Flags().BoolVar(&verify, "verify", false, "Run campaign verify after execute completes")
	cmd.Flags().BoolVar(&requireProviderObservation, "require-provider-observation", false, "Fail verify when provider-boundary observation windows are missing")
	cmd.Flags().BoolVar(&requireModelProvenance, "require-model-provenance", false, "Fail verify when model provenance attestation windows are missing or digest_match is false")
	cmd.Flags().BoolVar(&noAutoRefresh, "no-auto-refresh", false, "Do not refresh stale CLI operator bindings before execution")
	cmd.Flags().BoolVar(&waitForProviderIdle, "wait-for-provider-idle", true, "Wait for Ollama to become idle before each assignment")
	cmd.Flags().DurationVar(&providerIdlePoll, "provider-idle-poll", 2*time.Second, "Poll interval while waiting for Ollama idle")
	cmd.Flags().DurationVar(&providerSettle, "provider-settle", 8*time.Second, "Required stable /api/ps window before starting the next assignment")
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

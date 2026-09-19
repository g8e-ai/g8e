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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type campaignQueueRunResult struct {
	Planned   int                          `json:"planned"`
	Succeeded int                          `json:"succeeded"`
	Failed    int                          `json:"failed"`
	Failures  []campaignQueueRunFailure    `json:"failures,omitempty"`
	Runs      []campaignQueueRunSuccess    `json:"runs,omitempty"`
	LogDir    string                       `json:"log_dir,omitempty"`
}

type campaignQueueRunFailure struct {
	VariantID string `json:"variant_id"`
	Tag       string `json:"served_model_tag"`
	Error     string `json:"error"`
}

type campaignQueueRunSuccess struct {
	VariantID string `json:"variant_id"`
	Tag       string `json:"served_model_tag"`
	RunID     string `json:"run_id"`
}

func rolloutEvalRunCmd(deps nativeEvalDeps) *cobra.Command {
	var queueFile string
	var skipVariantIDs []string
	var skipVerified bool
	var continueOnError bool
	var dryRun bool
	var tierA bool
	var verify bool
	var publish bool
	var daemon bool
	var ollamaEndpoint string
	var ensembleURL string
	var inferenceSessionID string
	var dataSessionID string
	var providerSettle time.Duration
	var waitForWitnesses time.Duration
	var logDir string
	var ensembleHealthURL string
	var mirrorBootstrapURL string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run init-campaign start → verify for every queued model",
		Long: `Execute the init-campaign rollout queue unattended. Each entry runs the same
flow as 'g8e eval campaign start --queue <variant> --verify' and updates the queue
on Tier-A verify PASS.

Examples:
  g8e eval rollout run --tier-a --skip-verified
  g8e eval rollout run --dry-run --skip-variant granite3-3-2b
  g8e eval rollout run --tier-a --continue-on-error --log-dir .g8e/eval/logs/batch-001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			queue, queuePath, err := loadInitCampaignQueue(cfg.ProjectRoot, queueFile)
			if err != nil {
				return fmt.Errorf("evaluation: queue run: %w", err)
			}
			plan := queue.BuildBatchPlan(evaluation.CampaignQueueBatchPlanRequest{
				SkipVariantIDs: skipVariantIDs,
				SkipVerified:   skipVerified,
			})
			if len(plan) == 0 {
				cmd.Println("No queue entries selected")
				return nil
			}
			if dryRun {
				return writeCampaignQueueRunPlan(cmd, plan, output.JSONEnabled(cmd))
			}
			if logDir == "" {
				logDir = filepath.Join(cfg.ProjectRoot, ".g8e/eval/logs", "queue-run-"+deps.now().UTC().Format("20060102-150405"))
			} else {
				logDir = evaluation.ResolveEvalPath(cfg.ProjectRoot, logDir)
			}
			if err := os.MkdirAll(logDir, 0o700); err != nil {
				return fmt.Errorf("evaluation: queue run: create log dir: %w", err)
			}
			if err := preflightCampaignQueueRun(cmd, deps, cfg, waitForWitnesses, ensembleHealthURL, mirrorBootstrapURL); err != nil {
				return fmt.Errorf("evaluation: queue run: %w", err)
			}
			requireProviderObservation := tierA
			requireModelProvenance := tierA
			if tierA {
				verify = true
			}
			result := campaignQueueRunResult{
				Planned: len(plan),
				LogDir:  logDir,
			}
			stdout := cmd.OutOrStdout()
			stderr := cmd.ErrOrStderr()
			for _, entry := range plan {
				_, _ = fmt.Fprintf(stdout, "=== START %s (%s) ===\n", entry.VariantID, entry.ServedModelTag)
				modelLogPath := filepath.Join(logDir, entry.VariantID+".log")
				logFile, err := os.Create(modelLogPath)
				if err != nil {
					return fmt.Errorf("evaluation: queue run: create log file: %w", err)
				}
				teeOut := io.MultiWriter(stdout, logFile)
				teeErr := io.MultiWriter(stderr, logFile)
				subCmd := *cmd
				subCmd.SetOut(teeOut)
				subCmd.SetErr(teeErr)
				flowResult, runErr := runCampaignStartFlow(&subCmd, deps, campaignStartFlowOptions{
					QueueRef:                   entry.VariantID,
					InferenceSessionID:         inferenceSessionID,
					DataSessionID:              dataSessionID,
					EnsembleURL:                ensembleURL,
					OllamaEndpoint:             ollamaEndpoint,
					Publish:                    publish,
					Daemon:                     daemon,
					Verify:                     verify,
					RequireProviderObservation: requireProviderObservation,
					RequireModelProvenance:     requireModelProvenance,
					WaitForProviderIdle:        true,
					ProviderSettle:             providerSettle,
				})
				closeErr := logFile.Close()
				if runErr != nil {
					result.Failed++
					result.Failures = append(result.Failures, campaignQueueRunFailure{
						VariantID: entry.VariantID,
						Tag:       entry.ServedModelTag,
						Error:     runErr.Error(),
					})
					_, _ = fmt.Fprintf(stdout, "FAIL %s (%s) — see %s\n", entry.VariantID, entry.ServedModelTag, modelLogPath)
					if closeErr != nil {
						_, _ = fmt.Fprintf(stderr, "warning: close log file: %v\n", closeErr)
					}
					if !continueOnError {
						return finishCampaignQueueRun(cmd, queuePath, result, runErr)
					}
					continue
				}
				if closeErr != nil {
					_, _ = fmt.Fprintf(stderr, "warning: close log file: %v\n", closeErr)
				}
				result.Succeeded++
				if flowResult != nil && flowResult.Plan != nil {
					result.Runs = append(result.Runs, campaignQueueRunSuccess{
						VariantID: entry.VariantID,
						Tag:       entry.ServedModelTag,
						RunID:     flowResult.Plan.RunID,
					})
					_, _ = fmt.Fprintf(stdout, "PASS %s → %s\n", entry.VariantID, flowResult.Plan.RunID)
				}
			}
			return finishCampaignQueueRun(cmd, queuePath, result, nil)
		},
	}
	cmd.Flags().StringVar(&queueFile, "queue-file", "", "Queue manifest path (default: .g8e/eval/init-campaign-queue.json)")
	cmd.Flags().StringSliceVar(&skipVariantIDs, "skip-variant", nil, "Variant IDs to exclude (repeatable)")
	cmd.Flags().BoolVar(&skipVerified, "skip-verified", true, "Skip queue entries already marked verified")
	cmd.Flags().BoolVar(&continueOnError, "continue-on-error", false, "Log failure and continue to the next model")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the rollout plan without executing")
	cmd.Flags().BoolVar(&tierA, "tier-a", true, "Tier-A verify preset: require provider observation and model provenance")
	cmd.Flags().BoolVar(&verify, "verify", true, "Verify each run after execute completes")
	cmd.Flags().BoolVar(&publish, "publish", true, "Publish lifecycle projections during schedule and execute")
	cmd.Flags().BoolVar(&daemon, "daemon", true, "Execute continuously until each model matrix is exhausted")
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint (default: G8E_OLLAMA_ENDPOINT)")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Pin the inference Operator session ID")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Pin the data Operator session ID")
	cmd.Flags().DurationVar(&providerSettle, "provider-settle", 8*time.Second, "Required stable /api/ps window before each assignment")
	cmd.Flags().DurationVar(&waitForWitnesses, "wait-for-witnesses", 0, "Poll up to this duration for active observer and provenance operators")
	cmd.Flags().StringVar(&logDir, "log-dir", "", "Directory for per-model logs (default: .g8e/eval/logs/queue-run-TIMESTAMP)")
	cmd.Flags().StringVar(&ensembleHealthURL, "ensemble-health-url", "http://127.0.0.1:8000/health", "Preflight g8ee health URL")
	cmd.Flags().StringVar(&mirrorBootstrapURL, "mirror-bootstrap-url", "http://127.0.0.1:8082/bootstrap", "Preflight public mirror bootstrap URL")
	return cmd
}

func writeCampaignQueueRunPlan(cmd *cobra.Command, plan []evaluation.CampaignQueueModel, jsonOutput bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(map[string]any{"models": plan}, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Queue rollout plan (%d model(s))\n", len(plan))
	if err != nil {
		return err
	}
	for _, entry := range plan {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s) status=%s\n", entry.VariantID, entry.ServedModelTag, entry.Status)
		if err != nil {
			return err
		}
	}
	return nil
}

func finishCampaignQueueRun(cmd *cobra.Command, queuePath string, result campaignQueueRunResult, runErr error) error {
	if output.JSONEnabled(cmd) {
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		if err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Queue run finished: planned=%d succeeded=%d failed=%d queue=%s\n",
			result.Planned, result.Succeeded, result.Failed, queuePath)
		if result.LogDir != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logs: %s\n", result.LogDir)
		}
	}
	if runErr != nil {
		return runErr
	}
	if result.Failed > 0 {
		return fmt.Errorf("evaluation: queue run: %d model(s) failed", result.Failed)
	}
	return nil
}

func preflightCampaignQueueRun(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	cfg *config.Config,
	waitForWitnesses time.Duration,
	ensembleHealthURL string,
	mirrorBootstrapURL string,
) error {
	if err := checkHTTPReachable(cmd.Context(), ensembleHealthURL); err != nil {
		return fmt.Errorf("preflight ensemble health: %w", err)
	}
	if err := checkHTTPReachable(cmd.Context(), mirrorBootstrapURL); err != nil {
		return fmt.Errorf("preflight mirror bootstrap: %w", err)
	}
	deadline := deps.now()
	if waitForWitnesses > 0 {
		deadline = deadline.Add(waitForWitnesses)
	}
	for {
		status, err := campaignWitnessStatus(cmd, deps, cfg)
		if err != nil {
			return err
		}
		if status.Ready {
			if status.ActiveObserverOllamaCount < 1 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no active observer with --ollama; governed Ollama reset between assignments will be skipped\n")
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Preflight ok (observer=%d provenance=%d observer_ollama=%d)\n",
				status.ActiveObserverCount, status.ActiveProvenanceCount, status.ActiveObserverOllamaCount)
			return nil
		}
		if waitForWitnesses <= 0 || !deps.now().Before(deadline) {
			break
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Waiting for active observer and provenance operators...\n")
		time.Sleep(5 * time.Second)
	}
	status, err := campaignWitnessStatus(cmd, deps, cfg)
	if err != nil {
		return err
	}
	if status.ActiveObserverCount < 1 {
		return fmt.Errorf("preflight: no active provider-boundary observer enrolled")
	}
	if status.ActiveProvenanceCount < 1 {
		return fmt.Errorf("preflight: no active provenance operator enrolled")
	}
	return nil
}

func campaignWitnessStatus(cmd *cobra.Command, deps nativeEvalDeps, cfg *config.Config) (evaluation.CampaignWitnessStatus, error) {
	_, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatEvalDeps{
		configLoader:         deps.configLoader,
		fileSvcFactory:       deps.fileSvcFactory,
		authLoader:           deps.authLoader,
		clientFactory:        deps.clientFactory,
		refreshClientFactory: defaultRefreshClientFactory,
		now:                  deps.now,
		newID:                deps.newID,
	})
	if err != nil {
		return evaluation.CampaignWitnessStatus{}, err
	}
	operators, err := chatEvalListOperators(cmd, chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	}, cfg, authContext)
	if err != nil {
		return evaluation.CampaignWitnessStatus{}, err
	}
	_ = fileSvc
	return evaluation.CampaignWitnessStatusFromOperators(operators), nil
}

func checkHTTPReachable(ctx context.Context, rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("missing URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return nil
}

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
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const rolloutSmokeCampaignSuffix = "-smoke"

type rolloutRunResult struct {
	Planned   int                 `json:"planned"`
	Succeeded int                 `json:"succeeded"`
	Failed    int                 `json:"failed"`
	Failures  []rolloutRunFailure `json:"failures,omitempty"`
	Runs      []rolloutRunSuccess `json:"runs,omitempty"`
	LogDir    string              `json:"log_dir,omitempty"`
}

type rolloutRunFailure struct {
	VariantID string `json:"variant_id"`
	Tag       string `json:"served_model_tag"`
	Error     string `json:"error"`
}

type rolloutRunSuccess struct {
	VariantID string `json:"variant_id"`
	Tag       string `json:"served_model_tag"`
	RunID     string `json:"run_id"`
	Gate      string `json:"gate"`
}

const (
	rolloutGateSmoke = "smoke"
	rolloutGateFull  = "full"
)

type rolloutRunOptions struct {
	Until              int
	SkipVerified       bool
	DryRun             bool
	GateSmoke          bool
	PromoteOnPass      bool
	LogDir             string
	EnsembleHealthURL  string
	MirrorBootstrapURL string
}

func rolloutRunCmd(deps nativeEvalDeps) *cobra.Command {
	opts := rolloutRunOptions{SkipVerified: true}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Qualify pending rollout models: create, run, verify, and publish each",
		Long: `Execute the rollout queue unattended. Each pending entry is created as a
campaign from its registry model and run with publication, strict witness
verification (provider observation and model provenance), and the daemon
executor. An entry is marked verified only when a full run verifies; a failed
entry is marked failed and the batch continues.

--gate-smoke screens each model first with a fast smoke campaign
(<campaign>-smoke). A smoke pass does not verify the entry; add
--promote-on-pass to follow it with the full run.

Examples:
  g8e eval rollout run --until 1
  g8e eval rollout run --gate-smoke --promote-on-pass
  g8e eval rollout run --dry-run
  g8e eval rollout run --skip-verified=false --log-dir eval/logs/batch-001`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Until < 0 {
				return fmt.Errorf("evaluation: rollout run: --until must not be negative")
			}
			if opts.PromoteOnPass && !opts.GateSmoke {
				return fmt.Errorf("evaluation: rollout run: --promote-on-pass requires --gate-smoke")
			}
			return runRollout(cmd, deps, opts)
		},
	}
	cmd.Flags().IntVar(&opts.Until, "until", 0, "Run at most this many models (0 runs every pending model)")
	cmd.Flags().BoolVar(&opts.SkipVerified, "skip-verified", true, "Skip queue entries already marked verified")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Print the rollout plan without executing")
	cmd.Flags().BoolVar(&opts.GateSmoke, "gate-smoke", false, "Screen each model with the fast smoke campaign before any full run")
	cmd.Flags().BoolVar(&opts.PromoteOnPass, "promote-on-pass", false, "Run the full campaign for models that pass the smoke gate")
	cmd.Flags().StringVar(&opts.LogDir, "log-dir", "", "Directory for per-model logs (default: "+constants.RuntimeDirname+"/"+constants.EvaluationQueueLogsDirname+"/rollout-<timestamp>)")
	cmd.Flags().StringVar(&opts.EnsembleHealthURL, "ensemble-health-url", "http://127.0.0.1:8000/health", "Preflight g8ee health URL")
	cmd.Flags().StringVar(&opts.MirrorBootstrapURL, "mirror-bootstrap-url", "http://127.0.0.1:8082/bootstrap", "Preflight public mirror bootstrap URL")
	return cmd
}

// rolloutGateRunner runs one gate ("smoke" or "full") of one queue entry and
// returns the run it started. The run is non-nil whenever one started,
// including when it then failed.
type rolloutGateRunner func(cmd *cobra.Command, entry evaluation.CampaignQueueModel, gate string) (*rolloutRunSuccess, error)

// rolloutGateFactory builds the gate runner once the registry is loaded.
type rolloutGateFactory func(fileSvc fs.RuntimeFileService, registry []*evalv1.ModelVariant) rolloutGateRunner

// productionRolloutGates creates each gate's campaign from the entry's registry
// model and starts a run of it with strict witness verification.
func productionRolloutGates(deps nativeEvalDeps) rolloutGateFactory {
	return func(fileSvc fs.RuntimeFileService, registry []*evalv1.ModelVariant) rolloutGateRunner {
		return func(cmd *cobra.Command, entry evaluation.CampaignQueueModel, gate string) (*rolloutRunSuccess, error) {
			variant := findVariant(registry, entry.VariantID)
			if variant == nil {
				return nil, fmt.Errorf("model %q is not in the registry (run `g8e eval models import %s`): %w", entry.VariantID, entry.ServedModelTag, constants.ErrInferenceModelNotFound)
			}
			return runRolloutGate(cmd, deps, fileSvc, variant, entry, gate)
		}
	}
}

func runRollout(cmd *cobra.Command, deps nativeEvalDeps, opts rolloutRunOptions) error {
	return runRolloutWith(cmd, deps, opts, productionRolloutGates(deps))
}

func runRolloutWith(cmd *cobra.Command, deps nativeEvalDeps, opts rolloutRunOptions, gates rolloutGateFactory) error {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return err
	}
	queue, err := evaluation.LoadRolloutQueue(cmd.Context(), fileSvc)
	if err != nil {
		return fmt.Errorf("evaluation: rollout run: %w", err)
	}
	plan := queue.BuildBatchPlan(evaluation.CampaignQueueBatchPlanRequest{SkipVerified: opts.SkipVerified, Until: opts.Until})
	if len(plan) == 0 {
		cmd.Println("No queue entries selected")
		return nil
	}
	if opts.DryRun {
		return writeRolloutRunPlan(cmd, plan, output.JSONEnabled(cmd))
	}
	logDir, err := rolloutLogDir(opts.LogDir, deps)
	if err != nil {
		return err
	}
	if err := fileSvc.MkdirAll(cmd.Context(), logDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("evaluation: rollout run: create log dir: %w", err)
	}
	if err := preflightRollout(cmd, opts.EnsembleHealthURL, opts.MirrorBootstrapURL); err != nil {
		return fmt.Errorf("evaluation: rollout run: %w", err)
	}
	registry, err := newModelInventories(fileSvc, cfg.ProjectRoot).variants(cmd.Context(), modelScopeRegistry)
	if err != nil {
		return fmt.Errorf("evaluation: rollout run: %w", err)
	}
	runGate := gates(fileSvc, registry)
	jsonOutput := output.JSONEnabled(cmd)
	result := rolloutRunResult{Planned: len(plan), LogDir: logDir}
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	// Structured output stays one document on stdout, so progress moves to stderr.
	progress := stdout
	if jsonOutput {
		progress = stderr
	}
	for _, entry := range plan {
		_, _ = fmt.Fprintf(progress, "=== START %s (%s) ===\n", entry.VariantID, entry.ServedModelTag)
		modelLogPath := path.Join(logDir, entry.VariantID+constants.FileExtText)
		logFile, err := fileSvc.OpenForAppend(cmd.Context(), modelLogPath, constants.PermFilePrivate)
		if err != nil {
			return fmt.Errorf("evaluation: rollout run: create log file: %w", err)
		}
		entryOut := io.MultiWriter(stdout, logFile)
		if jsonOutput {
			// Structured output stays a single document; the per-model log
			// still records everything.
			entryOut = logFile
		}
		entryErr := io.MultiWriter(stderr, logFile)
		sub := *cmd
		sub.SetOut(entryOut)
		sub.SetErr(entryErr)

		runs, runErr := qualifyRolloutEntry(&sub, fileSvc, runGate, entry, opts)
		if closeErr := logFile.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(stderr, "warning: close log file: %v\n", closeErr)
		}
		result.Runs = append(result.Runs, runs...)
		if runErr != nil {
			result.Failed++
			result.Failures = append(result.Failures, rolloutRunFailure{VariantID: entry.VariantID, Tag: entry.ServedModelTag, Error: runErr.Error()})
			_, _ = fmt.Fprintf(entryErr, "Error: %v\n", runErr)
			_, _ = fmt.Fprintf(progress, "FAIL %s (%s), see %s\n", entry.VariantID, entry.ServedModelTag, modelLogPath)
			if markErr := markRolloutFailure(cmd.Context(), fileSvc, entry, runs, runErr); markErr != nil {
				_, _ = fmt.Fprintf(stderr, "warning: update queue entry %s: %v\n", entry.VariantID, markErr)
			}
			continue
		}
		result.Succeeded++
		if len(runs) > 0 {
			last := runs[len(runs)-1]
			_, _ = fmt.Fprintf(progress, "PASS %s %s -> %s\n", entry.VariantID, last.Gate, last.RunID)
		}
	}
	return finishRolloutRun(cmd, result)
}

// qualifyRolloutEntry runs the gates the options ask for against one queue
// entry and records the outcome in the queue. It returns every run that
// completed, in order, even when a later gate failed.
func qualifyRolloutEntry(cmd *cobra.Command, fileSvc fs.RuntimeFileService, runGate rolloutGateRunner, entry evaluation.CampaignQueueModel, opts rolloutRunOptions) ([]rolloutRunSuccess, error) {
	var runs []rolloutRunSuccess
	if opts.GateSmoke {
		run, err := runGate(cmd, entry, rolloutGateSmoke)
		if run != nil {
			runs = append(runs, *run)
		}
		if err != nil {
			return runs, err
		}
		if !opts.PromoteOnPass {
			// A smoke pass screens the model. Only a full run verifies it.
			_, err := evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
				Context:        cmd.Context(),
				FileService:    fileSvc,
				VariantID:      entry.VariantID,
				ServedModelTag: entry.ServedModelTag,
				Status:         evaluation.QueueStatusPending,
				Notes:          "smoke gate passed: run " + run.RunID,
			})
			return runs, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PROMOTING %s (%s) from the smoke gate to full qualification\n", entry.VariantID, entry.ServedModelTag)
	}
	run, err := runGate(cmd, entry, rolloutGateFull)
	if run != nil {
		runs = append(runs, *run)
	}
	if err != nil {
		return runs, err
	}
	_, err = evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
		Context:        cmd.Context(),
		FileService:    fileSvc,
		VariantID:      entry.VariantID,
		ServedModelTag: entry.ServedModelTag,
		Status:         evaluation.QueueStatusVerified,
		VerifiedRunID:  run.RunID,
		Notes:          evaluation.StrictWitnessVerifyNotes(run.RunID),
	})
	return runs, err
}

// runRolloutGate creates the campaign for one gate of one model and starts a
// run of it with strict witness verification. The returned run is non-nil
// whenever a run was started, including when it then failed.
func runRolloutGate(cmd *cobra.Command, deps nativeEvalDeps, fileSvc fs.RuntimeFileService, variant *evalv1.ModelVariant, entry evaluation.CampaignQueueModel, gate string) (*rolloutRunSuccess, error) {
	campaignID := entry.CampaignID
	smoke := gate == rolloutGateSmoke
	if smoke {
		campaignID += rolloutSmokeCampaignSuffix
	}
	if _, err := createCampaign(cmd.Context(), deps, fileSvc, campaignCreateSpec{
		CampaignID:  campaignID,
		Variants:    []*evalv1.ModelVariant{variant},
		Repetitions: 1,
		Smoke:       smoke,
	}); err != nil {
		return nil, err
	}
	result, err := runStartFlow(cmd, deps, runStartFlowOptions{
		CampaignID:         campaignID,
		Publish:            true,
		Daemon:             true,
		Verify:             true,
		RequireObservation: true,
		RequireProvenance:  true,
	})
	var run *rolloutRunSuccess
	if result != nil && result.RunID != "" {
		run = &rolloutRunSuccess{VariantID: entry.VariantID, Tag: entry.ServedModelTag, RunID: result.RunID, Gate: gate}
	}
	return run, err
}

func findVariant(variants []*evalv1.ModelVariant, variantID string) *evalv1.ModelVariant {
	for _, variant := range variants {
		if variant != nil && variant.GetVariantId() == variantID {
			return variant
		}
	}
	return nil
}

// markRolloutFailure records a failed entry, naming the run that failed when
// one started.
func markRolloutFailure(ctx context.Context, fileSvc fs.RuntimeFileService, entry evaluation.CampaignQueueModel, runs []rolloutRunSuccess, runErr error) error {
	notes := runErr.Error()
	runID := ""
	if len(runs) > 0 {
		runID = runs[len(runs)-1].RunID
		notes = fmt.Sprintf("run %s: %s", runID, runErr.Error())
	}
	if _, err := evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
		Context:       ctx,
		FileService:   fileSvc,
		VariantID:     entry.VariantID,
		Status:        evaluation.QueueStatusFailed,
		VerifiedRunID: runID,
		Notes:         notes,
	}); err != nil {
		return fmt.Errorf("evaluation: update rollout queue: %w", err)
	}
	return nil
}

func rolloutLogDir(requested string, deps nativeEvalDeps) (string, error) {
	if requested == "" {
		logDir := evaluation.QueueLogDir("rollout-" + deps.now().UTC().Format("20060102-150405"))
		if logDir == "" {
			return "", constants.ErrEvaluationRolloutLogDirInvalid
		}
		return logDir, nil
	}
	runtimePrefix := filepath.ToSlash(constants.RuntimeDirname) + "/"
	logDir := strings.TrimPrefix(filepath.ToSlash(requested), runtimePrefix)
	if filepath.IsAbs(logDir) || strings.HasPrefix(logDir, "../") || logDir == ".." || logDir == "" {
		return "", constants.ErrEvaluationRolloutLogDirNotRelative
	}
	return logDir, nil
}

type rolloutRunPlanJSON struct {
	Models []evaluation.CampaignQueueModel `json:"models"`
}

func writeRolloutRunPlan(cmd *cobra.Command, plan []evaluation.CampaignQueueModel, jsonOutput bool) error {
	if jsonOutput {
		return output.WriteJSON(cmd.OutOrStdout(), rolloutRunPlanJSON{Models: plan})
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Rollout plan (%d model(s))\n", len(plan)); err != nil {
		return err
	}
	for _, entry := range plan {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s) status=%s\n", entry.VariantID, entry.ServedModelTag, entry.Status); err != nil {
			return err
		}
	}
	return nil
}

func finishRolloutRun(cmd *cobra.Command, result rolloutRunResult) error {
	if output.JSONEnabled(cmd) {
		if err := output.WriteJSON(cmd.OutOrStdout(), result); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rollout finished: planned=%d succeeded=%d failed=%d\n", result.Planned, result.Succeeded, result.Failed)
		if result.LogDir != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logs: %s\n", result.LogDir)
		}
		for _, failure := range result.Failures {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Failed: %s (%s): %s\n", failure.VariantID, failure.Tag, failure.Error)
		}
	}
	if result.Failed > 0 {
		return fmt.Errorf("evaluation: rollout run: %d model(s) failed", result.Failed)
	}
	return nil
}

func preflightRollout(cmd *cobra.Command, ensembleHealthURL, mirrorBootstrapURL string) error {
	if err := checkHTTPReachable(cmd.Context(), ensembleHealthURL); err != nil {
		return fmt.Errorf("preflight ensemble health: %w", err)
	}
	if err := checkHTTPReachable(cmd.Context(), mirrorBootstrapURL); err != nil {
		return fmt.Errorf("preflight mirror bootstrap: %w", err)
	}
	if !output.JSONEnabled(cmd) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Preflight ok (platform health and mirror reachable)")
	}
	return nil
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

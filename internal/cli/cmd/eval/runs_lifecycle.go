// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// runStartFlowOptions describes one start of a campaign.
type runStartFlowOptions struct {
	CampaignID string
	// RunID names the run. When empty the run is named after the campaign and
	// the start time.
	RunID              string
	DryRun             bool
	PrepareOnly        bool
	Publish            bool
	Daemon             bool
	Verify             bool
	RequireObservation bool
	RequireProvenance  bool
	NoAutoBind         bool
	NoBackup           bool
	EnsembleURL        string
	FormationRunner    string
	JSONOutput         bool
}

type runStartFlowResult struct {
	CampaignID string
	RunID      string
	Lane       string
	Cells      uint64
	Models     []string
	Sessions   operatorSessions
	Prepared   bool
	Executed   int
	Report     *evalv1.EvaluationVerificationReport
}

type runStartPlanJSON struct {
	CampaignID          string   `json:"campaign_id"`
	RunID               string   `json:"run_id"`
	Lane                string   `json:"lane"`
	ModelTags           []string `json:"model_tags"`
	ModelRegistryDigest string   `json:"model_registry_digest"`
	CellCount           uint64   `json:"cell_count"`
	InferenceSession    string   `json:"inference_session"`
	DataSession         string   `json:"data_session"`
}

type runStartResultJSON struct {
	CampaignID     string   `json:"campaign_id"`
	RunID          string   `json:"run_id"`
	Executed       int      `json:"executed"`
	Verified       bool     `json:"verified"`
	VerifyStatus   string   `json:"verify_status"`
	FailureCount   int      `json:"failure_count"`
	FailureReasons []string `json:"failure_reasons"`
	Prepared       bool     `json:"prepared"`
}

func runStartPlan(result *runStartFlowResult, digest string) runStartPlanJSON {
	return runStartPlanJSON{
		CampaignID:          result.CampaignID,
		RunID:               result.RunID,
		Lane:                result.Lane,
		ModelTags:           result.Models,
		ModelRegistryDigest: digest,
		CellCount:           result.Cells,
		InferenceSession:    result.Sessions.InferenceSessionID,
		DataSession:         result.Sessions.DataSessionID,
	}
}

func writeRunStartPlan(cmd *cobra.Command, plan runStartPlanJSON, jsonOutput bool) error {
	if jsonOutput {
		return output.WriteJSON(cmd.OutOrStdout(), plan)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Run start plan\nCampaign: %s\nRun: %s\nLane: %s\nModels: %v\nCells: %d\nModel registry digest: %s\nInference session: %s\nData session: %s\n",
		plan.CampaignID, plan.RunID, plan.Lane, plan.ModelTags, plan.CellCount, plan.ModelRegistryDigest, plan.InferenceSession, plan.DataSession)
	return err
}

// runStartFlow starts one run of an existing campaign: it binds operator
// sessions, persists the run and its assignment matrix, and unless prepared
// only, executes and optionally verifies it.
func runStartFlow(cmd *cobra.Command, deps nativeEvalDeps, opts runStartFlowOptions) (*runStartFlowResult, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, err
	}
	store, archived, err := evaluation.LocateCampaign(cmd.Context(), fileSvc, opts.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	if archived {
		return nil, fmt.Errorf("evaluation: runs start: campaign %q: %w", opts.CampaignID, constants.ErrEvaluationArchived)
	}
	spec, err := store.LoadCampaignSpec(cmd.Context(), opts.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	laneName, stacks, err := campaignLane(cmd.Context(), store, opts.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	catalog, err := store.LoadScenarioCatalog(cmd.Context(), opts.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	sessions, err := resolveOperatorSessions(cmd, deps, operatorRoleInference, operatorRoleData)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	runID := opts.RunID
	if runID == "" {
		runID = fmt.Sprintf("%s-%d", opts.CampaignID, deps.now().UTC().Unix())
	}
	result := &runStartFlowResult{
		CampaignID: opts.CampaignID,
		RunID:      runID,
		Lane:       laneName,
		Cells:      campaignCells(catalog, spec, laneName, stacks),
		Sessions:   sessions,
	}
	for _, variant := range spec.GetModelRegistry() {
		result.Models = append(result.Models, variant.GetServedModelTag())
	}
	plan := runStartPlan(result, spec.GetModelRegistryDigest())
	if opts.DryRun {
		return result, writeRunStartPlan(cmd, plan, opts.JSONOutput)
	}
	if !opts.JSONOutput {
		if err := writeRunStartPlan(cmd, plan, false); err != nil {
			return nil, err
		}
	}
	authContext, err := deps.authLoader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: load CLI identity: %w", err)
	}
	lane := evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE
	if laneName == campaignLaneSystem {
		lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	}
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	if _, err := controller.StartRun(cmd.Context(), evaluation.RunStartRequest{
		CampaignID:                 opts.CampaignID,
		RunID:                      runID,
		InferenceOperatorSessionID: sessions.InferenceSessionID,
		DataOperatorSessionID:      sessions.DataSessionID,
		Deployment:                 nativeEvalDeployment(cfg, authContext, runID, ""),
		Lane:                       lane,
		PlatformRelease:            platformIdentity(cmd).Release,
	}); err != nil {
		return nil, fmt.Errorf("evaluation: runs start: %w", err)
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Started run %s of campaign %s\n", runID, opts.CampaignID)
	}
	if opts.Publish {
		publication, err := NewCampaignPublicationCoordinator(cmd, fileSvc)
		if err != nil {
			return nil, fmt.Errorf("evaluation: runs start: %w", err)
		}
		controller = controller.WithPublication(publication)
	}
	var assignments int
	if lane == evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM {
		assignments, err = controller.ScheduleHeterogeneousRun(cmd.Context(), runID)
	} else {
		assignments, err = controller.ScheduleHomogeneousRun(cmd.Context(), runID)
	}
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs start: schedule: %w", err)
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d assignments for run %s\n", assignments, runID)
	}
	if opts.PrepareOnly {
		result.Prepared = true
		return result, nil
	}
	if !opts.NoBackup {
		// Deferred so the snapshot also captures verification output and the
		// evidence of a run that failed or was cancelled.
		defer autoBackupEval(cmd, deps, opts.JSONOutput)
	}
	executed, err := executeRun(cmd, deps, runExecuteOptions{
		RunID:                    runID,
		Publish:                  opts.Publish,
		Daemon:                   opts.Daemon,
		EnsembleURL:              opts.EnsembleURL,
		EnforceProviderResidency: opts.RequireObservation || opts.RequireProvenance,
		NoAutoBind:               opts.NoAutoBind,
		FormationRunner:          opts.FormationRunner,
		JSONOutput:               opts.JSONOutput,
	})
	if err != nil {
		return result, err
	}
	result.Executed = executed
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %d assignment(s) for run %s\n", executed, runID)
	}
	if !opts.Verify {
		return result, nil
	}
	report, err := verifyRun(cmd, deps, store, runID, opts.RequireObservation, opts.RequireProvenance, opts.Publish, opts.JSONOutput)
	if err != nil {
		return result, err
	}
	result.Report = report
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run %s verification: %s (%d failure(s))\n", runID, report.GetStatus().String(), report.GetFailureCount())
		for _, reason := range report.GetFailureReasons() {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
		}
	}
	if report.GetFailureCount() > 0 {
		return result, constants.ErrEvalRunVerificationFailed
	}
	return result, nil
}

func runsStartCmd(deps nativeEvalDeps) *cobra.Command {
	var opts runStartFlowOptions
	var requireWitness bool
	cmd := &cobra.Command{
		Use:   "start <campaign>",
		Short: "Start a run of a campaign: bind sessions, schedule, execute, and optionally verify",
		Long: `Start one run of an existing campaign. The run binds the operator sessions,
persists its full assignment matrix, and then executes it. Progress is
resumable (g8e eval runs resume) and cancellable (g8e eval runs cancel).

Operator sessions are resolved from the active Inference Operator and the stack's
data-operator. The data-operator is bound to the CLI session unless --no-auto-bind
is set.

Examples:
  g8e eval runs start eval-qwen
  g8e eval runs start eval-qwen --require-witness
  g8e eval runs start eval-qwen --prepare-only
  g8e eval runs start eval-qwen --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.CampaignID = args[0]
			opts.JSONOutput = output.JSONEnabled(cmd)
			if requireWitness {
				opts.RequireObservation = true
				opts.RequireProvenance = true
				opts.Verify = true
			}
			result, err := runStartFlow(cmd, deps, opts)
			if err != nil {
				return err
			}
			if opts.DryRun {
				return nil
			}
			if opts.JSONOutput {
				return output.WriteJSON(cmd.OutOrStdout(), runStartResultJSON{
					CampaignID:     result.CampaignID,
					RunID:          result.RunID,
					Executed:       result.Executed,
					Verified:       result.Report != nil,
					VerifyStatus:   verificationStatusString(result.Report),
					FailureCount:   verificationFailureCount(result.Report),
					FailureReasons: verificationFailureReasons(result.Report),
					Prepared:       result.Prepared,
				})
			}
			if result.Prepared {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Prepared run %s. Execute it with:\n  ./g8e eval runs resume %s --publish --daemon\n", result.RunID, result.RunID)
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Print the resolved plan without starting")
	cmd.Flags().BoolVar(&opts.PrepareOnly, "prepare-only", false, "Persist the run and its assignments, then stop before executing")
	cmd.Flags().BoolVar(&opts.Publish, "publish", true, "Publish lifecycle projections to the public mirror")
	cmd.Flags().BoolVar(&opts.Daemon, "daemon", true, "Execute continuously until the assignment matrix is exhausted")
	cmd.Flags().BoolVar(&opts.Verify, "verify", false, "Verify the run after it completes")
	cmd.Flags().BoolVar(&requireWitness, "require-witness", false, "Require provider-boundary observation and model-provenance witness evidence (implies --verify)")
	cmd.Flags().BoolVar(&opts.RequireProvenance, "require-provenance", false, "Fail verification when model provenance attestation windows are missing")
	cmd.Flags().BoolVar(&opts.RequireObservation, "require-observation", false, "Fail verification when provider-boundary observation windows are missing")
	cmd.Flags().StringVar(&opts.EnsembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().BoolVar(&opts.NoAutoBind, "no-auto-bind", false, "Do not bind the data-operator to the CLI session when it is not bound")
	cmd.Flags().BoolVar(&opts.NoBackup, "no-backup", false, noBackupFlagUsage)
	cmd.Flags().StringVar(&opts.FormationRunner, "formation-runner", formationRunnerG8ee, formationRunnerFlagUsage)
	return cmd
}

const noBackupFlagUsage = "Do not back up evaluation evidence to eval/backups when the run finishes"

const formationRunnerFlagUsage = `Formation role execution: "g8ee" runs each role through the g8ee chat pipeline (graded, full transcripts); "direct" dispatches each role to the Inference Operator (ungraded). Both record storage attestation, observer windows, and peak VRAM`

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

type runResumeResult struct {
	AssignmentID string `json:"assignment_id"`
	Status       string `json:"status"`
	ResultDigest string `json:"result_digest"`
}

type runResumeJSON struct {
	RunID     string            `json:"run_id"`
	Executed  int               `json:"executed"`
	Remaining int64             `json:"remaining"`
	Results   []runResumeResult `json:"results"`
}

func runsResumeCmd(deps nativeEvalDeps) *cobra.Command {
	var limit uint32
	var ensembleURL, formationRunner string
	var noAutoBind, noBackup, publish, daemon bool
	cmd := &cobra.Command{
		Use:   "resume <run>",
		Short: "Execute the remaining queued assignments of a run",
		Long: `Execute queued assignments of an existing run through production POST
/api/v1/chat. By default one assignment runs; --limit raises that and --daemon
runs until the matrix is exhausted. An archived run cannot be resumed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			jsonOutput := output.JSONEnabled(cmd)
			results := make([]runResumeResult, 0)
			if !noBackup {
				defer autoBackupEval(cmd, deps, jsonOutput)
			}
			executed, err := executeRun(cmd, deps, runExecuteOptions{
				RunID:           runID,
				Publish:         publish,
				Daemon:          daemon,
				Limit:           limit,
				EnsembleURL:     ensembleURL,
				NoAutoBind:      noAutoBind,
				FormationRunner: formationRunner,
				JSONOutput:      jsonOutput,
				ResultOutput: func(result *evalv1.EvaluationAssignmentResult) {
					if jsonOutput && !daemon {
						results = append(results, runResumeResult{
							AssignmentID: result.GetAssignmentId(),
							Status:       result.GetLifecycleStatus().String(),
							ResultDigest: result.GetResultDigest(),
						})
					}
				},
			})
			if err != nil {
				return err
			}
			if !jsonOutput {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Executed %d assignment(s) for run %s\n", executed, runID)
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			summary, err := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() }).RunSummary(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs resume: %w", err)
			}
			remaining := int64(summary.ExpectedAssignment) - int64(summary.TerminalCount)
			if remaining < 0 {
				remaining = 0
			}
			return output.WriteJSON(cmd.OutOrStdout(), runResumeJSON{RunID: runID, Executed: executed, Remaining: remaining, Results: results})
		},
	}
	cmd.Flags().Uint32Var(&limit, "limit", 1, "Maximum queued assignments to execute in this invocation")
	cmd.Flags().BoolVar(&daemon, "daemon", false, "Run continuously until the queued matrix is exhausted")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish assignment lifecycle and terminal result projections to the public mirror")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().BoolVar(&noAutoBind, "no-auto-bind", false, "Do not bind the data-operator to the CLI session when it is not bound")
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, noBackupFlagUsage)
	cmd.Flags().StringVar(&formationRunner, "formation-runner", formationRunnerG8ee, formationRunnerFlagUsage)
	return cmd
}

type runCancelJSON struct {
	RunID        string `json:"run_id"`
	Stopped      int    `json:"stopped"`
	ClearedStale bool   `json:"cleared_stale_lease"`
	WasRunning   bool   `json:"was_running"`
}

func runsCancelCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <run>",
		Short: "Cancel a run: stop after the current assignment and stop the rest",
		Long: `Cancel a run. A run whose process is alive is asked to stop; cancel waits for
its current assignment to reach a terminal record and then stops every remaining
assignment. A lease whose process is gone is cleared and reported.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store, archived, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs cancel: %w", err)
			}
			if archived {
				return fmt.Errorf("evaluation: runs cancel: run %q: %w", runID, constants.ErrEvaluationArchived)
			}
			control, err := deps.runControl.process(fileSvc)
			if err != nil {
				return err
			}
			result := runCancelJSON{RunID: runID}
			jsonOutput := output.JSONEnabled(cmd)
			out := cmd.OutOrStdout()
			live := leaseLiveness(control)
			lease, err := store.LoadRunLease(cmd.Context(), runID)
			switch {
			case err == nil && live(*lease):
				result.WasRunning = true
				if err := awaitRunStop(cmd, deps, store, control, live, lease, jsonOutput); err != nil {
					return fmt.Errorf("evaluation: runs cancel: %w", err)
				}
			case err == nil:
				result.ClearedStale = true
				if err := store.ClearRunLease(cmd.Context(), runID); err != nil {
					return fmt.Errorf("evaluation: runs cancel: %w", err)
				}
				if !jsonOutput {
					_, _ = fmt.Fprintf(out, "Cleared stale lease held by process %d on %s\n", lease.PID, lease.Host)
				}
			case !isMissingRecord(err):
				return fmt.Errorf("evaluation: runs cancel: %w", err)
			}
			controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			if result.Stopped, err = controller.CancelRun(cmd.Context(), runID); err != nil {
				return fmt.Errorf("evaluation: runs cancel: %w", err)
			}
			if jsonOutput {
				return output.WriteJSON(out, result)
			}
			_, err = fmt.Fprintf(out, "Cancelled run %s: stopped %d assignment(s)\n", runID, result.Stopped)
			return err
		},
	}
}

// awaitRunStop asks the process holding a run's lease to stop and waits until
// it has released the lease, which happens once its current assignment reaches
// a terminal record.
func awaitRunStop(cmd *cobra.Command, deps nativeEvalDeps, store *evaluation.Store, control processControl, live evaluation.LeaseLiveness, lease *evaluation.RunLease, jsonOutput bool) error {
	if _, err := store.RequestRunCancel(cmd.Context(), lease.RunID, deps.now()); err != nil {
		return err
	}
	// Delivery is best effort: the cancel request in the lease reaches a process
	// that cannot be signalled.
	_ = control.Interrupt(lease.PID)
	if !jsonOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Waiting for process %d to finish its current assignment\n", lease.PID)
	}
	ticker := time.NewTicker(deps.runControl.interval())
	defer ticker.Stop()
	for {
		current, err := store.LoadRunLease(cmd.Context(), lease.RunID)
		if err != nil {
			if isMissingRecord(err) {
				return nil
			}
			return err
		}
		if !live(*current) {
			return store.ClearRunLease(cmd.Context(), lease.RunID)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-ticker.C:
		}
	}
}

func runsLogsCmd(deps nativeEvalDeps) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <run>",
		Short: "Print a run's execution log",
		Long: `Print the execution log of a run: the live log while a process holds the run,
otherwise its most recent one. With --follow, keep printing until the run's
process exits.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			control, err := deps.runControl.process(fileSvc)
			if err != nil {
				return err
			}
			store, archived, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs logs: %w", err)
			}
			live := leaseLiveness(control)
			logPath := ""
			holding := false
			if !archived {
				if lease, err := store.LoadRunLease(cmd.Context(), runID); err == nil && live(*lease) {
					logPath, holding = lease.LogPath, true
				}
			}
			if logPath == "" {
				if logPath, err = latestRunLogPath(cmd.Context(), fileSvc, runID); err != nil {
					return fmt.Errorf("evaluation: runs logs: %w", err)
				}
			}
			if logPath == "" {
				return fmt.Errorf("evaluation: runs logs: no log recorded for run %q: %w", runID, constants.ErrNotFound)
			}
			file, err := fileSvc.OpenForRead(cmd.Context(), logPath)
			if err != nil {
				return fmt.Errorf("evaluation: runs logs: %w", err)
			}
			defer file.Close()
			out := cmd.OutOrStdout()
			if _, err := io.Copy(out, file); err != nil {
				return fmt.Errorf("evaluation: runs logs: %w", err)
			}
			if !follow || !holding {
				return nil
			}
			return followRunLog(cmd, deps, store, runID, live, file)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Keep printing until the run's process exits")
	return noJSON(cmd)
}

// followRunLog prints appended log output until the run's lease is gone, then
// prints whatever the process wrote last.
func followRunLog(cmd *cobra.Command, deps nativeEvalDeps, store *evaluation.Store, runID string, live evaluation.LeaseLiveness, file io.Reader) error {
	ticker := time.NewTicker(deps.runControl.interval())
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-ticker.C:
		}
		if _, err := io.Copy(cmd.OutOrStdout(), file); err != nil {
			return fmt.Errorf("evaluation: runs logs: %w", err)
		}
		lease, err := store.LoadRunLease(cmd.Context(), runID)
		if err != nil && !isMissingRecord(err) {
			return fmt.Errorf("evaluation: runs logs: %w", err)
		}
		if err != nil || !live(*lease) {
			_, err := io.Copy(cmd.OutOrStdout(), file)
			return err
		}
	}
}

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
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// normalizeRuntimeEvalPath accepts a runtime-relative path with or without the
// leading runtime directory.
func normalizeRuntimeEvalPath(rawPath string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rawPath)), constants.RuntimeDirname+"/")
}

func runsEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Start, follow, verify, and publish campaign runs",
		Long: `A run is one execution of a campaign. Runs have a lifecycle: start, resume,
cancel, verify, publish, and export, and they can be archived.`,
	}
	addLeaves(cmd,
		runsListCmd(deps),
		runsShowCmd(deps),
		runsAssignmentsCmd(deps),
		runsStartCmd(deps),
		runsResumeCmd(deps),
		runsCancelCmd(deps),
		runsLogsCmd(deps),
		runsVerifyCmd(deps),
		runsPublishCmd(deps),
		runsExportCmd(deps),
		runsRepairCmd(deps),
		runsCompareCmd(deps),
		runsArchiveCmd(deps),
		runsUnarchiveCmd(deps),
	)
	return cmd
}

type runListJSON struct {
	Runs []runRow `json:"runs"`
}

func runsListCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignFilter, statusFilter string
	var includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs",
		Long: `List runs with their status and progress.

Archived runs are hidden unless --archived is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			control, err := deps.runControl.process(fileSvc)
			if err != nil {
				return err
			}
			runIDs, err := evaluation.NewStore(fileSvc).ListRunIDs(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: runs list: %w", err)
			}
			if includeArchived {
				archivedIDs, err := evaluation.NewArchivedStore(fileSvc).ListRunIDs(cmd.Context())
				if err != nil {
					return fmt.Errorf("evaluation: runs list: %w", err)
				}
				runIDs = append(runIDs, archivedIDs...)
			}
			sort.Strings(runIDs)
			rows, err := campaignRunRows(cmd.Context(), fileSvc, runIDs, leaseLiveness(control), deps)
			if err != nil {
				return fmt.Errorf("evaluation: runs list: %w", err)
			}
			selected := rows[:0:0]
			for _, row := range rows {
				if campaignFilter != "" && row.CampaignID != campaignFilter {
					continue
				}
				if statusFilter != "" && row.Status != statusFilter {
					continue
				}
				selected = append(selected, row)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), runListJSON{Runs: selected})
			}
			if len(selected) == 0 {
				cmd.Println("No runs found")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "RUN\tCAMPAIGN\tSTATUS\tPROGRESS\tLANE\tARCHIVED")
			for _, row := range selected {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%s\t%t\n", row.RunID, row.CampaignID, row.Status, row.Terminal, row.ExpectedAssignments, row.Lane, row.Archived)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&campaignFilter, "campaign", "", "Only runs of this campaign")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Only runs with this status (for example running, completed, verified, interrupted)")
	cmd.Flags().BoolVar(&includeArchived, "archived", false, "Include archived runs")
	return cmd
}

type runShowJSON struct {
	runRow
	ModelCount          int    `json:"model_count"`
	ScenarioCount       uint32 `json:"scenario_count"`
	RepetitionCount     uint32 `json:"repetition_count"`
	ModelRegistryDigest string `json:"model_registry_digest"`
	CatalogDigest       string `json:"catalog_digest"`
	InferenceSession    string `json:"inference_session,omitempty"`
	DataSession         string `json:"data_session,omitempty"`
}

func loadRunShow(ctx context.Context, deps nativeEvalDeps, store *evaluation.Store, archived bool, runID string, live evaluation.LeaseLiveness) (*runShowJSON, error) {
	row, summary, err := summarizeRun(ctx, store, archived, runID, live, deps.now, adaptNewID(deps.newID))
	if err != nil {
		return nil, err
	}
	binding := summary.Run.GetCampaignBinding()
	spec, err := store.LoadCampaignSpec(ctx, binding.GetCampaignId())
	if err != nil {
		return nil, err
	}
	return &runShowJSON{
		runRow:              *row,
		ModelCount:          len(spec.GetModelRegistry()),
		ScenarioCount:       spec.GetScenarioCount(),
		RepetitionCount:     spec.GetRepetitionCount(),
		ModelRegistryDigest: spec.GetModelRegistryDigest(),
		CatalogDigest:       spec.GetCatalogDigest(),
		InferenceSession:    binding.GetInferenceOperatorSessionId(),
		DataSession:         binding.GetDataOperatorSessionId(),
	}, nil
}

func writeRunShow(cmd *cobra.Command, show *runShowJSON) error {
	out := cmd.OutOrStdout()
	_, err := fmt.Fprintf(out, "Run: %s\nCampaign: %s\nStatus: %s\nLane: %s\nStarted: %s\nModels: %d\nScenarios: %d\nRepetitions: %d\nModel registry digest: %s\nCatalog digest: %s\nExpected assignments: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nStopped: %d\nNext assignment: %s\nArchived: %t\n",
		show.RunID, show.CampaignID, show.Status, show.Lane, show.StartedAt, show.ModelCount, show.ScenarioCount, show.RepetitionCount,
		show.ModelRegistryDigest, show.CatalogDigest, show.ExpectedAssignments, show.Queued, show.Running, show.Terminal, show.Stopped, show.NextAssignmentID, show.Archived)
	if err != nil {
		return err
	}
	if show.Holder != nil {
		_, err = fmt.Fprintf(out, "Held by: process %d on %s since %s\n", show.Holder.PID, show.Holder.Host, show.Holder.StartedAt)
	}
	return err
}

func runsShowCmd(deps nativeEvalDeps) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "show <run>",
		Short: "Show one run's status and progress",
		Long: `Show one run. The status is "running" only while a live process holds the
run's lease. A run left running with no live process reports "interrupted".

With --watch, keep reporting progress until the run reaches a settled status.`,
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
			live := leaseLiveness(control)
			store, archived, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs show: %w", err)
			}
			show, err := loadRunShow(cmd.Context(), deps, store, archived, runID, live)
			if err != nil {
				return fmt.Errorf("evaluation: runs show: %w", err)
			}
			if output.JSONEnabled(cmd) && !watch {
				return output.WriteJSON(cmd.OutOrStdout(), show)
			}
			if !output.JSONEnabled(cmd) {
				if err := writeRunShow(cmd, show); err != nil {
					return err
				}
			}
			if !watch {
				return nil
			}
			return watchRun(cmd, deps, store, archived, runID, live, show)
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "Keep reporting progress until the run settles")
	return cmd
}

func watchRun(cmd *cobra.Command, deps nativeEvalDeps, store *evaluation.Store, archived bool, runID string, live evaluation.LeaseLiveness, last *runShowJSON) error {
	emit := func(show *runShowJSON) error {
		if output.JSONEnabled(cmd) {
			body, err := json.Marshal(show)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %d/%d terminal  %d queued  %d running\n",
			deps.now().UTC().Format("15:04:05"), show.Status, show.Terminal, show.ExpectedAssignments, show.Queued, show.Running)
		return err
	}
	if output.JSONEnabled(cmd) {
		if err := emit(last); err != nil {
			return err
		}
	}
	for !terminalRunStatus(last.Status) {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-time.After(deps.runControl.interval()):
		}
		show, err := loadRunShow(cmd.Context(), deps, store, archived, runID, live)
		if err != nil {
			return fmt.Errorf("evaluation: runs show: %w", err)
		}
		if err := emit(show); err != nil {
			return err
		}
		last = show
	}
	return nil
}

type runPublishJSON struct {
	RunID            string `json:"run_id"`
	PublishedRecords int    `json:"published_records"`
	Force            bool   `json:"force"`
}

func runsPublishCmd(deps nativeEvalDeps) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "publish <run>",
		Short: "Publish missing public lifecycle projections for one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			if err := evaluation.RejectArchivedRun(cmd.Context(), fileSvc, runID); err != nil {
				return fmt.Errorf("evaluation: runs publish: %w", err)
			}
			publication, err := NewCampaignPublicationCoordinator(cmd, fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: runs publish: %w", err)
			}
			report, reportErr := evaluation.NewStore(fileSvc).LoadCampaignVerification(cmd.Context(), runID)
			if reportErr != nil && !isMissingRecord(reportErr) {
				return fmt.Errorf("evaluation: runs publish: load verification report: %w", reportErr)
			}
			if force {
				if err := publication.ResetPublicationIdempotency(cmd.Context(), runID); err != nil {
					return fmt.Errorf("evaluation: runs publish: %w", err)
				}
			}
			count, err := publication.PublishRunCatchUpWithVerification(cmd.Context(), runID, report)
			if err != nil {
				return fmt.Errorf("evaluation: runs publish: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), runPublishJSON{RunID: runID, PublishedRecords: count, Force: force})
			}
			suffix := ""
			if force {
				suffix = " (forced republish)"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d public projection record(s) for run %s%s\n", count, runID, suffix)
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Clear host publication idempotency and republish all projections (use after a gateway mirror volume wipe)")
	return cmd
}

type runExportJSON struct {
	RunID               string                          `json:"run_id"`
	CampaignID          string                          `json:"campaign_id"`
	OutputDir           string                          `json:"output_dir"`
	ExportedAt          string                          `json:"exported_at"`
	AssignmentCount     uint32                          `json:"assignment_count"`
	TerminalResultCount uint32                          `json:"terminal_result_count"`
	Files               []evaluation.CampaignExportFile `json:"files"`
}

func runsExportCmd(deps nativeEvalDeps) *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "export <run>",
		Short: "Generate disclosure-safe JSONL, CSV, and SQLite exports for one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store, _, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs export: %w", err)
			}
			report, err := evaluation.NewCampaignExporter(deps.now).ExportRun(cmd.Context(), store, fileSvc, runID, normalizeRuntimeEvalPath(outputDir))
			if err != nil {
				return fmt.Errorf("evaluation: runs export: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), runExportJSON{
					RunID:               report.RunID,
					CampaignID:          report.CampaignID,
					OutputDir:           report.OutputDir,
					ExportedAt:          report.ExportedAt.Format(time.RFC3339),
					AssignmentCount:     report.AssignmentCount,
					TerminalResultCount: report.TerminalResultCount,
					Files:               report.Files,
				})
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Exported run %s to %s\nAssignments: %d scheduled, %d terminal results\n",
				report.RunID, report.OutputDir, report.AssignmentCount, report.TerminalResultCount)
			for _, file := range report.Files {
				_, _ = fmt.Fprintf(out, "- %s (%s, %d record(s))\n", file.Name, file.Format, file.RecordCount)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Runtime-relative directory to write the export into")
	_ = cmd.MarkFlagRequired("output-dir")
	return cmd
}

type runRepairJSON struct {
	RunID   string `json:"run_id"`
	Count   int    `json:"repaired_trace_digests,omitempty"`
	Results int    `json:"repaired_results,omitempty"`
}

func runsRepairCmd(deps nativeEvalDeps) *cobra.Command {
	var results, traceDigests bool
	cmd := &cobra.Command{
		Use:   "repair <run>",
		Short: "Backfill or recompute persisted assignment records for one run",
		Long: `Repair one run's persisted records. Pass exactly one of:

  --results        backfill terminal results for assignments missing result records
  --trace-digests  recompute chat-probe trace digests using canonical g8e JSON`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			if results == traceDigests {
				return fmt.Errorf("evaluation: runs repair: pass exactly one of --results or --trace-digests: %w", constants.ErrEvaluationFlagsInvalid)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			if err := evaluation.RejectArchivedRun(cmd.Context(), fileSvc, runID); err != nil {
				return fmt.Errorf("evaluation: runs repair: %w", err)
			}
			store := evaluation.NewStore(fileSvc)
			controller := evaluation.NewCampaignController(store, nil, deps.now, adaptNewID(deps.newID))
			payload := runRepairJSON{RunID: runID}
			var message string
			if results {
				run, err := store.LoadRun(cmd.Context(), runID)
				if err != nil {
					return fmt.Errorf("evaluation: runs repair: %w", err)
				}
				catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
				if err != nil {
					return fmt.Errorf("evaluation: runs repair: %w", err)
				}
				artifacts, err := store.LoadScenarioArtifacts(cmd.Context(), run.GetCampaignBinding().GetCampaignId(), catalog)
				if err != nil {
					return fmt.Errorf("evaluation: runs repair: %w", err)
				}
				if payload.Results, err = controller.RepairAssignmentsWithoutResults(cmd.Context(), runID, artifacts); err != nil {
					return fmt.Errorf("evaluation: runs repair: %w", err)
				}
				message = fmt.Sprintf("Repaired %d terminal assignment result(s) for run %s\n", payload.Results, runID)
			} else {
				if payload.Count, err = controller.RepairAssignmentTraceDigests(cmd.Context(), runID); err != nil {
					return fmt.Errorf("evaluation: runs repair: %w", err)
				}
				message = fmt.Sprintf("Repaired %d assignment trace digest(s) for run %s\n", payload.Count, runID)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), message)
			return err
		},
	}
	cmd.Flags().BoolVar(&results, "results", false, "Backfill terminal results for assignments missing result records")
	cmd.Flags().BoolVar(&traceDigests, "trace-digests", false, "Recompute chat-probe trace digests")
	return cmd
}

// runCell is one cell of a run's matrix reduced to what compare reports.
type runCell struct {
	Status  string  `json:"status"`
	Verdict string  `json:"verdict,omitempty"`
	Passed  bool    `json:"passed"`
	Score   float64 `json:"score"`
}

// invalidEvidence reports a cell whose grades measure the harness, not the
// model. It is shown, never counted for or against the model.
func (c runCell) invalidEvidence() bool { return c.Verdict == invalidEvidenceVerdict }

const (
	verdictStatusPrefix    = "EVALUATION_VERDICT_STATUS_"
	invalidEvidenceVerdict = "invalid_evidence"
)

// cellFromResult reads one terminal result's outcome from the same verdict and
// pass rate every other surface publishes; it never re-tallies grades.
func cellFromResult(result *evalv1.EvaluationAssignmentResult) runCell {
	verdict := evaluation.DerivePublicSummaryStatus(result)
	cell := runCell{
		Status:  strings.ToLower(strings.TrimPrefix(result.GetLifecycleStatus().String(), "EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_")),
		Verdict: strings.ToLower(strings.TrimPrefix(verdict.String(), verdictStatusPrefix)),
		Passed:  verdict == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
	}
	if rate, ok := evaluation.DeterministicPassRate(result); ok {
		cell.Score = rate
	}
	return cell
}

type runCompareSide struct {
	RunID           string  `json:"run_id"`
	CampaignID      string  `json:"campaign_id"`
	Cells           int     `json:"cells"`
	Completed       int     `json:"completed"`
	Passed          int     `json:"passed"`
	InvalidEvidence int     `json:"invalid_evidence"`
	MeanScore       float64 `json:"mean_score"`
}

type runCompareChange struct {
	Cell  string  `json:"cell"`
	Kind  string  `json:"kind"`
	Left  runCell `json:"left"`
	Right runCell `json:"right"`
}

type runCompareJSON struct {
	Left         runCompareSide     `json:"left"`
	Right        runCompareSide     `json:"right"`
	OnlyInLeft   []string           `json:"only_in_left"`
	OnlyInRight  []string           `json:"only_in_right"`
	Regressions  int                `json:"regressions"`
	Improvements int                `json:"improvements"`
	Changes      []runCompareChange `json:"changes"`
}

// runCellKey names a matrix cell independently of the run it belongs to.
func runCellKey(assignment *evalv1.EvaluationAssignment) string {
	return fmt.Sprintf("%s | %s | rep %d", assignment.GetScenarioId(), assignmentTarget(assignment), assignment.GetRepetition())
}

// assignmentTarget names what an assignment exercises: `<served model>/<role>`
// for one model in one role, `stack:<id>` for a heterogeneous formation.
func assignmentTarget(assignment *evalv1.EvaluationAssignment) string {
	switch t := assignment.GetTarget().(type) {
	case *evalv1.EvaluationAssignment_Homogeneous:
		return fmt.Sprintf("%s/%s", t.Homogeneous.GetCandidateVariant().GetServedModelTag(), assignmentRoleLabel(t.Homogeneous.GetDesignatedRole()))
	case *evalv1.EvaluationAssignment_Heterogeneous:
		return "stack:" + t.Heterogeneous.GetStack().GetStackId()
	default:
		return "unknown"
	}
}

func assignmentRoleLabel(role evalv1.ModelCampaignRole) string {
	return strings.ToLower(strings.TrimPrefix(role.String(), "MODEL_CAMPAIGN_ROLE_"))
}

// executedAssignmentLine is the execution-log line for one finished assignment:
// what ran (scenario, target, repetition), how long it took, and its outcome.
func executedAssignmentLine(assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) string {
	took := ""
	if elapsed, ok := assignmentDuration(assignment, result); ok {
		took = ", " + elapsed.Round(100*time.Millisecond).String()
	}
	return fmt.Sprintf("Executed %s [%s%s]: %s", result.GetAssignmentId(), runCellKey(assignment), took, evaluation.AssignmentOutcomeSummary(result))
}

// assignmentDuration is the wall-clock time from start to completion, when both
// were recorded.
func assignmentDuration(assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) (time.Duration, bool) {
	started := assignment.GetStartedAt()
	completed := result.GetCompletedAt()
	if started == nil || completed == nil {
		return 0, false
	}
	return completed.AsTime().Sub(started.AsTime()), true
}

func loadRunCells(ctx context.Context, store *evaluation.Store, runID string) (map[string]runCell, error) {
	assignments, err := store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, err
	}
	cells := make(map[string]runCell, len(assignments))
	for _, assignment := range assignments {
		cell := runCell{Status: strings.ToLower(strings.TrimPrefix(assignment.GetLifecycleStatus().String(), "EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_"))}
		exists, err := store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		if exists {
			result, err := store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
			if err != nil {
				return nil, err
			}
			cell = cellFromResult(result)
		}
		cells[runCellKey(assignment)] = cell
	}
	return cells, nil
}

func summarizeRunCells(runID, campaignID string, cells map[string]runCell) runCompareSide {
	side := runCompareSide{RunID: runID, CampaignID: campaignID, Cells: len(cells)}
	var total float64
	for _, cell := range cells {
		if cell.Status == "completed" {
			side.Completed++
		}
		if cell.Passed {
			side.Passed++
		}
		if cell.invalidEvidence() {
			side.InvalidEvidence++
			continue
		}
		total += cell.Score
	}
	if measured := side.Cells - side.InvalidEvidence; measured > 0 {
		side.MeanScore = total / float64(measured)
	}
	return side
}

// compareRunCells reports what changed from left to right.
func compareRunCells(left, right map[string]runCell) (onlyLeft, onlyRight []string, changes []runCompareChange) {
	for key, l := range left {
		r, ok := right[key]
		if !ok {
			onlyLeft = append(onlyLeft, key)
			continue
		}
		switch {
		case l.invalidEvidence() || r.invalidEvidence():
			if l != r {
				changes = append(changes, runCompareChange{Cell: key, Kind: "changed", Left: l, Right: r})
			}
		case l.Passed && !r.Passed:
			changes = append(changes, runCompareChange{Cell: key, Kind: "regression", Left: l, Right: r})
		case !l.Passed && r.Passed:
			changes = append(changes, runCompareChange{Cell: key, Kind: "improvement", Left: l, Right: r})
		case l.Status != r.Status || l.Score != r.Score:
			changes = append(changes, runCompareChange{Cell: key, Kind: "changed", Left: l, Right: r})
		}
	}
	for key := range right {
		if _, ok := left[key]; !ok {
			onlyRight = append(onlyRight, key)
		}
	}
	sort.Strings(onlyLeft)
	sort.Strings(onlyRight)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Cell < changes[j].Cell })
	return onlyLeft, onlyRight, changes
}

func runsCompareCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "compare <run> <run>",
		Short: "Compare two runs cell by cell",
		Long: `Compare two runs over the matrix cells they share. Reports per-run totals and
every cell whose outcome or deterministic score differs. A cell that passed in
the first run and did not in the second is a regression. Archived runs compare
like any other.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			sides := make([]runCompareSide, 0, 2)
			cellSets := make([]map[string]runCell, 0, 2)
			for _, runID := range args {
				store, _, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
				if err != nil {
					return fmt.Errorf("evaluation: runs compare: %w", err)
				}
				run, err := store.LoadRun(cmd.Context(), runID)
				if err != nil {
					return fmt.Errorf("evaluation: runs compare: %w", err)
				}
				cells, err := loadRunCells(cmd.Context(), store, runID)
				if err != nil {
					return fmt.Errorf("evaluation: runs compare: %w", err)
				}
				cellSets = append(cellSets, cells)
				sides = append(sides, summarizeRunCells(runID, run.GetCampaignBinding().GetCampaignId(), cells))
			}
			onlyLeft, onlyRight, changes := compareRunCells(cellSets[0], cellSets[1])
			payload := runCompareJSON{Left: sides[0], Right: sides[1], OnlyInLeft: onlyLeft, OnlyInRight: onlyRight, Changes: changes}
			if payload.OnlyInLeft == nil {
				payload.OnlyInLeft = []string{}
			}
			if payload.OnlyInRight == nil {
				payload.OnlyInRight = []string{}
			}
			if payload.Changes == nil {
				payload.Changes = []runCompareChange{}
			}
			for _, change := range changes {
				switch change.Kind {
				case "regression":
					payload.Regressions++
				case "improvement":
					payload.Improvements++
				}
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			for _, side := range sides {
				_, _ = fmt.Fprintf(out, "%s (%s): %d cells, %d completed, %d passed, %d invalid evidence, mean score %.3f\n", side.RunID, side.CampaignID, side.Cells, side.Completed, side.Passed, side.InvalidEvidence, side.MeanScore)
			}
			_, _ = fmt.Fprintf(out, "Regressions: %d  Improvements: %d  Only in %s: %d  Only in %s: %d\n",
				payload.Regressions, payload.Improvements, args[0], len(onlyLeft), args[1], len(onlyRight))
			if len(changes) == 0 {
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "CELL\tKIND\tLEFT\tRIGHT")
			for _, change := range changes {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s %.3f\t%s %.3f\n", change.Cell, change.Kind, change.Left.Status, change.Left.Score, change.Right.Status, change.Right.Score)
			}
			return w.Flush()
		},
	}
}

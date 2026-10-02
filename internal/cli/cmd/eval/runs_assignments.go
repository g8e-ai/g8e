// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// runAssignmentJSON is one assignment of a run. `assignment` and `result` are
// canonical protojson from the one protocol encoder; `verdict`, `pass_rate`, and
// `outcome` state what the platform's own verdict and scoring say, so a zero rate
// is a stated 0 and an absent score is a stated null, never a dropped field.
type runAssignmentJSON struct {
	AssignmentID string          `json:"assignment_id"`
	ScenarioID   string          `json:"scenario_id"`
	Target       string          `json:"target"`
	Repetition   uint32          `json:"repetition"`
	Lifecycle    string          `json:"lifecycle"`
	Verdict      string          `json:"verdict,omitempty"`
	PassRate     *float64        `json:"pass_rate"`
	DurationMs   *int64          `json:"duration_ms"`
	Outcome      string          `json:"outcome,omitempty"`
	Assignment   json.RawMessage `json:"assignment"`
	Result       json.RawMessage `json:"result,omitempty"`
}

type runAssignmentsJSON struct {
	RunID       string              `json:"run_id"`
	Assignments []runAssignmentJSON `json:"assignments"`
}

// runAssignmentView is one assignment with its terminal result, when it has one.
type runAssignmentView struct {
	assignment *evalv1.EvaluationAssignment
	result     *evalv1.EvaluationAssignmentResult
}

// passed reports a terminal result whose verdict is PASS.
func (v runAssignmentView) passed() bool {
	return v.result != nil && evaluation.DerivePublicSummaryStatus(v.result) == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
}

// selectRunAssignments applies the `--failed` and `--assignment` filters.
// `--failed` keeps terminal results that did not pass, whether the model failed
// or the evidence was invalid, so every one is listed with its cause.
func selectRunAssignments(views []runAssignmentView, failedOnly bool, assignmentID string) ([]runAssignmentView, error) {
	selected := make([]runAssignmentView, 0, len(views))
	for _, view := range views {
		if assignmentID != "" && view.assignment.GetAssignmentId() != assignmentID {
			continue
		}
		if failedOnly && (view.result == nil || view.passed()) {
			continue
		}
		selected = append(selected, view)
	}
	if assignmentID != "" && len(selected) == 0 {
		return nil, fmt.Errorf("evaluation: runs assignments: assignment %q is not in this run or does not match --failed: %w", assignmentID, constants.ErrNotFound)
	}
	return selected, nil
}

func (v runAssignmentView) toJSON() (runAssignmentJSON, error) {
	assignmentBody, err := evalv1.MarshalCanonical(v.assignment)
	if err != nil {
		return runAssignmentJSON{}, err
	}
	row := runAssignmentJSON{
		AssignmentID: v.assignment.GetAssignmentId(),
		ScenarioID:   v.assignment.GetScenarioId(),
		Target:       assignmentTarget(v.assignment),
		Repetition:   v.assignment.GetRepetition(),
		Lifecycle:    strings.ToLower(strings.TrimPrefix(v.assignment.GetLifecycleStatus().String(), "EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_")),
		Assignment:   assignmentBody,
	}
	if v.result == nil {
		return row, nil
	}
	resultBody, err := evalv1.MarshalCanonical(v.result)
	if err != nil {
		return runAssignmentJSON{}, err
	}
	row.Result = resultBody
	row.Verdict = strings.ToLower(strings.TrimPrefix(evaluation.DerivePublicSummaryStatus(v.result).String(), verdictStatusPrefix))
	row.Outcome = evaluation.AssignmentOutcomeSummary(v.result)
	if rate, ok := evaluation.DeterministicPassRate(v.result); ok {
		row.PassRate = &rate
	}
	if elapsed, ok := assignmentDuration(v.assignment, v.result); ok {
		millis := elapsed.Milliseconds()
		row.DurationMs = &millis
	}
	return row, nil
}

// loadRunAssignmentViews pairs every assignment of a run with its result.
func loadRunAssignmentViews(cmd *cobra.Command, store *evaluation.Store, runID string) ([]runAssignmentView, error) {
	assignments, err := store.ListAssignments(cmd.Context(), runID)
	if err != nil {
		return nil, err
	}
	results, err := store.LoadAssignmentResults(cmd.Context(), runID, assignments)
	if err != nil {
		return nil, err
	}
	views := make([]runAssignmentView, 0, len(assignments))
	for _, assignment := range assignments {
		views = append(views, runAssignmentView{assignment: assignment, result: results[assignment.GetAssignmentId()]})
	}
	return views, nil
}

func writeRunAssignments(cmd *cobra.Command, views []runAssignmentView, detail bool) error {
	out := cmd.OutOrStdout()
	if len(views) == 0 {
		_, err := fmt.Fprintln(out, "No assignments found")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ASSIGNMENT\tSCENARIO\tTARGET\tREP\tDURATION\tOUTCOME")
	for _, view := range views {
		duration := "-"
		outcome := strings.ToLower(strings.TrimPrefix(view.assignment.GetLifecycleStatus().String(), "EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_"))
		if view.result != nil {
			outcome = evaluation.AssignmentOutcomeSummary(view.result)
			if elapsed, ok := assignmentDuration(view.assignment, view.result); ok {
				duration = elapsed.Round(time.Millisecond).String()
			}
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", view.assignment.GetAssignmentId(), view.assignment.GetScenarioId(), assignmentTarget(view.assignment), view.assignment.GetRepetition(), duration, outcome)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if !detail {
		return nil
	}
	for _, view := range views {
		if view.result == nil {
			continue
		}
		if err := writeAssignmentGrades(cmd, view.result); err != nil {
			return err
		}
	}
	return nil
}

// writeAssignmentGrades prints every deterministic grade of one result with its
// basis, so an operator sees the whole chain: the observations that counted, the
// derived grades that restate them, and the structural preconditions.
func writeAssignmentGrades(cmd *cobra.Command, result *evalv1.EvaluationAssignmentResult) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "\nGrades for %s\n", result.GetAssignmentId()); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CRITERION\tBASIS\tSTATUS\tDETAIL")
	for _, grade := range result.GetDeterministicGrades() {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", grade.GetCriterionId(), evaluation.GradeBasisLabel(grade.GetBasis()), strings.ToLower(strings.TrimPrefix(grade.GetStatus().String(), verdictStatusPrefix)), grade.GetDetail())
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if reason := result.GetFailureReason(); reason != "" {
		_, err := fmt.Fprintf(out, "Failure reason: %s\n", reason)
		return err
	}
	return nil
}

func runsAssignmentsCmd(deps nativeEvalDeps) *cobra.Command {
	var failedOnly bool
	var assignmentID string
	cmd := &cobra.Command{
		Use:   "assignments <run>",
		Short: "List one run's assignments with their verdicts and failed grades",
		Long: `List every assignment of a run with its scenario, target (model and role, or
stack), repetition, duration, verdict, deterministic pass rate, and each failed
grade with the cause it recorded, from the same verdict and scoring every other
surface publishes.

--failed keeps the assignments that did not pass: the model failed, or the
evidence was invalid and was not counted against the model.

--assignment shows one assignment with every grade and its basis (observation,
derived, structural). With --json the assignment and its result are canonical
protojson, with the verdict and pass rate stated beside them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store, _, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs assignments: %w", err)
			}
			views, err := loadRunAssignmentViews(cmd, store, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs assignments: %w", err)
			}
			selected, err := selectRunAssignments(views, failedOnly, assignmentID)
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				payload := runAssignmentsJSON{RunID: runID, Assignments: make([]runAssignmentJSON, 0, len(selected))}
				for _, view := range selected {
					row, err := view.toJSON()
					if err != nil {
						return fmt.Errorf("evaluation: runs assignments: %w", err)
					}
					payload.Assignments = append(payload.Assignments, row)
				}
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			return writeRunAssignments(cmd, selected, assignmentID != "")
		},
	}
	cmd.Flags().BoolVar(&failedOnly, "failed", false, "Only assignments that did not pass")
	cmd.Flags().StringVar(&assignmentID, "assignment", "", "Only this assignment, with every grade and its basis")
	return cmd
}

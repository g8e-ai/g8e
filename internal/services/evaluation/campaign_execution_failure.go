// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// BuildExecutorFailureAssignmentResult materializes one terminal assignment
// result when production chat execution fails before a canonical trace import.
func BuildExecutorFailureAssignmentResult(req AssignmentExecutionRequest, execErr error, now time.Time, newID func(string) string) (*evalv1.EvaluationAssignmentResult, error) {
	if req.Assignment == nil || execErr == nil {
		return nil, fmt.Errorf("evaluation: build executor failure assignment result: %w", constants.ErrMissingRequiredField)
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lifecycle := classifyExecutorFailureLifecycle(execErr)
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    req.Assignment.GetAssignmentId(),
		RunId:           req.Assignment.GetRunId(),
		CampaignId:      req.Assignment.GetCampaignId(),
		Lane:            req.Assignment.GetLane(),
		LifecycleStatus: lifecycle,
		DeterministicGrades: []*evalv1.DeterministicGrade{
			{
				GradeId:     req.Assignment.GetAssignmentId() + ":execution-failure",
				CriterionId: "execution-failure",
				Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
				Detail:      execErr.Error(),
			},
		},
		CompletedAt: timestamppb.New(now),
	}
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

// BuildRecoveredTerminalAssignmentResult materializes one terminal result for
// an assignment that reached a terminal lifecycle without a persisted result.
func BuildRecoveredTerminalAssignmentResult(assignment *evalv1.EvaluationAssignment, now time.Time) (*evalv1.EvaluationAssignmentResult, error) {
	if assignment == nil || !isTerminalAssignmentLifecycle(assignment.GetLifecycleStatus()) {
		return nil, fmt.Errorf("evaluation: build recovered terminal assignment result: %w", constants.ErrMissingRequiredField)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	completedAt := assignment.GetCompletedAt()
	if completedAt == nil {
		completedAt = timestamppb.New(now)
	}
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    assignment.GetAssignmentId(),
		RunId:           assignment.GetRunId(),
		CampaignId:      assignment.GetCampaignId(),
		Lane:            assignment.GetLane(),
		LifecycleStatus: assignment.GetLifecycleStatus(),
		DeterministicGrades: []*evalv1.DeterministicGrade{
			{
				GradeId:     assignment.GetAssignmentId() + ":recovered-terminal",
				CriterionId: "recovered-terminal",
				Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
				Detail:      "recovered terminal assignment lifecycle without a persisted result record",
			},
		},
		CompletedAt: completedAt,
	}
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

func classifyExecutorFailureLifecycle(execErr error) evalv1.EvaluationAssignmentLifecycleStatus {
	message := strings.ToLower(execErr.Error())
	switch {
	case strings.Contains(message, "status 422"),
		strings.Contains(message, "status 400"):
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED
	case strings.Contains(message, "wait for trace"),
		strings.Contains(message, "trace status"),
		strings.Contains(message, "provider"),
		strings.Contains(message, "inference"):
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED
	case strings.Contains(message, "submit chat"):
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED
	default:
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED
	}
}

func isRecoverableAssignmentExecutionError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "evaluation: execute assignment:") ||
		strings.Contains(message, "evaluation: import assignment result from trace:")
}

func (c *CampaignController) buildFailureAssignmentResult(ctx context.Context, req AssignmentExecutionRequest, execErr error) (*evalv1.EvaluationAssignmentResult, error) {
	if c == nil || c.store == nil || req.Assignment == nil {
		return nil, fmt.Errorf("evaluation: build failure assignment result: %w", constants.ErrMissingRequiredField)
	}
	trace, err := c.store.LoadAssignmentTrace(ctx, req.Assignment.GetRunId(), req.Assignment.GetAssignmentId())
	if err == nil && len(trace) > 0 {
		var traceEvidence *compliancev1.ComplianceEvidenceReference
		traceEvidence, err = BuildAssignmentTraceEvidenceReference(req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), req.AttemptID, trace, c.now().UTC())
		if err != nil {
			traceEvidence = nil
		}
		result, importErr := ImportAssignmentResultFromTrace(req, trace, traceEvidence, c.now().UTC(), c.newID)
		if importErr == nil {
			return result, nil
		}
	}
	return BuildExecutorFailureAssignmentResult(req, execErr, c.now().UTC(), c.newID)
}

func (c *CampaignController) persistTerminalAssignment(ctx context.Context, assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) error {
	if err := c.store.SaveAssignmentResult(ctx, result); err != nil {
		return err
	}
	assignment.LifecycleStatus = result.GetLifecycleStatus()
	if result.GetCompletedAt() != nil {
		assignment.CompletedAt = result.GetCompletedAt()
	} else {
		assignment.CompletedAt = timestamppb.New(c.now().UTC())
	}
	if err := c.store.SaveAssignment(ctx, assignment); err != nil {
		return err
	}
	return c.publishAssignmentTerminal(ctx, assignment, result)
}

// RepairAssignmentsWithoutResults backfills persisted terminal results for
// assignments that already reached a terminal lifecycle.
func (c *CampaignController) RepairAssignmentsWithoutResults(ctx context.Context, runID string, artifacts map[string]ScenarioArtifacts) (int, error) {
	if c == nil || c.store == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: repair assignments without results: %w", constants.ErrMissingRequiredField)
	}
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return 0, err
	}
	repaired := 0
	for _, assignment := range assignments {
		if assignment == nil || !isTerminalAssignmentLifecycle(assignment.GetLifecycleStatus()) {
			continue
		}
		exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return repaired, err
		}
		if exists {
			continue
		}
		result, err := c.recoverTerminalAssignmentResult(ctx, assignment, artifacts)
		if err != nil {
			return repaired, err
		}
		if err := c.persistTerminalAssignment(ctx, assignment, result); err != nil {
			return repaired, err
		}
		repaired++
	}
	return repaired, nil
}

func (c *CampaignController) recoverTerminalAssignmentResult(ctx context.Context, assignment *evalv1.EvaluationAssignment, artifacts map[string]ScenarioArtifacts) (*evalv1.EvaluationAssignmentResult, error) {
	trace, traceErr := c.store.LoadAssignmentTrace(ctx, assignment.GetRunId(), assignment.GetAssignmentId())
	if traceErr == nil && len(trace) > 0 && artifacts != nil {
		artifact, ok := artifacts[assignment.GetScenarioId()]
		if ok {
			var scenarioInput ScenarioInputFixture
			if err := json.Unmarshal(artifact.Input.Body, &scenarioInput); err == nil {
				var scenarioGold ScenarioGoldCriteria
				if err := json.Unmarshal(artifact.Gold.Body, &scenarioGold); err == nil {
					run, err := c.store.LoadRun(ctx, assignment.GetRunId())
					if err == nil {
						catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
						if err == nil {
							gradingMethod, err := scenarioGradingMethodForAssignment(catalog, assignment)
							if err == nil {
								scenarioTools, err := scenarioToolsForAssignment(catalog, assignment)
								if err == nil {
									requiredConcepts, err := scenarioRequiredConceptsForAssignment(catalog, assignment)
									if err == nil {
										req := AssignmentExecutionRequest{
											Assignment:       assignment,
											AttemptID:        "recovered",
											ScenarioInput:    scenarioInput,
											ScenarioGold:     scenarioGold,
											ScenarioTools:    scenarioTools,
											RequiredConcepts: requiredConcepts,
											GradingMethod:    gradingMethod,
										}
										traceEvidence, err := BuildAssignmentTraceEvidenceReference(assignment.GetRunId(), assignment.GetAssignmentId(), req.AttemptID, trace, c.now().UTC())
										if err != nil {
											traceEvidence = nil
										}
										result, importErr := ImportAssignmentResultFromTrace(req, trace, traceEvidence, c.now().UTC(), c.newID)
										if importErr == nil {
											return result, nil
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	recoveredErr := errors.New("recovered terminal assignment without persisted result")
	if traceErr != nil {
		recoveredErr = fmt.Errorf("%w: %v", recoveredErr, traceErr)
	}
	req := AssignmentExecutionRequest{Assignment: assignment, AttemptID: "recovered"}
	result, err := BuildExecutorFailureAssignmentResult(req, recoveredErr, c.now().UTC(), c.newID)
	if err != nil {
		return BuildRecoveredTerminalAssignmentResult(assignment, c.now().UTC())
	}
	result.LifecycleStatus = assignment.GetLifecycleStatus()
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

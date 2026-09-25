// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// IsHeterogeneousAssignment reports whether one assignment targets a frozen
// heterogeneous stack instead of a single homogeneous model-role cell.
func IsHeterogeneousAssignment(assignment *evalv1.EvaluationAssignment) bool {
	if assignment == nil {
		return false
	}
	heterogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Heterogeneous)
	return ok && heterogeneous.Heterogeneous != nil && heterogeneous.Heterogeneous.GetStack() != nil
}

// HeterogeneousStackFromAssignment returns the frozen stack binding for one
// system-lane assignment.
func HeterogeneousStackFromAssignment(assignment *evalv1.EvaluationAssignment) (*evalv1.HeterogeneousStackDefinition, error) {
	if !IsHeterogeneousAssignment(assignment) {
		return nil, fmt.Errorf("evaluation: heterogeneous stack: %w", constants.ErrMissingRequiredField)
	}
	stack := assignment.GetHeterogeneous().GetStack()
	if err := ValidateHeterogeneousStackDigest(stack); err != nil {
		return nil, err
	}
	return stack, nil
}

// CampaignFormationBindingRequest resolves one heterogeneous assignment and the
// frozen registry into a governed formation binding request.
func CampaignFormationBindingRequest(assignment *evalv1.EvaluationAssignment, variants []*evalv1.ModelVariant) (FormationBindingRequest, error) {
	stack, err := HeterogeneousStackFromAssignment(assignment)
	if err != nil {
		return FormationBindingRequest{}, fmt.Errorf("evaluation: campaign formation binding: %w", err)
	}
	return FormationBindingRequest{
		FormationID: stack.GetStackId(),
		Stack:       stack,
		Variants:    variants,
	}, nil
}

// CampaignFormationRunner executes one bound heterogeneous stack through the
// governed FormationRunner production seam.
type CampaignFormationRunner interface {
	RunHeterogeneousFormation(ctx context.Context, binding FormationBindingRequest, runContext FormationRunContext, initialState []byte) (*FormationRunResult, error)
}

type campaignFormationProductionRunner struct {
	deps FormationProductionDependencies
}

// NewCampaignFormationProductionRunner wires governed production dependencies
// into heterogeneous campaign execution.
func NewCampaignFormationProductionRunner(deps FormationProductionDependencies) CampaignFormationRunner {
	return &campaignFormationProductionRunner{deps: deps}
}

func (r *campaignFormationProductionRunner) RunHeterogeneousFormation(ctx context.Context, binding FormationBindingRequest, runContext FormationRunContext, initialState []byte) (*FormationRunResult, error) {
	deps := r.deps
	deps.RunContext = runContext
	return RunHeterogeneousFormationProduction(ctx, binding, deps, initialState)
}

// CampaignFormationExecutor runs heterogeneous system-lane assignments through
// Lite → Assistant → Primary governed formation execution.
type CampaignFormationExecutor struct {
	variants []*evalv1.ModelVariant
	runner   CampaignFormationRunner
	now      func() time.Time
	newID    func(string) string
}

// NewCampaignFormationExecutor constructs the heterogeneous campaign executor.
func NewCampaignFormationExecutor(variants []*evalv1.ModelVariant, runner CampaignFormationRunner, now func() time.Time, newID func(string) string) *CampaignFormationExecutor {
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	return &CampaignFormationExecutor{
		variants: variants,
		runner:   runner,
		now:      now,
		newID:    newID,
	}
}

// ExecuteAssignment executes one heterogeneous assignment and materializes the
// terminal assignment result from formation role telemetry.
func (e *CampaignFormationExecutor) ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	if e == nil || e.runner == nil {
		return nil, fmt.Errorf("evaluation: execute heterogeneous assignment: formation executor is required")
	}
	if !IsHeterogeneousAssignment(req.Assignment) {
		return nil, fmt.Errorf("evaluation: execute heterogeneous assignment: heterogeneous target required")
	}
	binding, err := CampaignFormationBindingRequest(req.Assignment, e.variants)
	if err != nil {
		return nil, err
	}
	initialState := []byte(req.ScenarioInput.UserPrompt)
	runContext := FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}
	formationResult, err := e.runner.RunHeterogeneousFormation(ctx, binding, runContext, initialState)
	if err != nil {
		return nil, fmt.Errorf("evaluation: execute heterogeneous assignment: %w", err)
	}
	return ImportAssignmentResultFromFormationRun(req, formationResult, e.now().UTC(), e.newID)
}

// CampaignAssignmentRouter dispatches one assignment to the homogeneous chat
// executor or the heterogeneous formation executor.
type CampaignAssignmentRouter struct {
	homogeneous   CampaignAssignmentExecutor
	heterogeneous CampaignAssignmentExecutor
}

// NewCampaignAssignmentRouter constructs a lane-aware campaign executor.
func NewCampaignAssignmentRouter(homogeneous, heterogeneous CampaignAssignmentExecutor) *CampaignAssignmentRouter {
	return &CampaignAssignmentRouter{homogeneous: homogeneous, heterogeneous: heterogeneous}
}

// ExecuteAssignment routes one assignment to the correct governed executor.
func (r *CampaignAssignmentRouter) ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation: execute assignment: router is required")
	}
	if IsHeterogeneousAssignment(req.Assignment) {
		if r.heterogeneous == nil {
			return nil, fmt.Errorf("evaluation: execute assignment: heterogeneous executor is required")
		}
		return r.heterogeneous.ExecuteAssignment(ctx, req)
	}
	if r.homogeneous == nil {
		return nil, fmt.Errorf("evaluation: execute assignment: homogeneous executor is required")
	}
	return r.homogeneous.ExecuteAssignment(ctx, req)
}

// ImportAssignmentResultFromFormationRun materializes one terminal heterogeneous
// assignment result from governed formation role telemetry.
func ImportAssignmentResultFromFormationRun(req AssignmentExecutionRequest, formationResult *FormationRunResult, now time.Time, newID func(string) string) (*evalv1.EvaluationAssignmentResult, error) {
	if req.Assignment == nil || formationResult == nil {
		return nil, fmt.Errorf("evaluation: import formation assignment result: %w", constants.ErrMissingRequiredField)
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lifecycle, grades := classifyFormationAssignmentOutcome(req, formationResult)
	modelInferences, scoredSpan := modelInferenceRecordsFromFormationRun(req.Assignment, req.AttemptID, formationResult, newID)
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:            CampaignSchemaVersion,
		AssignmentId:             req.Assignment.GetAssignmentId(),
		RunId:                    req.Assignment.GetRunId(),
		CampaignId:               req.Assignment.GetCampaignId(),
		Lane:                     req.Assignment.GetLane(),
		LifecycleStatus:          lifecycle,
		ModelInferences:          modelInferences,
		DeterministicGrades:      grades,
		ScoredInferenceSpanNanos: scoredSpan,
		CompletedAt:              timestamppb.New(now),
	}
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

func classifyFormationAssignmentOutcome(req AssignmentExecutionRequest, formationResult *FormationRunResult) (evalv1.EvaluationAssignmentLifecycleStatus, []*evalv1.DeterministicGrade) {
	grade := &evalv1.DeterministicGrade{
		GradeId:     req.Assignment.GetAssignmentId() + ":formation-roles",
		CriterionId: "formation-roles",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Detail:      "heterogeneous formation did not complete all three roles",
	}
	if formationResult == nil {
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED, []*evalv1.DeterministicGrade{grade}
	}
	if formationResult.Passed && len(formationResult.Roles) == 3 {
		grade.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
		grade.Score = 1
		grade.Detail = "heterogeneous formation completed Lite → Assistant → Primary"
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, []*evalv1.DeterministicGrade{grade}
	}
	if len(formationResult.Roles) == 0 {
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, []*evalv1.DeterministicGrade{grade}
	}
	return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, []*evalv1.DeterministicGrade{grade}
}

func modelInferenceRecordsFromFormationRun(assignment *evalv1.EvaluationAssignment, attemptID string, formationResult *FormationRunResult, newID func(string) string) ([]*evalv1.ModelInferenceRecord, *uint64) {
	if formationResult == nil || len(formationResult.Roles) == 0 {
		return nil, nil
	}
	records := make([]*evalv1.ModelInferenceRecord, 0, len(formationResult.Roles))
	var span uint64
	for _, role := range formationResult.Roles {
		if role.ProviderAttemptID == "" {
			continue
		}
		record := &evalv1.ModelInferenceRecord{
			InferenceRecordId:       newID("inference"),
			ProviderAttemptId:       role.ProviderAttemptID,
			AssignmentId:            assignment.GetAssignmentId(),
			EvaluationAttemptId:     attemptID,
			ModelRole:               formationRoleToCampaignRole(role.Role),
			CallSite:                "formation:" + string(role.Role),
			ModelVariant:            formationModelToVariant(role.Model),
			PrivacyAttested:         role.AttestationVerified || role.AttestationStatus == FormationAttestationNotNeeded,
			CompletionTokens:        role.GenerationTokens,
			GenerationDurationNanos: role.GenerationDurationNanos,
			UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
		}
		if role.TTFTNanos > 0 {
			record.FirstTokenAtUnixNanos = role.TTFTNanos
		}
		if role.GenerationDurationNanos > 0 {
			span += role.GenerationDurationNanos
		}
		records = append(records, record)
	}
	if span == 0 {
		return records, nil
	}
	return records, &span
}

func formationRoleToCampaignRole(role FormationRole) evalv1.ModelCampaignRole {
	switch role {
	case FormationRolePrimary:
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY
	case FormationRoleAssistant:
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT
	case FormationRoleLite:
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE
	default:
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED
	}
}

func formationModelToVariant(model FormationModel) *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      model.VariantID,
		ProviderClass:  model.ProviderClass,
		ServedModelTag: model.ServedModelTag,
		ModelDigest:    model.ModelDigest,
		ModelFamily:    model.Family,
		ParameterCount: model.ParameterCount,
		Quantization:   model.Quantization,
	}
}

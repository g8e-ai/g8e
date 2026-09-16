// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ValidateHomogeneousCampaignTrace verifies a terminal g8ee trace for one
// homogeneous model-role campaign assignment.
func ValidateHomogeneousCampaignTrace(req ChatProbeRequest, trace map[string]any) error {
	if err := ValidateChatProbeTrace(req, trace); err != nil {
		return err
	}
	modelCalls, _ := trace["model_calls"].([]any)
	return validateHomogeneousRoleTrace(trace, req.DesignatedModelRole, modelCalls)
}

// ImportAssignmentResultFromTrace materializes one terminal assignment result
// from a validated g8ee trace and optional content-addressed trace evidence.
func ImportAssignmentResultFromTrace(req AssignmentExecutionRequest, trace map[string]any, traceEvidence *compliancev1.ComplianceEvidenceReference, now time.Time, newID func(string) string) (*evalv1.EvaluationAssignmentResult, error) {
	if req.Assignment == nil || req.AttemptID == "" || len(trace) == 0 {
		return nil, fmt.Errorf("evaluation: import assignment result from trace: assignment, attempt, and trace are required")
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	probeReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding)
	if err != nil {
		return nil, err
	}
	lifecycle, _ := classifyCampaignTraceOutcome(probeReq, trace)
	candidate := homogeneousCandidateVariant(req.Assignment)
	grading, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   req.Assignment.GetAssignmentId(),
		ScenarioID:     req.Assignment.GetScenarioId(),
		DesignatedRole: probeReq.DesignatedModelRole,
		GradingMethod:  req.GradingMethod,
		ScenarioInput:  req.ScenarioInput,
		ScenarioGold:   req.ScenarioGold,
		Trace:          trace,
		Lifecycle:      lifecycle,
	})
	if err != nil {
		return nil, err
	}
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:       CampaignSchemaVersion,
		AssignmentId:        req.Assignment.GetAssignmentId(),
		RunId:               req.Assignment.GetRunId(),
		CampaignId:          req.Assignment.GetCampaignId(),
		Lane:                req.Assignment.GetLane(),
		LifecycleStatus:     lifecycle,
		ModelInferences:     modelInferenceRecordsFromTrace(req.Assignment, req.AttemptID, candidate, trace, newID),
		DeterministicGrades: grading.DeterministicGrades,
		SemanticGrades:      grading.SemanticGrades,
		DecomposedScores:    grading.DecomposedScores,
		CompletedAt:         timestamppb.New(now),
	}
	if traceEvidence != nil {
		result.EvidenceRefs = []*compliancev1.ComplianceEvidenceReference{traceEvidence}
	}
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

func classifyCampaignTraceOutcome(req ChatProbeRequest, trace map[string]any) (evalv1.EvaluationAssignmentLifecycleStatus, *evalv1.DeterministicGrade) {
	status, _ := trace["status"].(string)
	grade := &evalv1.DeterministicGrade{
		GradeId:     req.AssignmentID + ":role-invoked",
		CriterionId: "role-invoked",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Detail:      "designated model role was not invoked",
	}
	switch status {
	case "failed":
		grade.Detail = "g8ee assignment trace failed"
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, grade
	case "completed":
		if err := ValidateHomogeneousCampaignTrace(req, trace); err != nil {
			if strings.Contains(err.Error(), "role_outcome not invoked") || strings.Contains(err.Error(), "role_not_invoked") {
				grade.Detail = "designated model role was not invoked"
				return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, grade
			}
			grade.Detail = err.Error()
			return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED, grade
		}
		grade.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
		grade.Score = 1
		grade.Detail = "designated model role invoked with governed inference evidence"
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, grade
	default:
		grade.Detail = fmt.Sprintf("trace status %q is not terminal", status)
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED, grade
	}
}

func homogeneousCandidateVariant(assignment *evalv1.EvaluationAssignment) *evalv1.ModelVariant {
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil {
		return nil
	}
	return homogeneous.Homogeneous.GetCandidateVariant()
}

func modelInferenceRecordsFromTrace(assignment *evalv1.EvaluationAssignment, attemptID string, candidate *evalv1.ModelVariant, trace map[string]any, newID func(string) string) []*evalv1.ModelInferenceRecord {
	modelCalls, _ := trace["model_calls"].([]any)
	records := make([]*evalv1.ModelInferenceRecord, 0, len(modelCalls))
	for _, rawCall := range modelCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		provider, _ := call["provider"].(string)
		if !strings.EqualFold(provider, "G8EProvider") {
			continue
		}
		if succeeded, ok := call["succeeded"].(bool); ok && !succeeded {
			continue
		}
		providerAttemptID, _ := call["provider_attempt_id"].(string)
		if providerAttemptID == "" {
			continue
		}
		record := &evalv1.ModelInferenceRecord{
			InferenceRecordId:   newID("inference"),
			ProviderAttemptId:   providerAttemptID,
			AssignmentId:        assignment.GetAssignmentId(),
			EvaluationAttemptId: attemptID,
			ModelRole:           parseModelCampaignRole(call["model_role"]),
			AgentPersona:        stringValue(call["agent_role"]),
			CallSite:            stringValue(call["call_site"]),
			ModelVariant:        candidate,
			InputHash:           stringValue(call["normalized_request_hash"]),
			OutputHash:          stringValue(call["output_hash"]),
			ResultDigest:        stringValue(call["governed_result_digest"]),
			PrivacyAttested:     true,
		}
		if transactionID, _ := call["governed_transaction_id"].(string); transactionID != "" {
			record.GovernedReceiptRef = &compliancev1.ComplianceEvidenceReference{
				ArtifactId:   transactionID,
				ArtifactType: string(complianceevidence.ArtifactTypeEvalReceipt),
				SchemaRef:    "g8e.operator.v1.ActionReceipt",
			}
		}
		records = append(records, record)
	}
	return records
}

func parseModelCampaignRole(raw any) evalv1.ModelCampaignRole {
	role, _ := raw.(string)
	switch role {
	case "primary":
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY
	case "assistant":
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT
	case "lite":
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE
	default:
		return evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED
	}
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

// BuildAssignmentTraceEvidenceReference returns a content-addressed evidence
// reference for one imported g8ee assignment trace body.
func BuildAssignmentTraceEvidenceReference(runID, assignmentID, attemptID string, trace map[string]any, producedAt time.Time) (*compliancev1.ComplianceEvidenceReference, error) {
	if runID == "" || assignmentID == "" || attemptID == "" || len(trace) == 0 {
		return nil, fmt.Errorf("evaluation: build assignment trace evidence reference: run, assignment, attempt, and trace are required")
	}
	body, err := marshalSortedJSON(trace)
	if err != nil {
		return nil, fmt.Errorf("evaluation: build assignment trace evidence reference: %w", err)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("evaluation: build assignment trace evidence reference: %w", err)
	}
	artifactID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvaluationAssignmentTrace, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return nil, fmt.Errorf("evaluation: build assignment trace evidence reference: invalid content address")
	}
	if producedAt.IsZero() {
		producedAt = time.Now().UTC()
	}
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         artifactID,
		ArtifactType:       string(complianceevidence.ArtifactTypeEvaluationAssignmentTrace),
		Sha256:             digest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.eval.v1.EvaluationAssignmentTrace",
		ProducerIdentity:   "g8ee",
		ProducedAt:         timestamppb.New(producedAt),
		ScopeId:            runID,
		RunId:              runID,
		AttemptId:          attemptID,
		VerificationStatus: "imported",
	}, nil
}

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
	probeReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{
		GradingMethod:    req.GradingMethod,
		ScenarioGold:     req.ScenarioGold,
		ScenarioTools:    req.ScenarioTools,
		RequiredConcepts: req.RequiredConcepts,
	})
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
		ScenarioTools:  req.ScenarioTools,
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
		ToolDecisions:       toolDecisionRecordsFromTrace(req.Assignment, trace, newID),
		ToolCalls:           toolCallRecordsFromTrace(req.Assignment, trace, newID),
		GovernedActions:     governedActionBindingsFromTrace(req.Assignment, trace, newID),
		DeterministicGrades: grading.DeterministicGrades,
		SemanticGrades:      mergeSemanticGrades(grading.SemanticGrades, semanticGradesFromTrace(req.Assignment.GetAssignmentId(), trace)),
		GraderCalls:         graderCallRecordsFromTrace(req.Assignment, trace, newID),
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
	roleOutcome, _ := trace["role_outcome"].(string)
	grade := &evalv1.DeterministicGrade{
		GradeId:     req.AssignmentID + ":role-invoked",
		CriterionId: "role-invoked",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Detail:      "designated model role was not invoked",
	}
	// Homogeneous model-role evaluation treats "could not invoke designated role"
	// as a scored capability outcome, not an infrastructure/provider failure.
	if roleOutcome == "role_not_invoked" {
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, grade
	}
	switch status {
	case "failed":
		grade.Detail = "g8ee assignment trace failed"
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, grade
	case "completed":
		if err := ValidateHomogeneousCampaignTrace(req, trace); err != nil {
			if strings.Contains(err.Error(), "role_outcome not invoked") || strings.Contains(err.Error(), "role_not_invoked") {
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

func toolDecisionRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace map[string]any, newID func(string) string) []*evalv1.ToolDecisionRecord {
	records := make([]*evalv1.ToolDecisionRecord, 0)
	for _, rawDecision := range traceToolRecords(trace, "tool_decisions") {
		decision, ok := rawDecision.(map[string]any)
		if !ok {
			continue
		}
		toolName := stringValue(decision["tool_name"])
		if toolName == "" {
			continue
		}
		decisionID := stringValue(decision["decision_id"])
		if decisionID == "" {
			decisionID = newID("tool-decision")
		}
		records = append(records, &evalv1.ToolDecisionRecord{
			DecisionId:          decisionID,
			AssignmentId:        assignment.GetAssignmentId(),
			ToolName:            toolName,
			Recognized:          boolValue(decision["recognized"]),
			Selected:            boolValue(decision["selected"]),
			PermissionCompliant: boolValue(decision["permission_compliant"]),
			Unnecessary:         boolValue(decision["unnecessary"]),
			Outcome:             toolOutcomeStatus(stringValue(decision["outcome"])),
		})
	}
	return records
}

func toolCallRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace map[string]any, newID func(string) string) []*evalv1.ToolCallRecord {
	records := make([]*evalv1.ToolCallRecord, 0)
	for _, rawCall := range traceToolRecords(trace, "tool_calls") {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		toolName := stringValue(call["tool_name"])
		if toolName == "" {
			continue
		}
		callID := stringValue(call["call_id"])
		if callID == "" {
			callID = newID("tool-call")
		}
		record := &evalv1.ToolCallRecord{
			CallId:          callID,
			AssignmentId:    assignment.GetAssignmentId(),
			ToolName:        toolName,
			ArgumentsHash:   stringValue(call["arguments_hash"]),
			SchemaOutcome:   toolCallOutcome(boolValue(call["success"])),
			SemanticOutcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
		}
		if executionID := stringValue(call["execution_id"]); executionID != "" && boolValue(call["is_operator_tool"]) {
			record.GovernedBindingRef = &compliancev1.ComplianceEvidenceReference{
				ArtifactId:   executionID,
				ArtifactType: string(complianceevidence.ArtifactTypeEvalReceipt),
				SchemaRef:    "g8e.operator.v1.ActionReceipt",
			}
		}
		records = append(records, record)
	}
	return records
}

func governedActionBindingsFromTrace(assignment *evalv1.EvaluationAssignment, trace map[string]any, newID func(string) string) []*evalv1.GovernedActionBinding {
	records := make([]*evalv1.GovernedActionBinding, 0)
	for _, rawAction := range traceToolRecords(trace, "governed_actions") {
		action, ok := rawAction.(map[string]any)
		if !ok {
			continue
		}
		bindingID := stringValue(action["binding_id"])
		if bindingID == "" {
			bindingID = newID("governed-action")
		}
		transactionID := stringValue(action["transaction_id"])
		if transactionID == "" {
			transactionID = bindingID
		}
		record := &evalv1.GovernedActionBinding{
			BindingId:         bindingID,
			AssignmentId:      assignment.GetAssignmentId(),
			TransactionId:     transactionID,
			OperatorId:        stringValue(action["operator_id"]),
			OperatorSessionId: stringValue(action["operator_session_id"]),
			PolicyDecision:    stringValue(action["policy_decision"]),
		}
		if transactionID != "" {
			record.ReceiptRef = &compliancev1.ComplianceEvidenceReference{
				ArtifactId:   transactionID,
				ArtifactType: string(complianceevidence.ArtifactTypeEvalReceipt),
				SchemaRef:    "g8e.operator.v1.ActionReceipt",
			}
		}
		records = append(records, record)
	}
	return records
}

func mergeSemanticGrades(primary, imported []*evalv1.SemanticGrade) []*evalv1.SemanticGrade {
	if len(imported) > 0 {
		return imported
	}
	return primary
}

func graderCallRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace map[string]any, newID func(string) string) []*evalv1.GraderModelCallRecord {
	records := make([]*evalv1.GraderModelCallRecord, 0)
	for _, rawCall := range traceToolRecords(trace, "grader_calls") {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		callID := stringValue(call["grader_call_id"])
		if callID == "" {
			callID = newID("grader-call")
		}
		record := &evalv1.GraderModelCallRecord{
			GraderCallId:   callID,
			AssignmentId:   assignment.GetAssignmentId(),
			JudgeVariantId: stringValue(call["judge_variant_id"]),
		}
		if attemptID := stringValue(call["provider_attempt_id"]); attemptID != "" {
			record.InferenceRecordRef = &compliancev1.ComplianceEvidenceReference{
				ArtifactId:   attemptID,
				ArtifactType: string(complianceevidence.ArtifactTypeEvalReceipt),
				SchemaRef:    "g8e.eval.v1.ModelInferenceRecord",
			}
		}
		records = append(records, record)
	}
	return records
}

func toolOutcomeStatus(outcome string) evalv1.EvaluationVerdictStatus {
	switch outcome {
	case "pass":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	case "fail":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	default:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE
	}
}

func toolCallOutcome(success bool) evalv1.EvaluationVerdictStatus {
	if success {
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	}
	return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
}

func boolValue(raw any) bool {
	value, ok := raw.(bool)
	return ok && value
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
		if loadDuration := durationSecondsToNanos(call["load_duration_seconds"]); loadDuration > 0 {
			record.LoadDurationNanos = loadDuration
		}
		if generationDuration := durationSecondsToNanos(call["generation_duration_seconds"]); generationDuration > 0 {
			record.GenerationDurationNanos = generationDuration
		}
		if totalDuration := durationSecondsToNanos(call["total_duration_seconds"]); totalDuration > 0 {
			record.TotalDurationNanos = totalDuration
		}
		if ttft := durationSecondsToNanos(call["time_to_first_token_seconds"]); ttft > 0 {
			if monotonicStart := floatSeconds(call["monotonic_start"]); monotonicStart > 0 {
				record.RequestStartedAtUnixNanos = uint64(monotonicStart * 1_000_000_000)
				record.FirstTokenAtUnixNanos = record.RequestStartedAtUnixNanos + ttft
			}
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

func floatSeconds(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func durationSecondsToNanos(raw any) uint64 {
	seconds := floatSeconds(raw)
	if seconds <= 0 {
		return 0
	}
	return uint64(seconds * 1_000_000_000)
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

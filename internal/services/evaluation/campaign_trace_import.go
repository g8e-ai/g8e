// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ValidateHomogeneousCampaignTrace verifies a terminal g8ee trace for one
// homogeneous model-role campaign assignment.
func ValidateHomogeneousCampaignTrace(req ChatProbeRequest, trace EvaluationTrace) error {
	if err := ValidateChatProbeTrace(req, trace); err != nil {
		return err
	}
	modelCalls, _ := trace["model_calls"].([]any)
	return validateHomogeneousRoleTrace(trace, req.DesignatedModelRole, modelCalls)
}

// workspaceFromTrace extracts the ScenarioWorkspace from a trace's evaluation_context.
func workspaceFromTrace(trace EvaluationTrace) *ScenarioWorkspace {
	if len(trace) == 0 {
		return nil
	}
	evalCtx, ok := evaluationTrace(trace["evaluation_context"])
	if !ok {
		return nil
	}
	wsData, ok := evaluationTrace(evalCtx["workspace"])
	if !ok {
		return nil
	}
	root, _ := wsData["root"].(string)
	opWd, _ := wsData["operator_working_directory"].(string)
	if root == "" || opWd == "" {
		return nil
	}
	return &ScenarioWorkspace{
		Root:                     root,
		OperatorWorkingDirectory: opWd,
	}
}

// ImportAssignmentResultFromTrace materializes one terminal assignment result
// from a validated g8ee trace and optional content-addressed trace evidence.
func ImportAssignmentResultFromTrace(req AssignmentExecutionRequest, trace EvaluationTrace, traceEvidence *compliancev1.ComplianceEvidenceReference, now time.Time, newID func(string) string) (*evalv1.EvaluationAssignmentResult, error) {
	if req.Assignment == nil || req.AttemptID == "" || len(trace) == 0 {
		return nil, fmt.Errorf("evaluation: import assignment result from trace: assignment, attempt, and trace are required")
	}
	// The digest binds every field the result is derived from, so it is checked
	// before the trace is classified or graded, for every status. Classification
	// reads the trace first and reports a validation error as a scored FAILED
	// outcome whose reason this function drops, and it skips validation entirely
	// for a failed or role-not-invoked trace; a trace whose digest does not hold is
	// a harness failure and never a model result.
	if err := validateTraceDigest(trace); err != nil {
		return nil, fmt.Errorf("evaluation: import assignment result from trace: %w", err)
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	// The request is rebuilt around the workspace the trace echoes, so on this
	// path "the trace echoes the workspace that was sent" is circular. What binds
	// the workspace is ScenarioWorkspace.Validate(runID, attemptID) inside
	// ValidateChatProbeTrace, which re-derives the root from the run and attempt.
	ws := workspaceFromTrace(trace)
	probeReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{
		GradingMethod:    req.GradingMethod,
		ScenarioGold:     req.ScenarioGold,
		ScenarioTools:    req.ScenarioTools,
		RequiredConcepts: req.RequiredConcepts,
	}, ws)
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
	modelInferences, scoredSpan, err := modelInferenceRecordsFromTrace(req.Assignment, req.AttemptID, candidate, trace, newID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: import assignment result from trace: model telemetry: %w", err)
	}
	policyDecisions, err := policyDecisionRecordsFromTrace(req.Assignment, trace)
	if err != nil {
		return nil, fmt.Errorf("evaluation: import assignment result from trace: policy telemetry: %w", err)
	}
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:            CampaignSchemaVersion,
		AssignmentId:             req.Assignment.GetAssignmentId(),
		RunId:                    req.Assignment.GetRunId(),
		CampaignId:               req.Assignment.GetCampaignId(),
		Lane:                     req.Assignment.GetLane(),
		LifecycleStatus:          lifecycle,
		ModelInferences:          modelInferences,
		ToolDecisions:            toolDecisionRecordsFromTrace(req.Assignment, trace, newID),
		ToolCalls:                toolCallRecordsFromTrace(req.Assignment, trace, newID),
		GovernedActions:          governedActionBindingsFromTrace(req.Assignment, trace, newID),
		DeterministicGrades:      grading.DeterministicGrades,
		SemanticGrades:           mergeSemanticGrades(grading.SemanticGrades, semanticGradesFromTrace(req.Assignment.GetAssignmentId(), trace)),
		GraderCalls:              graderCallRecordsFromTrace(req.Assignment, trace, newID),
		DecomposedScores:         grading.DecomposedScores,
		PolicyDecisions:          policyDecisions,
		ToolDecisionsCaptured:    traceFieldCaptured(trace, "tool_decisions"),
		ToolCallsCaptured:        traceFieldCaptured(trace, "tool_calls"),
		GovernedActionsCaptured:  traceFieldCaptured(trace, "governed_actions"),
		PolicyDecisionsCaptured:  traceFieldCaptured(trace, "policy_decisions"),
		ScoredInferenceSpanNanos: scoredSpan,
		TrajectoryOutcome:        grading.Trajectory.Outcome,
		GuidedRetryCount:         grading.Trajectory.GuidedRetryCount,
		FailureReason:            grading.Trajectory.FailureReason,
		PublicFailureReason:      grading.Trajectory.PublicFailureReason,
		CompletedAt:              timestamppb.New(now),
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

// PartialAssignmentResultFromScoredTrace materializes only the scored model
// inference rows present in a non-terminal trace. It returns false when the
// trace has not yet reported usage for any governed model call.
func PartialAssignmentResultFromScoredTrace(req AssignmentExecutionRequest, trace EvaluationTrace, newID func(string) string) (*evalv1.EvaluationAssignmentResult, bool, error) {
	if req.Assignment == nil || len(trace) == 0 {
		return nil, false, nil
	}
	if !TraceHasReportedModelInference(trace) {
		return nil, false, nil
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	candidate := homogeneousCandidateVariant(req.Assignment)
	modelInferences, _, err := modelInferenceRecordsFromTrace(req.Assignment, req.AttemptID, candidate, trace, newID)
	if err != nil {
		return nil, false, err
	}
	partial := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    req.Assignment.GetAssignmentId(),
		RunId:           req.Assignment.GetRunId(),
		CampaignId:      req.Assignment.GetCampaignId(),
		Lane:            req.Assignment.GetLane(),
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING,
		ModelInferences: modelInferences,
	}
	if len(reportedModelInferences(partial)) == 0 {
		return nil, false, nil
	}
	return partial, true, nil
}

// TraceHasReportedModelInference reports whether a trace already carries at
// least one governed model call with reported usage telemetry.
func TraceHasReportedModelInference(trace EvaluationTrace) bool {
	modelCalls, ok := trace["model_calls"].([]any)
	if !ok {
		return false
	}
	for _, rawCall := range modelCalls {
		call, ok := evaluationTrace(rawCall)
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
		reported, ok := call["usage_reported"].(bool)
		if ok && reported {
			return true
		}
	}
	return false
}

func classifyCampaignTraceOutcome(req ChatProbeRequest, trace EvaluationTrace) (evalv1.EvaluationAssignmentLifecycleStatus, *evalv1.DeterministicGrade) {
	status, _ := trace["status"].(string)
	roleOutcome, _ := trace["role_outcome"].(string)
	grade := &evalv1.DeterministicGrade{
		GradeId:     req.AssignmentID + ":role-invoked",
		CriterionId: "role-invoked",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Detail:      "designated model role was not invoked",
	}
	if status == "failed" && trace["provider_tool_rejection"] != nil {
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, grade
	}
	modelCalls, _ := trace["model_calls"].([]any)
	// A failed trace with zero model calls never reached governed inference — that
	// is provider/infrastructure failure, not a scored capability miss.
	if status == "failed" && len(modelCalls) == 0 {
		grade.Detail = "g8ee assignment trace failed before model invocation"
		return evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, grade
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

func toolDecisionRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace EvaluationTrace, newID func(string) string) []*evalv1.ToolDecisionRecord {
	records := make([]*evalv1.ToolDecisionRecord, 0)
	for _, rawDecision := range traceToolRecords(trace, "tool_decisions") {
		decision, ok := evaluationTrace(rawDecision)
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

func toolCallRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace EvaluationTrace, newID func(string) string) []*evalv1.ToolCallRecord {
	records := make([]*evalv1.ToolCallRecord, 0)
	for _, rawCall := range traceToolRecords(trace, "tool_calls") {
		call, ok := evaluationTrace(rawCall)
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
		var loopTurn uint32
		if rawTurn, present := call["loop_turn"]; present && rawTurn != nil {
			if converted, err := uint32Value(rawTurn); err == nil {
				loopTurn = converted
			}
		}
		errType := stringValue(call["error_type"])
		guidanceShown := stringValue(call["error"]) != "" || stringValue(call["suggestion"]) != ""
		record := &evalv1.ToolCallRecord{
			CallId:          callID,
			AssignmentId:    assignment.GetAssignmentId(),
			ToolName:        toolName,
			ArgumentsHash:   stringValue(call["arguments_hash"]),
			SchemaOutcome:   toolCallOutcome(boolValue(call["success"])),
			SemanticOutcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
			LoopTurn:        loopTurn,
			ErrorType:       errType,
			GuidanceShown:   guidanceShown,
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

func governedActionBindingsFromTrace(assignment *evalv1.EvaluationAssignment, trace EvaluationTrace, newID func(string) string) []*evalv1.GovernedActionBinding {
	records := make([]*evalv1.GovernedActionBinding, 0)
	for _, rawAction := range traceToolRecords(trace, "governed_actions") {
		action, ok := evaluationTrace(rawAction)
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

func graderCallRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace EvaluationTrace, newID func(string) string) []*evalv1.GraderModelCallRecord {
	records := make([]*evalv1.GraderModelCallRecord, 0)
	for _, rawCall := range traceToolRecords(trace, "grader_calls") {
		call, ok := evaluationTrace(rawCall)
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

func traceSchemaVersion(trace EvaluationTrace) string {
	version, _ := trace["schema_version"].(string)
	if version == "" {
		return "1"
	}
	return version
}

// toolsDeclaredFromTraceCall reads the tool names a model call actually sent.
// An absent or null field is "not reported". A field that is not a list of
// names is an error: recording it as "reported, none declared" would claim the
// harness withheld every tool.
func toolsDeclaredFromTraceCall(call EvaluationTrace) (declared []string, reported bool, err error) {
	raw, present := call["tools_declared"]
	if !present || raw == nil {
		return nil, false, nil
	}
	switch tools := raw.(type) {
	case []string:
		return append([]string{}, tools...), true, nil
	case []any:
		declared = make([]string, 0, len(tools))
		for _, tool := range tools {
			name, ok := tool.(string)
			if !ok {
				return nil, false, fmt.Errorf("tools_declared must list tool names")
			}
			declared = append(declared, name)
		}
		return declared, true, nil
	default:
		return nil, false, fmt.Errorf("tools_declared must be a list of tool names")
	}
}

func optionalUint32FromTraceCall(call EvaluationTrace, name, schemaVersion string) (*uint32, error) {
	if schemaVersion == "1" {
		return nil, nil
	}
	raw, present := call[name]
	if !present || raw == nil {
		return nil, nil
	}
	converted, err := uint32Value(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &converted, nil
}

func requiredUint32FromTraceCall(call EvaluationTrace, name string) (uint32, error) {
	raw, present := call[name]
	if !present {
		return 0, fmt.Errorf("usage_reported call missing %s", name)
	}
	converted, err := uint32Value(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return converted, nil
}

// traceScoredChainClassification is the wire spelling of
// EVALUATION_CALL_CLASSIFICATION_SCORED_CHAIN in a g8ee trace model call.
const traceScoredChainClassification = "scored_chain"

// callClassificationFromTrace reads the chain a producer stated for a model
// call. A missing or unknown value is an error: the importer never guesses a
// chain from the agent role or timing.
func callClassificationFromTrace(call EvaluationTrace) (evalv1.EvaluationCallClassification, error) {
	raw, _ := call["classification"].(string)
	switch raw {
	case traceScoredChainClassification:
		return evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_SCORED_CHAIN, nil
	case "post_turn":
		return evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_POST_TURN, nil
	case "grader":
		return evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_GRADER, nil
	default:
		return evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_UNSPECIFIED,
			fmt.Errorf("evaluation: trace model call classification %q: %w", raw, constants.ErrEvidenceArtifactMalformed)
	}
}

func modelInferenceRecordsFromTrace(assignment *evalv1.EvaluationAssignment, attemptID string, candidate *evalv1.ModelVariant, trace EvaluationTrace, newID func(string) string) ([]*evalv1.ModelInferenceRecord, *uint64, error) {
	schemaVersion := traceSchemaVersion(trace)
	modelCalls, ok := trace["model_calls"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("model_calls must be an array")
	}
	records := make([]*evalv1.ModelInferenceRecord, 0, len(modelCalls))
	var monotonicStarts []uint64
	var monotonicEnds []uint64
	scoredCallsCount := 0
	for _, rawCall := range modelCalls {
		call, ok := evaluationTrace(rawCall)
		if !ok {
			return nil, nil, fmt.Errorf("model call must be an object")
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
		classification, err := callClassificationFromTrace(call)
		if err != nil {
			return nil, nil, err
		}
		if classification == evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_GRADER {
			return nil, nil, fmt.Errorf("evaluation: grader call %q in model_calls: %w", providerAttemptID, constants.ErrEvidenceArtifactMalformed)
		}
		record := &evalv1.ModelInferenceRecord{
			Classification:      classification,
			InferenceRecordId:   newID("inference"),
			ProviderAttemptId:   providerAttemptID,
			AssignmentId:        assignment.GetAssignmentId(),
			EvaluationAttemptId: attemptID,
			ModelRole:           parseModelCampaignRole(call["model_role"]),
			AgentPersona:        stringValue(call["agent_role"]),
			CallSite:            stringValue(call["call_site"]),
			ModelVariant:        candidate,
			InputHash:           stringValue(call["normalized_request_hash"]),
			OutputHash:          stringValue(call["governed_output_hash"]),
			ResultDigest:        stringValue(call["governed_result_digest"]),
			PrivacyAttested:     true,
			UsageAvailability:   evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE,
			FinishReason:        stringValue(call["finish_reason"]),
		}
		declared, reported, err := toolsDeclaredFromTraceCall(call)
		if err != nil {
			return nil, nil, err
		}
		record.ToolsDeclared = declared
		record.ToolsDeclaredReported = reported
		if reported, present := call["usage_reported"]; present {
			value, ok := reported.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("usage_reported must be a boolean")
			}
			if value {
				record.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED
				promptTokens, err := requiredUint32FromTraceCall(call, "input_tokens")
				if err != nil {
					return nil, nil, err
				}
				completionTokens, err := requiredUint32FromTraceCall(call, "output_tokens")
				if err != nil {
					return nil, nil, err
				}
				record.PromptTokens = promptTokens
				record.CompletionTokens = completionTokens
				thinkingTokens, err := optionalUint32FromTraceCall(call, "thinking_tokens", schemaVersion)
				if err != nil {
					return nil, nil, err
				}
				cacheTokens, err := optionalUint32FromTraceCall(call, "cache_tokens", schemaVersion)
				if err != nil {
					return nil, nil, err
				}
				record.ThinkingTokens = thinkingTokens
				record.CacheTokens = cacheTokens
			} else {
				record.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE
			}
		}
		if retry, present := call["retry_count"]; present {
			converted, err := uint32Value(retry)
			if err != nil {
				return nil, nil, fmt.Errorf("retry_count: %w", err)
			}
			if converted > 1000 {
				return nil, nil, fmt.Errorf("retry_count exceeds 1000")
			}
			record.RetryCount = &converted
		}
		if raw, present := call["load_duration_seconds"]; present && raw != nil {
			converted, err := durationSecondsToNanosChecked(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("load_duration_seconds: %w", err)
			}
			record.LoadDurationNanos = converted
			nanos := int64(converted)
			record.LoadState = operatorLoadStateToEvaluation(models.ClassifyLoadState(&nanos))
		}
		for _, duration := range []struct {
			name string
			dest *uint64
		}{{"generation_duration_seconds", &record.GenerationDurationNanos}, {"total_duration_seconds", &record.TotalDurationNanos}} {
			if raw, present := call[duration.name]; present && raw != nil {
				converted, err := durationSecondsToNanosChecked(raw)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: %w", duration.name, err)
				}
				*duration.dest = converted
			}
		}
		if transactionID := stringValue(call["governed_transaction_id"]); transactionID != "" {
			record.GovernedReceiptRef = &compliancev1.ComplianceEvidenceReference{
				ArtifactId:   transactionID,
				ArtifactType: string(complianceevidence.ArtifactTypeEvalReceipt),
				SchemaRef:    "g8e.operator.v1.ActionReceipt",
			}
		}
		if classification == evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_SCORED_CHAIN {
			scoredCallsCount++
			if start, end, complete, err := monotonicCallBounds(call); err != nil {
				return nil, nil, err
			} else if complete {
				monotonicStarts = append(monotonicStarts, start)
				monotonicEnds = append(monotonicEnds, end)
			}
		}
		records = append(records, record)
	}
	var span *uint64
	if scoredCallsCount > 0 && len(monotonicStarts) == scoredCallsCount {
		minStart, maxEnd := monotonicStarts[0], monotonicEnds[0]
		for index := 1; index < len(monotonicStarts); index++ {
			if monotonicStarts[index] < minStart {
				minStart = monotonicStarts[index]
			}
			if monotonicEnds[index] > maxEnd {
				maxEnd = monotonicEnds[index]
			}
		}
		if maxEnd < minStart {
			return nil, nil, fmt.Errorf("scored inference monotonic span is negative")
		}
		value := maxEnd - minStart
		span = &value
	}
	return records, span, nil
}

func policyDecisionRecordsFromTrace(assignment *evalv1.EvaluationAssignment, trace EvaluationTrace) ([]*evalv1.PolicyDecisionRecord, error) {
	raw, present := trace["policy_decisions"]
	if !present {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("policy_decisions must be an array")
	}
	records := make([]*evalv1.PolicyDecisionRecord, 0, len(items))
	for _, item := range items {
		decision, ok := evaluationTrace(item)
		if !ok {
			return nil, fmt.Errorf("policy decision must be an object")
		}
		decisionID, ok := decision["decision_id"].(string)
		if !ok || decisionID == "" {
			return nil, fmt.Errorf("policy decision requires decision_id")
		}
		toolName, ok := decision["tool_name"].(string)
		if !ok {
			return nil, fmt.Errorf("policy decision tool_name must be a string")
		}
		outcome, ok := policyDecisionOutcome(decision["outcome"])
		if !ok {
			return nil, fmt.Errorf("policy decision has unknown outcome")
		}
		detail, ok := decision["detail"].(string)
		if !ok {
			return nil, fmt.Errorf("policy decision detail must be a string")
		}
		records = append(records, &evalv1.PolicyDecisionRecord{DecisionId: decisionID, AssignmentId: assignment.GetAssignmentId(), ToolName: toolName, Outcome: outcome, Detail: detail})
	}
	return records, nil
}

func policyDecisionOutcome(raw any) (evalv1.EvaluationPolicyDecisionOutcome, bool) {
	value, ok := raw.(string)
	if !ok {
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_UNSPECIFIED, false
	}
	switch value {
	case "allow":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW, true
	case "deny":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY, true
	case "refused":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_REFUSED, true
	default:
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_UNSPECIFIED, false
	}
}

func traceFieldCaptured(trace EvaluationTrace, field string) bool {
	_, present := trace[field]
	return present
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

func durationSecondsToNanosChecked(raw any) (uint64, error) {
	seconds, ok := numericFloat(raw)
	if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("must be a finite nonnegative number")
	}
	converted := math.Round(seconds * 1_000_000_000)
	if converted >= float64(^uint64(0)) {
		return 0, fmt.Errorf("is outside uint64 nanosecond range")
	}
	return uint64(converted), nil
}

func uint32Value(raw any) (uint32, error) {
	value, ok := numericFloat(raw)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(^uint32(0)) || math.Trunc(value) != value {
		return 0, fmt.Errorf("must be a finite nonnegative integer")
	}
	return uint32(value), nil
}

func numericFloat(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	case uint32:
		return float64(value), true
	case int32:
		return float64(value), true
	case int16:
		return float64(value), true
	case uint:
		return float64(value), true
	default:
		return 0, false
	}
}

func monotonicCallBounds(call EvaluationTrace) (uint64, uint64, bool, error) {
	rawStart, startPresent := call["monotonic_start"]
	rawEnd, endPresent := call["monotonic_end"]
	if !startPresent && !endPresent {
		return 0, 0, false, nil
	}
	if !startPresent || !endPresent {
		return 0, 0, false, nil
	}
	start, err := durationSecondsToNanosChecked(rawStart)
	if err != nil {
		return 0, 0, false, fmt.Errorf("monotonic_start: %w", err)
	}
	end, err := durationSecondsToNanosChecked(rawEnd)
	if err != nil {
		return 0, 0, false, fmt.Errorf("monotonic_end: %w", err)
	}
	if end < start {
		return 0, 0, false, fmt.Errorf("monotonic_end precedes monotonic_start")
	}
	return start, end, true, nil
}

// BuildAssignmentTraceEvidenceReference returns a content-addressed evidence
// reference for one imported g8ee assignment trace body.
func BuildAssignmentTraceEvidenceReference(runID, assignmentID, attemptID string, trace EvaluationTrace, producedAt time.Time) (*compliancev1.ComplianceEvidenceReference, error) {
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

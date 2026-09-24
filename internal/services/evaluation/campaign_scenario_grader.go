// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ScenarioGradingRequest carries one homogeneous assignment and its catalog
// gold criteria for deterministic scenario grading.
type ScenarioGradingRequest struct {
	AssignmentID   string
	ScenarioID     string
	DesignatedRole string
	GradingMethod  evalv1.EvaluationGradingMethod
	ScenarioInput  ScenarioInputFixture
	ScenarioGold   ScenarioGoldCriteria
	ScenarioTools  ScenarioToolExpectations
	Trace          EvaluationTrace
	Lifecycle      evalv1.EvaluationAssignmentLifecycleStatus
}

// ScenarioGradingResult materializes deterministic and semantic grades plus
// decomposed score records derived from catalog gold criteria.
type ScenarioGradingResult struct {
	DeterministicGrades []*evalv1.DeterministicGrade
	SemanticGrades      []*evalv1.SemanticGrade
	DecomposedScores    []*evalv1.DecomposedScoreRecord
}

var exactFormatPromptPattern = regexp.MustCompile(`(?i)reply with exactly:\s*(\S+)`)

// GradeHomogeneousScenario evaluates one homogeneous model-role assignment
// against its frozen catalog gold criteria and imported g8ee trace.
func GradeHomogeneousScenario(req ScenarioGradingRequest) (*ScenarioGradingResult, error) {
	if req.AssignmentID == "" || req.ScenarioID == "" || len(req.Trace) == 0 {
		return nil, fmt.Errorf("evaluation: grade homogeneous scenario: assignment, scenario, and trace are required")
	}
	result := &ScenarioGradingResult{
		DeterministicGrades: []*evalv1.DeterministicGrade{},
		SemanticGrades:      []*evalv1.SemanticGrade{},
		DecomposedScores:    []*evalv1.DecomposedScoreRecord{},
	}
	roleInvoked := traceRoleInvoked(req.Trace, req.DesignatedRole)
	result.DeterministicGrades = append(result.DeterministicGrades, newDeterministicGrade(req.AssignmentID, "role-invoked", roleInvokedGradeStatus(roleInvoked, req.Lifecycle), roleInvokedDetail(roleInvoked, req.Lifecycle), roleInvokedScore(roleInvoked, req.Lifecycle)))
	result.DeterministicGrades = append(result.DeterministicGrades, gradeRoutingAgreement(req.AssignmentID, req.Trace))
	result.DeterministicGrades = append(result.DeterministicGrades, gradeGovernedInference(req.AssignmentID, req.Trace))
	result.DeterministicGrades = append(result.DeterministicGrades, gradeRoleCriteria(req)...)
	result.DeterministicGrades = append(result.DeterministicGrades, gradePipelineCriteria(req)...)
	result.DeterministicGrades = append(result.DeterministicGrades, gradeRequiredEvidenceTypes(req)...)
	if toolGrade := gradeToolSelection(req); toolGrade != nil {
		result.DeterministicGrades = append(result.DeterministicGrades, toolGrade)
	}
	if policyGrade := gradePolicyExpectation(req); policyGrade != nil {
		result.DeterministicGrades = append(result.DeterministicGrades, policyGrade)
	}
	if contentGrade := gradeScenarioContent(req); contentGrade != nil {
		result.DeterministicGrades = append(result.DeterministicGrades, contentGrade)
	}
	result.SemanticGrades = append(result.SemanticGrades, semanticGradesForRequest(req)...)
	result.DecomposedScores = deriveScenarioDecomposedScores(req.AssignmentID, result.DeterministicGrades)
	return result, nil
}

func gradeRoleCriteria(req ScenarioGradingRequest) []*evalv1.DeterministicGrade {
	grades := make([]*evalv1.DeterministicGrade, 0, len(req.ScenarioGold.RoleCriteria))
	for _, roleCriteria := range req.ScenarioGold.RoleCriteria {
		if roleCriteria.Role != req.DesignatedRole {
			continue
		}
		for _, criterion := range roleCriteria.Criteria {
			grade := newDeterministicGrade(req.AssignmentID, criterion.CriterionID, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, "role responsibility grading requires scenario content evidence", 0)
			if !criterion.Deterministic {
				grade.Detail = "criterion requires semantic judge grading"
				grades = append(grades, grade)
				continue
			}
			contentGrade := gradeScenarioContent(req)
			if contentGrade != nil {
				grade.Status = contentGrade.GetStatus()
				grade.Score = contentGrade.GetScore()
				grade.Detail = contentGrade.GetDetail()
			} else if traceRoleInvoked(req.Trace, req.DesignatedRole) && req.Lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED {
				grade.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
				grade.Score = 1
				grade.Detail = "designated role completed without a scenario-specific content check"
			}
			grades = append(grades, grade)
		}
	}
	return grades
}

func gradePipelineCriteria(req ScenarioGradingRequest) []*evalv1.DeterministicGrade {
	grades := make([]*evalv1.DeterministicGrade, 0, len(req.ScenarioGold.PipelineCriteria))
	for _, pipeline := range req.ScenarioGold.PipelineCriteria {
		if pipeline.Lane != "homogeneous" {
			continue
		}
		for _, criterion := range pipeline.Criteria {
			status := evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
			detail := "homogeneous pipeline did not complete with designated role evidence"
			score := 0.0
			if traceRoleInvoked(req.Trace, req.DesignatedRole) && req.Lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED {
				status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
				detail = "homogeneous pipeline completed with designated role evidence"
				score = 1
			}
			grades = append(grades, newDeterministicGrade(req.AssignmentID, criterion.CriterionID, status, detail, score))
		}
	}
	return grades
}

func gradeRequiredEvidenceTypes(req ScenarioGradingRequest) []*evalv1.DeterministicGrade {
	grades := make([]*evalv1.DeterministicGrade, 0, len(req.ScenarioGold.RequiredEvidenceTypes))
	for _, evidenceType := range req.ScenarioGold.RequiredEvidenceTypes {
		status, detail, score := requiredEvidenceGrade(req, evidenceType)
		grades = append(grades, newDeterministicGrade(req.AssignmentID, "required-evidence:"+evidenceType, status, detail, score))
	}
	return grades
}

func requiredEvidenceGrade(req ScenarioGradingRequest, evidenceType string) (evalv1.EvaluationVerdictStatus, string, float64) {
	switch evidenceType {
	case "model_inference":
		if hasGovernedModelCalls(req.Trace) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "governed model inference evidence is present", 1
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "governed model inference evidence is missing", 0
	case "deterministic_grade":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "deterministic grading executed", 1
	case "tool_decision":
		if hasTraceToolDecisions(req.Trace) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "tool decision evidence is present", 1
		}
		if satisfiesToolDecisionWithoutCall(req) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "forbidden tool was not selected", 1
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "tool decision evidence is missing", 0
	case "tool_call":
		if hasTraceToolCalls(req.Trace) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "tool call evidence is present", 1
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "tool call evidence is missing", 0
	case "governed_action":
		if hasTraceGovernedActions(req.Trace) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "governed action evidence is present", 1
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "governed action evidence is missing", 0
	case "policy_decision":
		if hasTracePolicyDecisions(req.Trace) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "policy decision evidence is present", 1
		}
		if satisfiesPolicyDecisionWithoutCall(req) {
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "policy rejection satisfied without a governed effect", 1
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "policy decision evidence is missing", 0
	case "semantic_grade":
		return requiredSemanticGradeEvidence(req)
	case "final_response":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, evidenceType + " evidence is not yet bound in campaign traces", 0
	default:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, "unknown required evidence type " + evidenceType, 0
	}
}

func gradeRoutingAgreement(assignmentID string, trace EvaluationTrace) *evalv1.DeterministicGrade {
	assignment, ok := evaluationTrace(trace["controlled_role_assignment"])
	if !ok {
		return newDeterministicGrade(assignmentID, "routing-agreement", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, "controlled role assignment is missing from trace", 0)
	}
	agreement, _ := assignment["routing_agreement"].(bool)
	if agreement {
		return newDeterministicGrade(assignmentID, "routing-agreement", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role matches natural triage route", 1)
	}
	return newDeterministicGrade(assignmentID, "routing-agreement", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role intentionally diverged from natural triage route", 1)
}

func gradeGovernedInference(assignmentID string, trace EvaluationTrace) *evalv1.DeterministicGrade {
	if hasGovernedModelCalls(trace) {
		return newDeterministicGrade(assignmentID, "governed-inference", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "at least one governed inference call is recorded", 1)
	}
	return newDeterministicGrade(assignmentID, "governed-inference", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "no governed inference calls are recorded", 0)
}

func gradeScenarioContent(req ScenarioGradingRequest) *evalv1.DeterministicGrade {
	output := strings.TrimSpace(designatedRoleOutput(req.Trace))
	if output == "" {
		return nil
	}
	switch req.ScenarioID {
	case "instruction-exact-format":
		expected := parseExactFormatExpected(req.ScenarioInput.UserPrompt)
		if expected == "" {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "exact-format expectation is missing from scenario input", 0)
		}
		if output == expected {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role output matches the exact required token", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "designated role output does not match the exact required token", 0)
	case "instruction-bounded-count":
		count := countWords(output)
		if count == 3 {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role output contains exactly three words", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, fmt.Sprintf("designated role output contains %d words, expected 3", count), 0)
	case "instruction-classify-severity":
		label := strings.ToUpper(strings.TrimSpace(output))
		if label == "ERROR" || strings.Contains(label, "ERROR") {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role output labels the synthetic log as ERROR", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "designated role output does not label the synthetic log as ERROR", 0)
	case "instruction-constraint-json":
		payload := extractJSONObject(output)
		if len(payload) == 0 {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "designated role output is not valid JSON", 0)
		}
		status, _ := payload["status"].(string)
		code, hasCode := numericValue(payload["code"])
		if strings.EqualFold(status, "ok") && hasCode && code == 200 {
			return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "designated role output matches the requested JSON shape", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "scenario-content", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "designated role output does not match the requested JSON shape", 0)
	default:
		return nil
	}
}

func deriveScenarioDecomposedScores(assignmentID string, grades []*evalv1.DeterministicGrade) []*evalv1.DecomposedScoreRecord {
	if len(grades) == 0 {
		return nil
	}
	passed := 0
	deterministic := 0
	for _, grade := range grades {
		if grade == nil {
			continue
		}
		switch grade.GetStatus() {
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
			deterministic++
			if grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
				passed++
			}
		}
	}
	if deterministic == 0 {
		return nil
	}
	passRate := float64(passed) / float64(deterministic)
	taskScore := 0.0
	if passed == deterministic {
		taskScore = 1.0
	}
	return []*evalv1.DecomposedScoreRecord{
		{
			ScoreId:           assignmentID + ":task-score",
			Dimension:         "task_score",
			Value:             taskScore,
			Unit:              evalv1.EvaluationMetricUnit_EVALUATION_METRIC_UNIT_RATIO,
			Direction:         evalv1.EvaluationMetricDirection_EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER,
			MissingDataPolicy: evalv1.EvaluationMissingDataPolicy_EVALUATION_MISSING_DATA_POLICY_FAIL,
		},
		{
			ScoreId:           assignmentID + ":deterministic-pass-rate",
			Dimension:         "deterministic_pass_rate",
			Value:             passRate,
			Unit:              evalv1.EvaluationMetricUnit_EVALUATION_METRIC_UNIT_RATIO,
			Direction:         evalv1.EvaluationMetricDirection_EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER,
			MissingDataPolicy: evalv1.EvaluationMissingDataPolicy_EVALUATION_MISSING_DATA_POLICY_FAIL,
		},
	}
}

func newDeterministicGrade(assignmentID, criterionID string, status evalv1.EvaluationVerdictStatus, detail string, score float64) *evalv1.DeterministicGrade {
	return &evalv1.DeterministicGrade{
		GradeId:     assignmentID + ":" + criterionID,
		CriterionId: criterionID,
		Status:      status,
		Score:       score,
		Detail:      detail,
	}
}

func traceRoleInvoked(trace EvaluationTrace, designatedRole string) bool {
	roleOutcome, _ := trace["role_outcome"].(string)
	return roleOutcome == "invoked" && designatedRole != ""
}

func roleInvokedGradeStatus(invoked bool, lifecycle evalv1.EvaluationAssignmentLifecycleStatus) evalv1.EvaluationVerdictStatus {
	if invoked && lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED {
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	}
	if lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL {
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	}
	return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
}

func roleInvokedDetail(invoked bool, lifecycle evalv1.EvaluationAssignmentLifecycleStatus) string {
	if invoked && lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED {
		return "designated model role invoked with governed inference evidence"
	}
	if lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL {
		return "designated model role was not invoked"
	}
	return "designated model role was not invoked or assignment did not complete"
}

func roleInvokedScore(invoked bool, lifecycle evalv1.EvaluationAssignmentLifecycleStatus) float64 {
	if invoked && lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED {
		return 1
	}
	return 0
}

func semanticGradesForRequest(req ScenarioGradingRequest) []*evalv1.SemanticGrade {
	if imported := semanticGradesFromTrace(req.AssignmentID, req.Trace); len(imported) > 0 {
		return imported
	}
	if req.GradingMethod == evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE {
		return []*evalv1.SemanticGrade{{
			GradeId:     req.AssignmentID + ":semantic-judge",
			CriterionId: "semantic-judge",
			Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
			Detail:      "semantic judge evidence is missing from campaign trace",
		}}
	}
	return nil
}

func requiredSemanticGradeEvidence(req ScenarioGradingRequest) (evalv1.EvaluationVerdictStatus, string, float64) {
	grades := semanticGradesFromTrace(req.AssignmentID, req.Trace)
	if len(grades) == 0 {
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "semantic grade evidence is missing", 0
	}
	for _, grade := range grades {
		switch grade.GetStatus() {
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "semantic judge grading executed", 1
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, grade.GetDetail(), 0
		}
	}
	return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, "semantic judge grading is unavailable", 0
}

func semanticGradesFromTrace(assignmentID string, trace EvaluationTrace) []*evalv1.SemanticGrade {
	records := traceToolRecords(trace, "semantic_grades")
	grades := make([]*evalv1.SemanticGrade, 0, len(records))
	for _, rawGrade := range records {
		record, ok := evaluationTrace(rawGrade)
		if !ok {
			continue
		}
		gradeID := stringValue(record["grade_id"])
		if gradeID == "" {
			gradeID = assignmentID + ":semantic-judge"
		}
		criterionID := stringValue(record["criterion_id"])
		if criterionID == "" {
			criterionID = "semantic-judge"
		}
		grades = append(grades, &evalv1.SemanticGrade{
			GradeId:        gradeID,
			CriterionId:    criterionID,
			Status:         semanticOutcomeStatus(stringValue(record["status"])),
			JudgeVariantId: stringValue(record["judge_variant_id"]),
			Detail:         stringValue(record["detail"]),
		})
	}
	return grades
}

func semanticOutcomeStatus(outcome string) evalv1.EvaluationVerdictStatus {
	switch outcome {
	case "pass":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	case "fail":
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	default:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE
	}
}

func gradeToolSelection(req ScenarioGradingRequest) *evalv1.DeterministicGrade {
	if len(req.ScenarioTools.ExpectedTools) == 0 && len(req.ScenarioTools.ForbiddenTools) == 0 {
		return nil
	}
	selectedTools := selectedToolNames(req.Trace)
	for _, forbidden := range req.ScenarioTools.ForbiddenTools {
		if selectedTools[forbidden] {
			return newDeterministicGrade(req.AssignmentID, "tool-selection", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "forbidden tool "+forbidden+" was selected", 0)
		}
	}
	for _, expected := range req.ScenarioTools.ExpectedTools {
		if selectedTools[expected] {
			return newDeterministicGrade(req.AssignmentID, "tool-selection", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "expected tool "+expected+" was selected", 1)
		}
	}
	if satisfiesToolDecisionWithoutCall(req) {
		return newDeterministicGrade(req.AssignmentID, "tool-selection", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "forbidden tools were not selected", 1)
	}
	return newDeterministicGrade(req.AssignmentID, "tool-selection", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "expected tool selection evidence is missing", 0)
}

func gradePolicyExpectation(req ScenarioGradingRequest) *evalv1.DeterministicGrade {
	expected := strings.TrimSpace(req.ScenarioGold.PolicyExpectation.ExpectedOutcome)
	if expected == "" || expected == "not_applicable" {
		return nil
	}
	switch expected {
	case "allow":
		if hasTraceGovernedActions(req.Trace) {
			return newDeterministicGrade(req.AssignmentID, "policy-expectation", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "governed action evidence matches allow expectation", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "policy-expectation", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "governed allow evidence is missing", 0)
	case "deny":
		if hasTracePolicyDecisions(req.Trace) || satisfiesPolicyDecisionWithoutCall(req) {
			return newDeterministicGrade(req.AssignmentID, "policy-expectation", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "policy denial evidence is present", 1)
		}
		return newDeterministicGrade(req.AssignmentID, "policy-expectation", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "policy denial evidence is missing", 0)
	default:
		return newDeterministicGrade(req.AssignmentID, "policy-expectation", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, "unknown policy expectation "+expected, 0)
	}
}

func satisfiesToolDecisionWithoutCall(req ScenarioGradingRequest) bool {
	if len(req.ScenarioTools.ForbiddenTools) == 0 {
		return false
	}
	selectedTools := selectedToolNames(req.Trace)
	for _, forbidden := range req.ScenarioTools.ForbiddenTools {
		if selectedTools[forbidden] {
			return false
		}
	}
	return req.Lifecycle == evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
}

func satisfiesPolicyDecisionWithoutCall(req ScenarioGradingRequest) bool {
	if req.ScenarioGold.PolicyExpectation.ExpectedOutcome != "deny" {
		return false
	}
	return satisfiesToolDecisionWithoutCall(req)
}

func selectedToolNames(trace EvaluationTrace) map[string]bool {
	selected := make(map[string]bool)
	for _, rawDecision := range traceToolRecords(trace, "tool_decisions") {
		decision, ok := evaluationTrace(rawDecision)
		if !ok {
			continue
		}
		toolName, _ := decision["tool_name"].(string)
		if toolName == "" {
			continue
		}
		selectedFlag, _ := decision["selected"].(bool)
		if selectedFlag {
			selected[toolName] = true
		}
	}
	return selected
}

func hasTraceToolDecisions(trace EvaluationTrace) bool {
	return len(traceToolRecords(trace, "tool_decisions")) > 0
}

func hasTraceToolCalls(trace EvaluationTrace) bool {
	return len(traceToolRecords(trace, "tool_calls")) > 0
}

func hasTraceGovernedActions(trace EvaluationTrace) bool {
	return len(traceToolRecords(trace, "governed_actions")) > 0
}

func hasTracePolicyDecisions(trace EvaluationTrace) bool {
	return len(traceToolRecords(trace, "policy_decisions")) > 0
}

func traceToolRecords(trace EvaluationTrace, field string) []any {
	records, _ := trace[field].([]any)
	return records
}

func hasGovernedModelCalls(trace EvaluationTrace) bool {
	modelCalls, _ := trace["model_calls"].([]any)
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
		if transactionID, _ := call["governed_transaction_id"].(string); transactionID != "" {
			return true
		}
	}
	return false
}

func designatedRoleOutput(trace EvaluationTrace) string {
	output, _ := trace["designated_role_output"].(string)
	return output
}

func parseExactFormatExpected(prompt string) string {
	match := exactFormatPromptPattern.FindStringSubmatch(prompt)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func countWords(value string) int {
	fields := strings.Fields(value)
	return len(fields)
}

func extractJSONObject(value string) EvaluationTrace {
	trimmed := strings.TrimSpace(value)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return nil
	}
	payload := EvaluationTrace{}
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &payload); err != nil {
		return nil
	}
	return payload
}

func numericValue(raw any) (int64, bool) {
	switch typed := raw.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizeGradesForComparison(grades []*evalv1.DeterministicGrade) []*evalv1.DeterministicGrade {
	if len(grades) == 0 {
		return nil
	}
	clones := make([]*evalv1.DeterministicGrade, 0, len(grades))
	for _, grade := range grades {
		if grade == nil {
			continue
		}
		clones = append(clones, &evalv1.DeterministicGrade{
			GradeId:     grade.GetGradeId(),
			CriterionId: grade.GetCriterionId(),
			Status:      grade.GetStatus(),
			Score:       grade.GetScore(),
			Detail:      grade.GetDetail(),
		})
	}
	return clones
}

func gradesEquivalent(left, right []*evalv1.DeterministicGrade) bool {
	leftNorm := normalizeGradesForComparison(left)
	rightNorm := normalizeGradesForComparison(right)
	if len(leftNorm) != len(rightNorm) {
		return false
	}
	leftByID := make(map[string]*evalv1.DeterministicGrade, len(leftNorm))
	for _, grade := range leftNorm {
		leftByID[grade.GetCriterionId()] = grade
	}
	for _, grade := range rightNorm {
		other := leftByID[grade.GetCriterionId()]
		if other == nil || other.GetStatus() != grade.GetStatus() || other.GetScore() != grade.GetScore() || other.GetDetail() != grade.GetDetail() {
			return false
		}
	}
	return true
}

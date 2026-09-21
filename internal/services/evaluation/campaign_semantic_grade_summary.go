// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func BuildPublicGradeSummarySet(result *evalv1.EvaluationAssignmentResult, scenario *PublicScenarioContext) (*PublicGradeSummarySet, error) {
	if result == nil || scenario == nil {
		return nil, fmt.Errorf("evaluation: build public grade summaries: %w", constants.ErrEvidenceScopeMismatch)
	}
	deterministic := make([]PublicGradeSummary, 0, len(result.GetDeterministicGrades()))
	for _, grade := range result.GetDeterministicGrades() {
		if grade == nil || grade.GetCriterionId() == "" {
			return nil, fmt.Errorf("evaluation: build public grade summaries: %w", constants.ErrEvidenceArtifactMalformed)
		}
		deterministic = append(deterministic, PublicGradeSummary{CriterionID: grade.GetCriterionId(), Status: publicVerdictStatus(grade.GetStatus())})
	}
	semantic := make([]*evalv1.PublicSemanticGradeSummary, 0, len(result.GetSemanticGrades()))
	for _, grade := range result.GetSemanticGrades() {
		if grade == nil || grade.GetCriterionId() == "" {
			return nil, fmt.Errorf("evaluation: build public semantic grade: %w", constants.ErrEvidenceArtifactMalformed)
		}
		publicGrade := BuildPublicSemanticGrade(grade, scenario)
		if publicGrade == nil {
			return nil, fmt.Errorf("evaluation: build public semantic grade: %w", constants.ErrEvidenceArtifactMalformed)
		}
		semantic = append(semantic, publicGrade)
	}
	sort.SliceStable(deterministic, func(i, j int) bool { return deterministic[i].CriterionID < deterministic[j].CriterionID })
	sort.SliceStable(semantic, func(i, j int) bool { return semantic[i].GetCriterionId() < semantic[j].GetCriterionId() })
	return &PublicGradeSummarySet{Deterministic: deterministic, Semantic: semantic}, nil
}

func BuildPublicSemanticGrade(grade *evalv1.SemanticGrade, scenario *PublicScenarioContext) *evalv1.PublicSemanticGradeSummary {
	if grade == nil || scenario == nil || grade.GetCriterionId() == "" {
		return nil
	}
	method := scenario.GradingMethod
	for _, criterion := range scenario.Criteria {
		if criterion != nil && criterion.GetCriterionId() == grade.GetCriterionId() && criterion.GetGradingMethod() != evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED {
			method = criterion.GetGradingMethod()
			break
		}
	}
	explanation := PublicGradeExplanation(grade.GetStatus())
	if grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE {
		explanation = evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_GRADER_UNAVAILABLE
	}
	return &evalv1.PublicSemanticGradeSummary{CriterionId: grade.GetCriterionId(), Status: grade.GetStatus(), GradingMethod: method, JudgeVariantId: grade.GetJudgeVariantId(), ExplanationCode: explanation}
}

func PublicGradeExplanation(status evalv1.EvaluationVerdictStatus) evalv1.PublicGradeExplanationCode {
	switch status {
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
		return evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_CRITERION_PASSED
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
		return evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_CRITERION_FAILED
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED:
		return evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_UNSUPPORTED
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE:
		return evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_INVALID_EVIDENCE
	default:
		return evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_EVIDENCE_UNAVAILABLE
	}
}

func buildPublicGradeSummaries(result *evalv1.EvaluationAssignmentResult) []PublicGradeSummary {
	if result == nil {
		return nil
	}
	out := make([]PublicGradeSummary, 0, len(result.GetDeterministicGrades()))
	for _, grade := range result.GetDeterministicGrades() {
		if grade != nil && grade.GetCriterionId() != "" {
			out = append(out, PublicGradeSummary{CriterionID: grade.GetCriterionId(), Status: publicVerdictStatus(grade.GetStatus())})
		}
	}
	return out
}

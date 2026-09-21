// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

var publicSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var publicEvidenceKinds = map[string]struct{}{
	"campaign_profile": {}, "model_registry": {}, "verification_report": {},
	"evaluation_projection": {}, "comparison_row": {}, "efficiency_observation": {},
	"statistical_analysis": {}, "source_manifest": {},
}

var publicActivityUnavailable = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED

// BuildPublicAssignmentEvidence creates the disclosure-approved activity
// families and copies only already-classified public proof bindings.
func BuildPublicAssignmentEvidence(input PublicAssignmentEvidenceInput) (*PublicAssignmentActivity, []*evalv1.PublicEvidenceBinding, error) {
	if input.Result == nil {
		return nil, nil, fmt.Errorf("evaluation: build public assignment evidence: %w", constants.ErrMissingRequiredField)
	}
	activity, err := buildPublicActivity(input.Result)
	if err != nil {
		return nil, nil, err
	}
	bindings := make([]*evalv1.PublicEvidenceBinding, 0, len(input.PublicProofs))
	seen := make(map[string]struct{}, len(input.PublicProofs))
	if len(input.PublicProofs) > 32 {
		return nil, nil, fmt.Errorf("evaluation: build public assignment evidence: too many evidence bindings: %w", constants.ErrEvidenceArtifactMalformed)
	}
	for _, binding := range input.PublicProofs {
		if binding == nil || !publicSHA256Pattern.MatchString(binding.GetSha256()) || !validPublicSchemaRef(binding.GetSchemaRef()) {
			return nil, nil, fmt.Errorf("evaluation: build public assignment evidence: malformed public proof binding: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if _, ok := publicEvidenceKinds[binding.GetKind()]; !ok {
			return nil, nil, fmt.Errorf("evaluation: build public assignment evidence: unsupported public proof kind %q: %w", binding.GetKind(), constants.ErrEvidenceSchemaMismatch)
		}
		if _, ok := seen[binding.GetSha256()]; ok {
			return nil, nil, fmt.Errorf("evaluation: build public assignment evidence: duplicate public proof binding: %w", constants.ErrEvidenceArtifactMalformed)
		}
		seen[binding.GetSha256()] = struct{}{}
		bindings = append(bindings, &evalv1.PublicEvidenceBinding{Sha256: binding.GetSha256(), SchemaRef: binding.GetSchemaRef(), Kind: binding.GetKind()})
	}
	return activity, bindings, nil
}

func buildPublicActivity(result *evalv1.EvaluationAssignmentResult) (*PublicAssignmentActivity, error) {
	model, err := buildPublicModelActivity(result.GetModelInferences())
	if err != nil {
		return nil, err
	}
	decisions, err := buildPublicToolDecisionActivity(result)
	if err != nil {
		return nil, err
	}
	calls, err := buildPublicToolCallActivity(result)
	if err != nil {
		return nil, err
	}
	policies, err := buildPublicPolicyActivity(result)
	if err != nil {
		return nil, err
	}
	actions, err := buildPublicGovernedActionActivity(result)
	if err != nil {
		return nil, err
	}
	return &PublicAssignmentActivity{Summary: &evalv1.PublicAssignmentActivitySummary{
		ModelActivity: model, ToolDecisions: decisions, ToolCalls: calls,
		PolicyDecisions: policies, GovernedActions: actions,
	}}, nil
}

func buildPublicModelActivity(records []*evalv1.ModelInferenceRecord) (*evalv1.PublicModelActivity, error) {
	family := &evalv1.PublicModelActivity{}
	if len(records) == 0 {
		family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE
		family.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS
		return family, nil
	}
	family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED
	family.Records = make([]*evalv1.PublicModelActivityRecord, 0, len(records))
	for _, record := range records {
		if record == nil || !validModelRole(record.GetModelRole()) || !validPublicLabel(record.GetAgentPersona()) || !validPublicLabel(record.GetModelVariant().GetVariantId()) || record.GetUsageAvailability() > evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: malformed model activity record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if record.GetTotalDurationNanos() > maxPublicDurationMS*1_000_000 || record.GetGenerationDurationNanos() > maxPublicDurationMS*1_000_000 || uint64(record.GetRetryCount()) > maxPublicRetryCount {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: model activity value exceeds bound: %w", constants.ErrEvidenceArtifactMalformed)
		}
		finish, err := publicFinishState(record.GetFinishReason())
		if err != nil {
			return nil, err
		}
		load, err := publicLoadState(record.GetLoadState())
		if err != nil {
			return nil, err
		}
		usage := record.GetUsageAvailability()
		if usage == evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNSPECIFIED {
			usage = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE
		}
		out := &evalv1.PublicModelActivityRecord{ModelRole: record.GetModelRole(), AgentPersona: record.GetAgentPersona(), VariantId: record.GetModelVariant().GetVariantId(), UsageAvailability: usage, TotalDurationNanos: record.GetTotalDurationNanos(), GenerationDurationNanos: record.GetGenerationDurationNanos(), FinishState: finish, LoadState: load}
		if usage == evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED {
			out.InputTokens, out.OutputTokens, out.ThinkingTokens, out.CacheTokens = uint64(record.GetPromptTokens()), uint64(record.GetCompletionTokens()), uint64(record.GetThinkingTokens()), uint64(record.GetCacheTokens())
		}
		if record.RetryCount != nil {
			value := record.GetRetryCount()
			out.RetryCount = &value
		}
		family.Records = append(family.Records, out)
	}
	return family, nil
}

func buildPublicToolDecisionActivity(result *evalv1.EvaluationAssignmentResult) (*evalv1.PublicToolDecisionActivity, error) {
	family := &evalv1.PublicToolDecisionActivity{}
	if !result.GetToolDecisionsCaptured() {
		family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE
		family.UnavailableReason = publicActivityUnavailable
		return family, nil
	}
	family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED
	for _, record := range result.GetToolDecisions() {
		if record == nil || !validPublicLabel(record.GetToolName()) || !validVerdict(record.GetOutcome()) {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: malformed tool decision: %w", constants.ErrEvidenceArtifactMalformed)
		}
		family.Records = append(family.Records, &evalv1.PublicToolDecisionActivityRecord{ToolLabel: record.GetToolName(), Recognized: record.GetRecognized(), Selected: record.GetSelected(), PermissionCompliant: record.GetPermissionCompliant(), Unnecessary: record.GetUnnecessary(), Outcome: record.GetOutcome(), EvidenceSource: evalv1.PublicEvidenceSource_PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED})
	}
	return family, nil
}

func buildPublicToolCallActivity(result *evalv1.EvaluationAssignmentResult) (*evalv1.PublicToolCallActivity, error) {
	family := &evalv1.PublicToolCallActivity{}
	if !result.GetToolCallsCaptured() {
		family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE
		family.UnavailableReason = publicActivityUnavailable
		return family, nil
	}
	family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED
	for _, record := range result.GetToolCalls() {
		if record == nil || !validPublicLabel(record.GetToolName()) || !validVerdict(record.GetSchemaOutcome()) || !validVerdict(record.GetSemanticOutcome()) {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: malformed tool call: %w", constants.ErrEvidenceArtifactMalformed)
		}
		family.Records = append(family.Records, &evalv1.PublicToolCallActivityRecord{ToolLabel: record.GetToolName(), ExecutionOutcome: record.GetSchemaOutcome(), SemanticOutcome: record.GetSemanticOutcome(), EvidenceSource: evalv1.PublicEvidenceSource_PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED})
	}
	return family, nil
}

func buildPublicPolicyActivity(result *evalv1.EvaluationAssignmentResult) (*evalv1.PublicPolicyDecisionActivity, error) {
	family := &evalv1.PublicPolicyDecisionActivity{}
	if !result.GetPolicyDecisionsCaptured() {
		family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE
		family.UnavailableReason = publicActivityUnavailable
		return family, nil
	}
	family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED
	for _, record := range result.GetPolicyDecisions() {
		if record == nil || !validPolicyOutcome(record.GetOutcome()) || !validPublicLabelAllowEmpty(record.GetToolName()) {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: malformed policy decision: %w", constants.ErrEvidenceArtifactMalformed)
		}
		family.Records = append(family.Records, &evalv1.PublicPolicyDecisionActivityRecord{ToolLabel: record.GetToolName(), Outcome: record.GetOutcome(), EvidenceSource: evalv1.PublicEvidenceSource_PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED})
	}
	return family, nil
}

func buildPublicGovernedActionActivity(result *evalv1.EvaluationAssignmentResult) (*evalv1.PublicGovernedActionActivity, error) {
	family := &evalv1.PublicGovernedActionActivity{}
	if !result.GetGovernedActionsCaptured() {
		family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE
		family.UnavailableReason = publicActivityUnavailable
		return family, nil
	}
	family.Availability = evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED
	for _, record := range result.GetGovernedActions() {
		if record == nil {
			return nil, fmt.Errorf("evaluation: build public assignment evidence: nil governed action: %w", constants.ErrEvidenceArtifactMalformed)
		}
		outcome, err := publicPolicyText(record.GetPolicyDecision())
		if err != nil {
			return nil, err
		}
		family.Records = append(family.Records, &evalv1.PublicGovernedActionActivityRecord{ActionLabel: "governed action", ReportedPolicyOutcome: outcome, ReceiptStatus: evalv1.PublicReceiptStatus_PUBLIC_RECEIPT_STATUS_UNAVAILABLE, EvidenceSource: evalv1.PublicEvidenceSource_PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED})
	}
	return family, nil
}

func validPublicLabel(value string) bool {
	return value != "" && validPublicText(value)
}
func validPublicLabelAllowEmpty(value string) bool { return value == "" || validPublicLabel(value) }
func validPublicSchemaRef(value string) bool {
	return validPublicReference(value) && strings.Contains(strings.ToLower(value), "public")
}
func validPublicReference(value string) bool { return value != "" && validPublicText(value) }
func validPublicText(value string) bool {
	return utf8.ValidString(value) && len(value) <= 128 && !strings.ContainsAny(value, "\r\n\t") && !strings.ContainsFunc(value, unicode.IsControl)
}
func validModelRole(value evalv1.ModelCampaignRole) bool {
	return value >= evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY && value <= evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE
}
func validVerdict(value evalv1.EvaluationVerdictStatus) bool {
	switch value {
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE:
		return true
	default:
		return false
	}
}
func validPolicyOutcome(value evalv1.EvaluationPolicyDecisionOutcome) bool {
	return value >= evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW && value <= evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_REFUSED
}

func publicFinishState(value string) (evalv1.PublicFinishState, error) {
	switch strings.ToLower(value) {
	case "stop":
		return evalv1.PublicFinishState_PUBLIC_FINISH_STATE_STOP, nil
	case "length":
		return evalv1.PublicFinishState_PUBLIC_FINISH_STATE_LENGTH, nil
	case "tool_call", "tool-call":
		return evalv1.PublicFinishState_PUBLIC_FINISH_STATE_TOOL_CALL, nil
	case "error":
		return evalv1.PublicFinishState_PUBLIC_FINISH_STATE_ERROR, nil
	case "":
		return evalv1.PublicFinishState_PUBLIC_FINISH_STATE_UNAVAILABLE, nil
	default:
		return 0, fmt.Errorf("evaluation: build public assignment evidence: unsupported finish reason: %w", constants.ErrEvidenceArtifactMalformed)
	}
}
func publicLoadState(value evalv1.EvaluationLoadState) (evalv1.EvaluationLoadState, error) {
	switch value {
	case evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_COLD, evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_WARM, evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNAVAILABLE:
		return value, nil
	case evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNSPECIFIED:
		return evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNAVAILABLE, nil
	default:
		return 0, fmt.Errorf("evaluation: build public assignment evidence: unsupported load state: %w", constants.ErrEvidenceArtifactMalformed)
	}
}
func publicPolicyText(value string) (evalv1.EvaluationPolicyDecisionOutcome, error) {
	switch strings.ToLower(value) {
	case "allow":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW, nil
	case "deny":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY, nil
	case "refused":
		return evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_REFUSED, nil
	default:
		return 0, fmt.Errorf("evaluation: build public assignment evidence: unsupported policy outcome: %w", constants.ErrEvidenceArtifactMalformed)
	}
}

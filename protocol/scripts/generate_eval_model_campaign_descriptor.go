// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	schemaVersion      = "1.0.0"
	protobufPackage    = "g8e.eval.v1"
	outputRelativePath = "descriptors/eval/v1/model_campaign.json"
)

var modelCampaignEnums = []string{
	"EvaluationLane",
	"ModelCampaignRole",
	"EvaluationScenarioCategory",
	"EvaluationAssignmentLifecycleStatus",
	"EvaluationGradingMethod",
	"ModelCapabilityKind",
	"EvaluationLoadState",
	"EvaluationUsageAvailability",
	"EvaluationPolicyDecisionOutcome",
	"PublicGradeExplanationCode",
	"PublicActivityAvailability",
	"PublicUnavailableReason",
	"PublicFinishState",
	"PublicEvidenceSource",
	"PublicVerificationProvenance",
	"PublicReceiptStatus",
	"PublicToolScoreDimension",
}

var modelCampaignMessages = []string{
	"ModelCampaignBinding",
	"EvaluationCampaignSpec",
	"EvaluationScenarioCatalog",
	"EvaluationScenarioDefinition",
	"PublicScenarioCriterion",
	"PublicToolScoreDimensionRequirement",
	"ModelVariant",
	"ModelCapabilityObservation",
	"RoleAssignment",
	"HeterogeneousStackDefinition",
	"EvaluationAssignment",
	"HomogeneousAssignmentTarget",
	"HeterogeneousAssignmentTarget",
	"ModelInferenceRecord",
	"ToolDecisionRecord",
	"PolicyDecisionRecord",
	"ToolCallRecord",
	"EscalationRecord",
	"HandoffRecord",
	"RecoveryRecord",
	"GovernedActionBinding",
	"DeterministicGrade",
	"SemanticGrade",
	"DecomposedScoreRecord",
	"GraderModelCallRecord",
	"EvaluationAssignmentResult",
	"EvaluationVerificationReport",
	"EvaluationVerifiedPopulationEntry",
	"EvaluationVerifiedPopulation",
	"PublicCampaignIdentity",
	"PublicModelVariantIdentity",
	"PublicAssignmentLifecycleRecord",
	"PublicModelCallSummary",
	"PublicScenarioSummary",
	"PublicSemanticGradeSummary",
	"PublicModelActivityRecord",
	"PublicToolDecisionActivityRecord",
	"PublicToolCallActivityRecord",
	"PublicPolicyDecisionActivityRecord",
	"PublicGovernedActionActivityRecord",
	"PublicModelActivity",
	"PublicToolDecisionActivity",
	"PublicToolCallActivity",
	"PublicPolicyDecisionActivity",
	"PublicGovernedActionActivity",
	"PublicAssignmentActivitySummary",
	"PublicEvidenceBinding",
	"PublicVerificationMetadata",
	"PublicAssignmentResultProjection",
}

var publicMessages = map[string]bool{
	"PublicCampaignIdentity":                true,
	"PublicModelVariantIdentity":            true,
	"PublicAssignmentLifecycleRecord":       true,
	"PublicModelCallSummary":                true,
	"PublicScenarioCriterion":               true,
	"PublicToolScoreDimensionRequirement":   true,
	"PublicScenarioSummary":                 true,
	"PublicSemanticGradeSummary":            true,
	"PublicModelActivityRecord":             true,
	"PublicToolDecisionActivityRecord":      true,
	"PublicToolCallActivityRecord":          true,
	"PublicPolicyDecisionActivityRecord":    true,
	"PublicGovernedActionActivityRecord":    true,
	"PublicModelActivity":                   true,
	"PublicToolDecisionActivity":            true,
	"PublicToolCallActivity":                true,
	"PublicPolicyDecisionActivity":          true,
	"PublicGovernedActionActivity":          true,
	"PublicAssignmentActivitySummary":       true,
	"PublicEvidenceBinding":                 true,
	"PublicVerificationMetadata":             true,
	"PublicAssignmentResultProjection":      true,
}

var prohibitedPublicFields = map[string][]string{
	"PublicAssignmentResultProjection": {
		"prompt",
		"output",
		"thinking",
		"receipt",
		"transaction_id",
		"operator_session_id",
		"operator_id",
		"governed_receipt_ref",
		"model_inferences",
		"tool_calls",
		"governed_actions",
	},
}

var canonicalVectors = []map[string]string{
	{"message_type": "EvaluationReport", "vector_path": "vectors/eval/phase1_report.json"},
	{"message_type": "EvaluationCampaignSpec", "vector_path": "vectors/eval/model_campaign_spec.json"},
	{"message_type": "EvaluationAssignmentResult", "vector_path": "vectors/eval/model_assignment_result.json"},
	{"message_type": "PublicAssignmentResultProjection", "vector_path": "vectors/eval/public_assignment_result.json"},
	{"message_type": "EvaluationAssignmentResult", "vector_path": "vectors/eval/model_assignment_result_enriched.json"},
	{"message_type": "PublicAssignmentResultProjection", "vector_path": "vectors/eval/public_assignment_result_enriched.json"},
	{"message_type": "EvaluationVerificationReport", "vector_path": "vectors/eval/run_verification_bound.json"},
}

type descriptorDocument struct {
	SchemaVersion   string                       `json:"schema_version"`
	ProtobufPackage string                       `json:"protobuf_package"`
	Enums           map[string][]string          `json:"enums"`
	Messages        map[string]messageDescriptor `json:"messages"`
	CanonicalVectors []map[string]string         `json:"canonical_vectors"`
}

type messageDescriptor struct {
	Visibility        string   `json:"visibility"`
	Fields            []string `json:"fields"`
	ProhibitedFields  []string `json:"prohibited_fields,omitempty"`
	OneofGroups       []string `json:"oneof_groups,omitempty"`
}

func main() {
	document := descriptorDocument{
		SchemaVersion:    schemaVersion,
		ProtobufPackage:  protobufPackage,
		Enums:            map[string][]string{},
		Messages:         map[string]messageDescriptor{},
		CanonicalVectors: canonicalVectors,
	}

	for _, enumName := range modelCampaignEnums {
		document.Enums[enumName] = enumValues(enumName)
	}
	for _, messageName := range modelCampaignMessages {
		document.Messages[messageName] = messageFields(messageName)
	}

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal descriptor: %v\n", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')

	protocolRoot := filepath.Join("protocol")
	if _, err := os.Stat(protocolRoot); err != nil {
		protocolRoot = "."
	}
	outputPath := filepath.Join(protocolRoot, outputRelativePath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create descriptor directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputPath, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write descriptor: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", outputPath)
}

func enumValues(enumName string) []string {
	enumType, err := protoregistry.GlobalTypes.FindEnumByName(protoreflect.FullName(protobufPackage + "." + enumName))
	if err != nil {
		panic(err)
	}
	values := enumType.Descriptor().Values()
	names := make([]string, 0, values.Len())
	for index := 0; index < values.Len(); index++ {
		names = append(names, string(values.Get(index).Name()))
	}
	sort.Strings(names)
	return names
}

func messageFields(messageName string) messageDescriptor {
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(protobufPackage + "." + messageName))
	if err != nil {
		panic(err)
	}
	fields := messageType.Descriptor().Fields()
	fieldNames := make([]string, 0, fields.Len())
	oneofGroups := make([]string, 0)
	for index := 0; index < fields.Len(); index++ {
		field := fields.Get(index)
		if field.ContainingOneof() != nil && !field.ContainingOneof().IsSynthetic() {
			oneofName := string(field.ContainingOneof().Name())
			if !contains(oneofGroups, oneofName) {
				oneofGroups = append(oneofGroups, oneofName)
			}
			continue
		}
		fieldNames = append(fieldNames, string(field.Name()))
	}
	sort.Strings(fieldNames)
	sort.Strings(oneofGroups)

	visibility := "private"
	if publicMessages[messageName] {
		visibility = "public"
	}
	descriptor := messageDescriptor{
		Visibility: visibility,
		Fields:     fieldNames,
	}
	if prohibited := prohibitedPublicFields[messageName]; len(prohibited) > 0 {
		descriptor.ProhibitedFields = append([]string(nil), prohibited...)
	}
	if len(oneofGroups) > 0 {
		descriptor.OneofGroups = oneofGroups
	}
	return descriptor
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

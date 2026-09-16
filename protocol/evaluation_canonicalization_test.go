// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package protocol_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type evaluationCanonicalizationVector struct {
	MessageType   string `json:"message_type"`
	CanonicalJSON string `json:"canonical_json"`
}

//go:embed vectors/eval/phase1_report.json
var evaluationCanonicalizationVectorJSON []byte

//go:embed vectors/eval/model_campaign_spec.json
var modelCampaignSpecVectorJSON []byte

//go:embed vectors/eval/model_assignment_result.json
var modelAssignmentResultVectorJSON []byte

//go:embed vectors/eval/public_assignment_result.json
var publicAssignmentResultVectorJSON []byte

func TestEvaluationReportCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector evaluationCanonicalizationVector
	require.NoError(t, json.Unmarshal(evaluationCanonicalizationVectorJSON, &vector))
	assert.Equal(t, "EvaluationReport", vector.MessageType)

	report := &evalv1.EvaluationReport{}
	encoded := []byte(vector.CanonicalJSON)
	require.NoError(t, evalv1.UnmarshalCanonical(encoded, report))

	canonical, err := evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	assert.Equal(t, encoded, canonical)

	reparsed := &evalv1.EvaluationReport{}
	require.NoError(t, evalv1.UnmarshalCanonical(canonical, reparsed))
	assert.True(t, proto.Equal(report, reparsed))
}

func TestEvaluationCanonicalParserRejectsUnknownAndNoncanonicalJSON(t *testing.T) {
	tests := []struct {
		name    string
		encoded []byte
	}{
		{name: "unknown field is rejected", encoded: []byte(`{"schema_version":"1.0.0","unknown":true}`)},
		{name: "whitespace is rejected", encoded: []byte("{\n  \"schema_version\": \"1.0.0\"\n}")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evalv1.UnmarshalCanonical(tt.encoded, &evalv1.EvaluationReport{})
			require.Error(t, err)
		})
	}
}

func TestEvaluationCampaignSpecCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector evaluationCanonicalizationVector
	require.NoError(t, json.Unmarshal(modelCampaignSpecVectorJSON, &vector))
	assert.Equal(t, "EvaluationCampaignSpec", vector.MessageType)

	spec := &evalv1.EvaluationCampaignSpec{}
	encoded := []byte(vector.CanonicalJSON)
	require.NoError(t, evalv1.UnmarshalCanonical(encoded, spec))

	canonical, err := evalv1.MarshalCanonical(spec)
	require.NoError(t, err)
	assert.Equal(t, encoded, canonical)
	assert.Equal(t, "phase1a-smoke", spec.GetCampaignId())
	assert.Equal(t, uint32(25), spec.GetScenarioCount())
}

func TestEvaluationAssignmentResultCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector evaluationCanonicalizationVector
	require.NoError(t, json.Unmarshal(modelAssignmentResultVectorJSON, &vector))
	assert.Equal(t, "EvaluationAssignmentResult", vector.MessageType)

	result := &evalv1.EvaluationAssignmentResult{}
	encoded := []byte(vector.CanonicalJSON)
	require.NoError(t, evalv1.UnmarshalCanonical(encoded, result))

	canonical, err := evalv1.MarshalCanonical(result)
	require.NoError(t, err)
	assert.Equal(t, encoded, canonical)
	assert.Equal(t, evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE, result.GetLane())
	assert.Len(t, result.GetModelInferences(), 1)
}

func TestPublicAssignmentResultProjectionCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector evaluationCanonicalizationVector
	require.NoError(t, json.Unmarshal(publicAssignmentResultVectorJSON, &vector))
	assert.Equal(t, "PublicAssignmentResultProjection", vector.MessageType)

	projection := &evalv1.PublicAssignmentResultProjection{}
	encoded := []byte(vector.CanonicalJSON)
	require.NoError(t, evalv1.UnmarshalCanonical(encoded, projection))

	canonical, err := evalv1.MarshalCanonical(projection)
	require.NoError(t, err)
	assert.Equal(t, encoded, canonical)
	assert.Equal(t, "assign-1", projection.GetAssignmentId())
}

func TestEvaluationProtocolRegistersNativeRecordSet(t *testing.T) {
	requiredMessages := []string{
		"EvaluationRun",
		"EvaluationAttempt",
		"EvaluationObservation",
		"EvaluationAssertion",
		"EvaluationVerdict",
		"EvaluationMetric",
		"EvaluationReport",
		"EvaluationCampaignSpec",
		"EvaluationScenarioCatalog",
		"EvaluationScenarioDefinition",
		"ModelVariant",
		"EvaluationAssignment",
		"EvaluationAssignmentResult",
		"ModelInferenceRecord",
		"GovernedActionBinding",
		"EvaluationVerificationReport",
		"PublicAssignmentResultProjection",
	}
	for _, name := range requiredMessages {
		t.Run(name, func(t *testing.T) {
			messageType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("g8e.eval.v1." + name))
			require.NoError(t, err)
			assert.NotNil(t, messageType)
		})
	}
}

func TestEvaluationProtocolUsesExternalVerificationArtifactAndTypedRegistryFields(t *testing.T) {
	reportFields := (&evalv1.EvaluationReport{}).ProtoReflect().Descriptor().Fields()
	assert.Nil(t, reportFields.ByName("verification_report"))

	runFields := (&evalv1.EvaluationRun{}).ProtoReflect().Descriptor().Fields()
	assert.Equal(t, protoreflect.EnumKind, runFields.ByName("active_posture").Kind())

	deploymentFields := (&evalv1.EvaluationDeploymentIdentity{}).ProtoReflect().Descriptor().Fields()
	assert.Equal(t, protoreflect.MessageKind, deploymentFields.ByName("topology_ref").Kind())

	metricFields := (&evalv1.EvaluationMetric{}).ProtoReflect().Descriptor().Fields()
	assert.Equal(t, protoreflect.EnumKind, metricFields.ByName("unit").Kind())
	assert.Equal(t, protoreflect.MessageKind, metricFields.ByName("eligible_population_ref").Kind())
}

func TestEvaluationLaneIncludesModelCampaignLanes(t *testing.T) {
	assert.Equal(t, evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE, evalv1.EvaluationLane(2))
	assert.Equal(t, evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM, evalv1.EvaluationLane(3))
}

func TestPublicAssignmentProjectionOmitsPrivateEvidenceFields(t *testing.T) {
	forbidden := []string{
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
	}
	fields := (&evalv1.PublicAssignmentResultProjection{}).ProtoReflect().Descriptor().Fields()
	for _, name := range forbidden {
		assert.Nil(t, fields.ByName(protoreflect.Name(name)), "public projection must not expose %q", name)
	}
}

func TestEvaluationAssignmentResultRetainsPrivateInferenceBindings(t *testing.T) {
	fields := (&evalv1.EvaluationAssignmentResult{}).ProtoReflect().Descriptor().Fields()
	assert.NotNil(t, fields.ByName("model_inferences"))
	assert.NotNil(t, fields.ByName("governed_actions"))
	assert.NotNil(t, fields.ByName("result_digest"))
}

func TestEvaluationValuePreservesTypedArtifactReference(t *testing.T) {
	reference := &compliancev1.ComplianceEvidenceReference{
		ArtifactId:   "action-receipt:sha256:abc",
		ArtifactType: "action-receipt",
		Sha256:       "abc",
	}
	value := &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_ArtifactReference{ArtifactReference: reference}}

	encoded, err := evalv1.MarshalCanonical(value)
	require.NoError(t, err)
	reparsed := &evalv1.EvaluationValue{}
	require.NoError(t, evalv1.UnmarshalCanonical(encoded, reparsed))
	assert.True(t, proto.Equal(value, reparsed))
	assert.Equal(t, reference.GetArtifactId(), reparsed.GetArtifactReference().GetArtifactId())
}

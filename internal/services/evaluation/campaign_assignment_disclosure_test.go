// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestValidatePublicAssignmentRecordAcceptsHistoricalAndEnrichedRecords(t *testing.T) {
	projection := validDisclosureProjection(t)

	tests := []struct {
		name       string
		version    string
		extensions map[string]any
	}{
		{name: "historical", version: campaignProjectionEnvelopeHistoricalVersion},
		{
			name:    "enriched",
			version: campaignProjectionEnvelopeEnrichedVersion,
			extensions: map[string]any{
				"resource_summary": map[string]any{
					"latency_ms": map[string]any{"value": 0},
					"retries":    map[string]any{"unavailable_reason": "no_scored_calls"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := assignmentEnvelope(t, tt.version, projection, tt.extensions)
			require.NoError(t, ValidatePublicAssignmentRecord("", body))
		})
	}
}

func TestValidatePublicAssignmentRecordRejectsMalformedAssignmentGrammar(t *testing.T) {
	projection := validDisclosureProjection(t)
	tests := []struct {
		name       string
		version    string
		extensions map[string]any
		mutate     func(map[string]any)
		want       error
	}{
		{
			name:    "unknown projection field",
			version: campaignProjectionEnvelopeEnrichedVersion,
			mutate: func(record map[string]any) {
				record["private_detail"] = "should reject"
			},
			want: constants.ErrEvidenceSchemaMismatch,
		},
		{
			name:    "private grade detail",
			version: campaignProjectionEnvelopeEnrichedVersion,
			extensions: map[string]any{
				"benchmark_observations": map[string]any{
					"grade_summaries": []any{map[string]any{"criterion_id": "criterion", "detail": "private grader output"}},
				},
			},
			want: constants.ErrPublicFeedRestrictedField,
		},
		{
			name:    "historical enriched extension",
			version: campaignProjectionEnvelopeHistoricalVersion,
			extensions: map[string]any{
				"resource_summary": map[string]any{"latency_ms": map[string]any{"value": 1}},
			},
			want: constants.ErrEvidenceSchemaMismatch,
		},
		{
			name:    "restricted string carrier",
			version: campaignProjectionEnvelopeHistoricalVersion,
			mutate: func(record map[string]any) {
				record["scenario_id"] = "spiffe://g8e.local/user/private"
			},
			want: constants.ErrPublicFeedRestrictedField,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := assignmentEnvelope(t, tt.version, projection, tt.extensions)
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(body, &envelope))
			record := envelope["record"].(map[string]any)
			if tt.mutate != nil {
				tt.mutate(record)
			}
			envelope["record"] = record
			body, err := json.Marshal(envelope)
			require.NoError(t, err)
			err = ValidatePublicAssignmentRecord("", body)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidatePublicAssignmentRecordLeavesOtherPublicProjectionsToGeneralValidation(t *testing.T) {
	require.NoError(t, ValidatePublicAssignmentRecord("", []byte(`{"kind":"model_summary","detail":"not an assignment envelope"}`)))
	assert.Error(t, ValidatePublicAssignmentRecord("", []byte(`{"schema_version":"1.1.0","message_type":"UnknownCampaignRecord","record":{}}`)))
}

func validDisclosureProjection(t *testing.T) map[string]any {
	t.Helper()
	digest := sha256.Sum256([]byte("result"))
	projection := &evalv1.PublicAssignmentResultProjection{
		AssignmentId:     "assignment-1",
		RunId:            "run-1",
		ScenarioId:       "scenario-1",
		ScenarioCategory: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
		Lane:             evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus:  evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		SummaryStatus:    evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		ResultDigest:     hex.EncodeToString(digest[:]),
	}
	encoded, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(projection)
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(encoded, &record))
	return record
}

func assignmentEnvelope(t *testing.T, version string, projection map[string]any, extensions map[string]any) []byte {
	t.Helper()
	record := make(map[string]any, len(projection)+len(extensions))
	for key, value := range projection {
		record[key] = value
	}
	for key, value := range extensions {
		record[key] = value
	}
	body, err := json.Marshal(map[string]any{
		"schema_version":  version,
		"message_type":    publicMessageTypeAssignmentResult,
		"idempotency_key": "run-1:assignment-1:result",
		"record":          record,
	})
	require.NoError(t, err)
	return body
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildAssignmentLifecycleProjection(t *testing.T) {
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		ScenarioId:      "instruction-exact-format",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		Repetition:      1,
		QueuedAt:        timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			},
		},
	}
	projection, err := BuildAssignmentLifecycleProjection(assignment, evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, "assign-1", projection.GetAssignmentId())
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, projection.GetDesignatedRole())
	assert.Equal(t, "qwen3-4b", projection.GetVariantId())
}

func TestMarshalCampaignProjectionEnvelope(t *testing.T) {
	projection := &evalv1.PublicAssignmentLifecycleRecord{
		AssignmentId:     "assign-1",
		RunId:            "run-1",
		ScenarioId:       "instruction-exact-format",
		ScenarioCategory: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
		Lane:             evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus:  evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		ObservedAt:       timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
	body, err := MarshalCampaignProjectionEnvelope(publicMessageTypeAssignmentLifecycle, "run-1:assign-1:lifecycle:queued", projection)
	require.NoError(t, err)
	envelope := CampaignProjectionEnvelope{}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, publicMessageTypeAssignmentLifecycle, envelope.MessageType)
	assert.Equal(t, "run-1:assign-1:lifecycle:queued", envelope.IdempotencyKey)
	assert.NotEmpty(t, envelope.Record)
}

func TestDerivePublicSummaryStatus(t *testing.T) {
	pass := &evalv1.EvaluationAssignmentResult{
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		}},
	}
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, DerivePublicSummaryStatus(pass))

	fail := &evalv1.EvaluationAssignmentResult{
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL,
	}
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, DerivePublicSummaryStatus(fail))
}

func TestBuildAssignmentResultProjection(t *testing.T) {
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId: "assign-1",
		RunId:        "run-1",
		ScenarioId:   "instruction-exact-format",
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			},
		},
	}
	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		ResultDigest:    "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		CompletedAt:     timestamppb.New(time.Unix(1_700_000_100, 0).UTC()),
	}
	projection, err := BuildAssignmentResultProjection(
		assignment,
		result,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		"unverified",
	)
	require.NoError(t, err)
	assert.Equal(t, result.GetResultDigest(), projection.GetResultDigest())
	assert.Equal(t, "unverified", projection.GetVerificationStatus())
}

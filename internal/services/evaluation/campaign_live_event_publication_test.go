// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License 1.1 —
// see LICENSE for details.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildAssignmentPassMetricSignal_IsBinaryPassNotRate(t *testing.T) {
	assignment := &evalv1.EvaluationAssignment{
		RunId:        "run-live-1",
		AssignmentId: "assign-1",
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			},
		},
	}
	cases := []struct {
		name  string
		grade evalv1.EvaluationVerdictStatus
		want  float64
	}{
		{"passed", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, 1},
		{"failed", evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &evalv1.EvaluationAssignmentResult{
				LifecycleStatus:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
				DeterministicGrades: []*evalv1.DeterministicGrade{{CriterionId: "trajectory", Status: tc.grade}},
			}
			signal, ok, err := buildAssignmentPassMetricSignal(assignment, result, "2026-10-02T12:00:00.000Z", 1, 5)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, assignmentPassMetricID, signal.MetricID, "the per-assignment verdict is `pass`, never the run-level `pass_rate`")
			require.NotNil(t, signal.Rate)
			assert.InDelta(t, tc.want, *signal.Rate, 0)
			assert.Equal(t, 1, signal.Denominator)
		})
	}
}

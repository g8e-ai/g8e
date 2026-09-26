// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package evaluation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildPublicAssignmentEvidence_EmitsEmptyActivityRecordsOnWire(t *testing.T) {
	activity, _, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{
		Result: &evalv1.EvaluationAssignmentResult{},
	})
	require.NoError(t, err)
	require.NotNil(t, activity.Summary)

	record, err := MarshalPublicAssignmentRecord(&PublicAssignmentRecord{
		Projection: &evalv1.PublicAssignmentResultProjection{
			AssignmentId:       "assign-1",
			RunId:              "run-1",
			ScenarioId:         "scenario-1",
			LifecycleStatus:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			SummaryStatus:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			ResultDigest:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			VerificationStatus: "verified",
			ActivitySummary:    activity.Summary,
		},
	})
	require.NoError(t, err)

	var payload struct {
		ActivitySummary publicActivitySummaryWire `json:"activity_summary"`
	}
	require.NoError(t, json.Unmarshal(record, &payload))
	for familyName, familyRaw := range map[string]json.RawMessage{
		"model_activity":   payload.ActivitySummary.ModelActivity,
		"tool_decisions":   payload.ActivitySummary.ToolDecisions,
		"tool_calls":       payload.ActivitySummary.ToolCalls,
		"policy_decisions": payload.ActivitySummary.PolicyDecisions,
		"governed_actions": payload.ActivitySummary.GovernedActions,
	} {
		var family publicActivityFamilyWire
		require.NoError(t, json.Unmarshal(familyRaw, &family), familyName)
		assert.Equal(t, json.RawMessage("[]"), family.Records, "%s should include records array on wire", familyName)
	}
}

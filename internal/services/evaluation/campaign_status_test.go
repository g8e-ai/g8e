// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignRunStatus(t *testing.T) {
	t.Run("initialized when no assignments are scheduled", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{ExpectedAssignment: 75}, nil)
		assert.Equal(t, "initialized", status)
	})

	t.Run("scheduled when assignments are queued but not started", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			QueuedCount:        75,
		}, nil)
		assert.Equal(t, "scheduled", status)
	})

	t.Run("running when an assignment is active", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			QueuedCount:        10,
			RunningCount:       1,
			TerminalCount:      64,
		}, nil)
		assert.Equal(t, "running", status)
	})

	t.Run("in_progress when partially complete", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			QueuedCount:        10,
			TerminalCount:      65,
		}, nil)
		assert.Equal(t, "in_progress", status)
	})

	t.Run("completed when all assignments are terminal", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			TerminalCount:      75,
		}, nil)
		assert.Equal(t, "completed", status)
	})

	t.Run("verified when verification passed", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			TerminalCount:      75,
		}, &evalv1.EvaluationVerificationReport{
			Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		})
		assert.Equal(t, "verified", status)
	})

	t.Run("verify_failed when verification failed", func(t *testing.T) {
		status := CampaignRunStatus(&CampaignRunSummary{
			ExpectedAssignment: 75,
			TerminalCount:      75,
		}, &evalv1.EvaluationVerificationReport{
			Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		})
		assert.Equal(t, "verify_failed", status)
	})
}

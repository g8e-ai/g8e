// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignRunStatus derives a human-readable campaign run status from canonical
// assignment counts, an optional persisted verification report, and whether a
// live process holds the run's lease.
//
// A run is "running" only while its lease holder is alive. An assignment left
// running with no live holder, as after a crash or a restored backup, reports
// "interrupted".
func CampaignRunStatus(summary *CampaignRunSummary, verification *evalv1.EvaluationVerificationReport, executing bool) string {
	if summary == nil {
		return "unknown"
	}
	if verification != nil {
		switch verification.GetStatus() {
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
			return "verified"
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
			return "verify_failed"
		default:
			return strings.ToLower(strings.TrimPrefix(verification.GetStatus().String(), "EVALUATION_VERDICT_STATUS_"))
		}
	}
	scheduled := summary.QueuedCount + summary.RunningCount + summary.TerminalCount
	if scheduled == 0 {
		return "initialized"
	}
	if executing {
		return "running"
	}
	if summary.StoppedCount > 0 && summary.QueuedCount == 0 && summary.RunningCount == 0 {
		return "cancelled"
	}
	if summary.RunningCount > 0 {
		return "interrupted"
	}
	if summary.ExpectedAssignment > 0 && uint64(summary.TerminalCount) >= summary.ExpectedAssignment {
		return "completed"
	}
	if summary.TerminalCount == 0 && summary.QueuedCount > 0 {
		return "scheduled"
	}
	return "in_progress"
}

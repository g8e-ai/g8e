// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// runHolderJSON identifies the live process executing a run.
type runHolderJSON struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	StartedAt string `json:"started_at"`
	LogPath   string `json:"log_path,omitempty"`
}

// runRow is the resumable state of one run as commands report it.
type runRow struct {
	RunID               string         `json:"run_id"`
	CampaignID          string         `json:"campaign_id"`
	Status              string         `json:"status"`
	Lane                string         `json:"lane"`
	StartedAt           string         `json:"started_at,omitempty"`
	ExpectedAssignments uint64         `json:"expected_assignments"`
	Queued              uint32         `json:"queued"`
	Running             uint32         `json:"running"`
	Terminal            uint32         `json:"terminal"`
	Stopped             uint32         `json:"stopped"`
	NextAssignmentID    string         `json:"next_assignment_id,omitempty"`
	Archived            bool           `json:"archived"`
	Holder              *runHolderJSON `json:"holder,omitempty"`
}

// summarizeRun reads a run's canonical assignment records, its persisted
// verification, and its lease. The status is "running" only while a live
// process holds the lease.
func summarizeRun(ctx context.Context, store *evaluation.Store, archived bool, runID string, live evaluation.LeaseLiveness, now func() time.Time, newID func(string) string) (*runRow, *evaluation.CampaignRunSummary, error) {
	summary, err := evaluation.NewCampaignController(store, nil, now, newID).RunSummary(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	verification, err := store.LoadCampaignVerification(ctx, runID)
	if err != nil {
		verification = nil
	}
	row := &runRow{
		RunID:               runID,
		CampaignID:          summary.Run.GetCampaignBinding().GetCampaignId(),
		Lane:                laneLabel(summary.Run.GetLane()),
		ExpectedAssignments: summary.ExpectedAssignment,
		Queued:              summary.QueuedCount,
		Running:             summary.RunningCount,
		Terminal:            summary.TerminalCount,
		Stopped:             summary.StoppedCount,
		NextAssignmentID:    summary.NextAssignmentID,
		Archived:            archived,
	}
	if summary.Run.GetStartedAt() != nil {
		row.StartedAt = summary.Run.GetStartedAt().AsTime().Format(time.RFC3339)
	}
	executing := false
	if !archived {
		lease, leaseErr := store.LoadRunLease(ctx, runID)
		switch {
		case leaseErr == nil:
			if live != nil && live(*lease) {
				executing = true
				row.Holder = &runHolderJSON{PID: lease.PID, Host: lease.Host, StartedAt: lease.StartedAt.Format(time.RFC3339), LogPath: lease.LogPath}
			}
		case !errors.Is(leaseErr, constants.ErrNotFound):
			return nil, nil, fmt.Errorf("evaluation: run lease: %w", leaseErr)
		}
	}
	row.Status = evaluation.CampaignRunStatus(summary, verification, executing)
	return row, summary, nil
}

func laneLabel(lane evalv1.EvaluationLane) string {
	switch lane {
	case evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM:
		return campaignLaneSystem
	default:
		return campaignLaneModelRole
	}
}

const (
	campaignLaneModelRole = "model-role"
	campaignLaneSystem    = "system"
)

// terminalRunStatus reports whether a run status will not change without an
// operator acting.
func terminalRunStatus(status string) bool {
	switch status {
	case "verified", "verify_failed", "completed", "cancelled", "interrupted":
		return true
	default:
		return false
	}
}

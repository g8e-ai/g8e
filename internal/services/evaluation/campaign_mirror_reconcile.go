// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"time"
)

const (
	CampaignMirrorDefaultRunTimeout       = 5 * time.Minute
	CampaignMirrorRunTimeoutPerAssignment = 2 * time.Second
	CampaignMirrorRunTimeoutMax           = 20 * time.Minute
)

// CampaignMirrorReconcileRunTimeout scales the per-run deadline with assignment
// volume so large verified runs can finish catch-up publication.
func CampaignMirrorReconcileRunTimeout(base time.Duration, assignmentCount int) time.Duration {
	if base <= 0 {
		base = CampaignMirrorDefaultRunTimeout
	}
	if assignmentCount <= 0 {
		return base
	}
	scaled := base + time.Duration(assignmentCount)*CampaignMirrorRunTimeoutPerAssignment
	if scaled > CampaignMirrorRunTimeoutMax {
		return CampaignMirrorRunTimeoutMax
	}
	return scaled
}

func assignmentCountForMirrorTimeout(ctx context.Context, store CampaignStore, runID string) int {
	if store == nil || runID == "" {
		return 0
	}
	concrete, ok := store.(*Store)
	if !ok {
		return 0
	}
	count, err := concrete.CountAssignments(ctx, runID)
	if err != nil {
		return 0
	}
	return count
}

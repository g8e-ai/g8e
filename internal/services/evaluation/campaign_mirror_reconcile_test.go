// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignMirrorReconcileRunTimeout_ScalesWithAssignmentCount(t *testing.T) {
	base := 5 * time.Minute
	assert.Equal(t, base, CampaignMirrorReconcileRunTimeout(base, 0))
	assert.Equal(t, 7*time.Minute, CampaignMirrorReconcileRunTimeout(base, 60))
	assert.Equal(t, CampaignMirrorRunTimeoutMax, CampaignMirrorReconcileRunTimeout(base, 10_000))
}

func TestCampaignMirrorReconciler_ReconcileVerifiedQueuePresenceOnlyReportsMissing(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)

	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: run.GetRunId()}}}
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, false, false, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.MissingRunIDs)
	assert.Empty(t, result.RestoredRunIDs)
}

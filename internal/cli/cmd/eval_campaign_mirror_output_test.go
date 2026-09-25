// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestWriteCampaignMirrorRestoreQueueResult(t *testing.T) {
	var stdout, stderr bytes.Buffer
	writeCampaignMirrorRestoreQueueResult(&stdout, &stderr, &evaluation.CampaignMirrorReconcileResult{
		RestoredRunIDs:   []string{"run-1"},
		PublishedRecords: 12,
		SkippedRunIDs:    []string{"run-2"},
		HostAbsentRunIDs: []string{"run-3"},
		FailedRuns:       map[string]string{"run-4": "mirror unavailable"},
	}, false)
	out := stdout.String()
	assert.Contains(t, out, "Restored 1 verified dataset(s)")
	assert.Contains(t, out, "already present")
	assert.Contains(t, out, "local campaign artifacts")
	assert.Contains(t, out, "[run-3]")
	assert.Contains(t, stderr.String(), "mirror restore failed for run-4")
}

func TestWriteCampaignMirrorRestoreRunResult(t *testing.T) {
	var stdout bytes.Buffer
	writeCampaignMirrorRestoreRunResult(&stdout, "run-1", 0, false)
	assert.Contains(t, stdout.String(), "already present")
	stdout.Reset()
	writeCampaignMirrorRestoreRunResult(&stdout, "run-1", 3, false)
	assert.Contains(t, stdout.String(), "Restored run run-1")
	stdout.Reset()
	writeCampaignMirrorRestoreRunResult(&stdout, "run-1", 3, true)
	assert.Contains(t, stdout.String(), "Republished run run-1")
}

func TestWriteCampaignMirrorRestoreQueueResult_ForceRepublish(t *testing.T) {
	var stdout, stderr bytes.Buffer
	writeCampaignMirrorRestoreQueueResult(&stdout, &stderr, &evaluation.CampaignMirrorReconcileResult{
		RepublishedRunIDs: []string{"run-1", "run-2"},
		PublishedRecords:  24,
	}, true)
	assert.Contains(t, stdout.String(), "Republished 2 verified dataset(s)")
}

func TestWriteCampaignMirrorRestoreInitSummary(t *testing.T) {
	var stdout bytes.Buffer
	writeCampaignMirrorRestoreInitSummary(&stdout, &evaluation.CampaignMirrorReconcileResult{
		RestoredRunIDs:   []string{"run-1"},
		HostAbsentRunIDs: []string{"run-2"},
	})
	assert.Contains(t, stdout.String(), "Restored 1 verified dataset(s)")
	assert.Contains(t, stdout.String(), "local campaign artifacts")

	stdout.Reset()
	writeCampaignMirrorRestoreInitSummary(&stdout, &evaluation.CampaignMirrorReconcileResult{
		SkippedRunIDs: []string{"run-1", "run-2"},
	})
	assert.Contains(t, stdout.String(), "already contains 2 restorable verified dataset(s)")
}

func TestFormatRunIDList(t *testing.T) {
	assert.Equal(t, "[]", formatRunIDList(nil))
	assert.Equal(t, "[run-1 run-2]", formatRunIDList([]string{"run-1", "run-2"}))
}

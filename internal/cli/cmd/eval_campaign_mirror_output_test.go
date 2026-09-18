// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestWriteCampaignMirrorRestoreQueueResult_AllAlreadyPresent(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	writeCampaignMirrorRestoreQueueResult(&stdout, &stderr, &evaluation.CampaignMirrorReconcileResult{
		SkippedRunIDs: []string{
			"eval-init-gemma2-9b-1789729690",
			"eval-init-gemma3-1b-1789733152",
			"eval-init-gemma3-270m-1789739892",
		},
		HostAbsentRunIDs: []string{"eval-init-deepseek-r1-7b-1789674361"},
	})
	out := stdout.String()
	assert.Contains(t, out, "already present in public mirror (nothing to do)")
	assert.NotContains(t, out, "Restored 0")
	assert.Contains(t, out, "Do not run mirror restore again")
	assert.Contains(t, out, "eval campaign start --queue")
	assert.Contains(t, out, "Skipped run IDs:")
	assert.Empty(t, stderr.String())
}

func TestWriteCampaignMirrorRestoreQueueResult_RestoredSome(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	writeCampaignMirrorRestoreQueueResult(&stdout, &stderr, &evaluation.CampaignMirrorReconcileResult{
		RestoredRunIDs:   []string{"eval-init-gemma4-e2b-123"},
		SkippedRunIDs:    []string{"eval-init-gemma2-9b-1789729690"},
		PublishedRecords: 162,
	})
	out := stdout.String()
	assert.Contains(t, out, "Restored 1 verified dataset(s) to public mirror (162 record(s))")
	assert.Contains(t, out, "1 verified dataset(s) were already present")
	assert.Empty(t, stderr.String())
}

func TestWriteCampaignMirrorRestoreQueueResult_FailedRunsOnStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	writeCampaignMirrorRestoreQueueResult(&stdout, &stderr, &evaluation.CampaignMirrorReconcileResult{
		FailedRuns: map[string]string{"run-bad": "probe unavailable"},
	})
	require.Contains(t, stderr.String(), "error: mirror restore failed for run-bad")
	assert.NotContains(t, stdout.String(), "Restored 0")
}

func TestWriteCampaignMirrorRestoreRunResult_AlreadyPresent(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	writeCampaignMirrorRestoreRunResult(&stdout, "eval-init-gemma3-1b-1789733152", 0)
	out := stdout.String()
	assert.Contains(t, out, "already present in public mirror (nothing to do)")
	assert.False(t, strings.Contains(out, "Restored 0"))
}

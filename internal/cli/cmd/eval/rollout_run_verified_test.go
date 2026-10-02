// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// runSkipVerifiedRollout runs the queue with --skip-verified and returns the
// gates that were started.
func runSkipVerifiedRollout(t *testing.T, env *runEnv) []string {
	t.Helper()
	opts := rolloutCanaryOptions(t)
	opts.GateSmoke = false
	env.cmd.SetOut(&bytes.Buffer{})
	env.cmd.SetErr(&bytes.Buffer{})
	var started []string
	gates := func(fs.RuntimeFileService, []*evalv1.ModelVariant) rolloutGateRunner {
		return func(_ *cobra.Command, entry evaluation.CampaignQueueModel, gate string) (*rolloutRunSuccess, error) {
			started = append(started, entry.VariantID+"/"+gate)
			return &rolloutRunSuccess{VariantID: entry.VariantID, Tag: entry.ServedModelTag, RunID: "run-new", Gate: gate}, nil
		}
	}
	require.NoError(t, runRolloutWith(env.cmd, env.deps, opts, gates))
	return started
}

// markVerifiedOn queues qwen3:4b as verified by a prepared run of a campaign
// over the given suite.
func markVerifiedOn(t *testing.T, env *runEnv, suiteID string) {
	t.Helper()
	env.mustRun(t, "rollout", "add", "qwen3:4b")
	campaignID := "verified-on-" + suiteID
	_, err := createCampaign(context.Background(), env.deps, env.fileSvc(t), campaignCreateSpec{
		Platform:    testPlatform,
		CampaignID:  campaignID,
		Variants:    []*evalv1.ModelVariant{testQwenVariant()},
		Repetitions: 1,
		SuiteID:     suiteID,
	})
	require.NoError(t, err)
	runID := env.startPrepared(t, campaignID, "run-verified")
	_, err = evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
		FileService:   env.fileSvc(t),
		VariantID:     "qwen3-4b",
		Status:        evaluation.QueueStatusVerified,
		VerifiedRunID: runID,
	})
	require.NoError(t, err)
}

func TestRolloutRun_SkipVerifiedSkipsAnEntryVerifiedOnTheCurrentCatalog(t *testing.T) {
	env := setupRunEnv(t)
	markVerifiedOn(t, env, evaluation.DefaultSuiteID)

	assert.Empty(t, runSkipVerifiedRollout(t, env))
}

func TestRolloutRun_SkipVerifiedRequalifiesAnEntryVerifiedOnAnOlderCatalog(t *testing.T) {
	env := setupRunEnv(t)
	// The smoke suite stands in for a catalog that is no longer the built-in one.
	markVerifiedOn(t, env, evaluation.SmokeSuiteID)

	assert.Equal(t, []string{"qwen3-4b/" + rolloutGateFull}, runSkipVerifiedRollout(t, env))
}

func TestRolloutRun_SkipVerifiedRequalifiesAnEntryWhoseVerifiedRunIsGone(t *testing.T) {
	env := setupRunEnv(t)
	env.mustRun(t, "rollout", "add", "qwen3:4b")
	_, err := evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
		FileService:   env.fileSvc(t),
		VariantID:     "qwen3-4b",
		Status:        evaluation.QueueStatusVerified,
		VerifiedRunID: "run-deleted",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"qwen3-4b/" + rolloutGateFull}, runSkipVerifiedRollout(t, env))
}

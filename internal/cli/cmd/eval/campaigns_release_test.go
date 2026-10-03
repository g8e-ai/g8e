// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

// createLegacyCampaign persists a campaign spec as v2.2.7 froze it: no
// platform release in the spec or its digest.
func (e *runEnv) createLegacyCampaign(t *testing.T, campaignID string) {
	t.Helper()
	e.createCampaign(t, campaignID)
	spec, err := e.store(t).LoadCampaignSpec(context.Background(), campaignID)
	require.NoError(t, err)
	spec.PlatformRelease, spec.SourceRevision = "", ""
	spec.CampaignDigest, err = evaluation.ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	require.NoError(t, e.store(t).SaveCampaignSpec(context.Background(), spec))
}

func TestCampaignsCreate_RecordsTheBuildReleaseInTheFrozenSpec(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")

	spec, err := env.store(t).LoadCampaignSpec(context.Background(), "eval-a")
	require.NoError(t, err)
	assert.Equal(t, testReleaseVersion, spec.GetPlatformRelease())

	var payload campaignListJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "list"))
	require.Len(t, payload.Campaigns, 1)
	assert.Equal(t, testReleaseVersion, payload.Campaigns[0].Release)
	assert.Equal(t, string(evaluation.ReleaseBasisRecorded), payload.Campaigns[0].ReleaseBasis)
	assert.Contains(t, env.mustRun(t, "campaigns", "list"), testReleaseVersion+" (recorded)")
}

func TestCampaignsTag_AssertsTheReleaseOfALegacyCampaign(t *testing.T) {
	env := setupRunEnv(t)
	env.createLegacyCampaign(t, "eval-old")

	var before campaignListJSON
	require.NoError(t, env.runJSON(t, &before, "campaigns", "list"))
	require.Len(t, before.Campaigns, 1)
	assert.Equal(t, string(evaluation.ReleaseBasisUnknown), before.Campaigns[0].ReleaseBasis)

	out := env.mustRun(t, "campaigns", "tag", "--release", "v2.2.7", "eval-old")
	assert.Contains(t, out, "v2.2.7 (asserted)")

	var after campaignListJSON
	require.NoError(t, env.runJSON(t, &after, "campaigns", "list"))
	assert.Equal(t, "v2.2.7", after.Campaigns[0].Release)
	assert.Equal(t, string(evaluation.ReleaseBasisAsserted), after.Campaigns[0].ReleaseBasis)
}

func TestCampaignsTag_RefusesACampaignThatRecordedItsRelease(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")

	_, err := env.run(t, "campaigns", "tag", "--release", "v2.2.7", "eval-a")
	require.ErrorIs(t, err, constants.ErrEvaluationReleaseRecorded)
}

func TestCampaignsTag_RequiresARelease(t *testing.T) {
	env := setupRunEnv(t)
	env.createLegacyCampaign(t, "eval-old")

	_, err := env.run(t, "campaigns", "tag", "eval-old")
	require.ErrorIs(t, err, constants.ErrEvaluationFlagsInvalid)
}

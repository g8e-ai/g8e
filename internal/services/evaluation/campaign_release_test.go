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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// saveLegacyCampaign freezes a copy of the fixture campaign as a campaign
// frozen before releases were recorded: its spec has no platform_release.
func saveLegacyCampaign(t *testing.T, f *archiveFixture, campaignID string) {
	t.Helper()
	ctx := context.Background()
	legacy, err := f.store.LoadCampaignSpec(ctx, f.req.CampaignID)
	require.NoError(t, err)
	legacy.CampaignId = campaignID
	legacy.PlatformRelease, legacy.SourceRevision = "", ""
	legacy.CampaignDigest, err = ComputeCampaignSpecDigest(legacy)
	require.NoError(t, err)
	require.NoError(t, f.store.SaveCampaignSpec(ctx, legacy))
	require.NoError(t, f.store.SaveScenarioCatalog(ctx, campaignID, f.req.Catalog))
	require.NoError(t, f.store.SaveScenarioArtifacts(ctx, campaignID, f.req.Catalog, f.req.ScenarioArtifacts))
}

func TestStoreLoadCampaignRelease_ARecordedReleaseIsTheDigestBoundOne(t *testing.T) {
	f := newArchiveFixture(t)

	release, err := f.store.LoadCampaignRelease(context.Background(), f.req.CampaignID)
	require.NoError(t, err)
	assert.Equal(t, CampaignRelease{Release: testPlatform.Release, Basis: ReleaseBasisRecorded}, release)
}

func TestStoreTagCampaignRelease_AssertsAReleaseOnALegacyCampaign(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	saveLegacyCampaign(t, f, "legacy-campaign")

	before, err := f.store.LoadCampaignRelease(ctx, "legacy-campaign")
	require.NoError(t, err)
	assert.Equal(t, CampaignRelease{Basis: ReleaseBasisUnknown}, before)

	require.NoError(t, f.store.TagCampaignRelease(ctx, "legacy-campaign", "v2.2.7", archiveTestNow))
	after, err := f.store.LoadCampaignRelease(ctx, "legacy-campaign")
	require.NoError(t, err)
	assert.Equal(t, CampaignRelease{Release: "v2.2.7", Basis: ReleaseBasisAsserted}, after)

	require.NoError(t, f.store.TagCampaignRelease(ctx, "legacy-campaign", "v2.2.6", archiveTestNow), "an assertion can be corrected")
	corrected, err := f.store.LoadCampaignRelease(ctx, "legacy-campaign")
	require.NoError(t, err)
	assert.Equal(t, "v2.2.6", corrected.Release)
}

func TestStoreTagCampaignRelease_RefusesToRelabelARecordedRelease(t *testing.T) {
	f := newArchiveFixture(t)

	err := f.store.TagCampaignRelease(context.Background(), f.req.CampaignID, "v2.2.7", archiveTestNow)
	require.ErrorIs(t, err, constants.ErrEvaluationReleaseRecorded)
	release, err := f.store.LoadCampaignRelease(context.Background(), f.req.CampaignID)
	require.NoError(t, err)
	assert.Equal(t, CampaignRelease{Release: testPlatform.Release, Basis: ReleaseBasisRecorded}, release)
}

func TestStoreTagCampaignRelease_RequiresACampaignAndARelease(t *testing.T) {
	f := newArchiveFixture(t)

	require.ErrorIs(t, f.store.TagCampaignRelease(context.Background(), f.req.CampaignID, " ", archiveTestNow), constants.ErrMissingRequiredField)
	require.Error(t, f.store.TagCampaignRelease(context.Background(), "no-such-campaign", "v2.2.7", archiveTestNow))
}

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

	"github.com/stretchr/testify/require"

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

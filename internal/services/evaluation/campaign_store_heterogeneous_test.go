// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestStoreHeterogeneousStackSet_RoundTrip(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   testHeterogeneousVariants(),
	})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.SaveHeterogeneousStackSet(ctx, stackSet.CampaignID, stackSet))

	loaded, err := store.LoadHeterogeneousStackSet(ctx, stackSet.CampaignID)
	require.NoError(t, err)
	assert.Equal(t, stackSet.SetDigest, loaded.SetDigest)
	assert.Equal(t, len(stackSet.Stacks), len(loaded.Stacks))
}

func TestStoreHeterogeneousStackSet_RejectsMissingCampaignOnLoad(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	ctx := context.Background()
	_, err := store.LoadHeterogeneousStackSet(ctx, "missing-campaign")
	require.Error(t, err)
}

func TestIsCampaignFeedSequenceOutOfOrder(t *testing.T) {
	t.Parallel()
	assert.True(t, isCampaignFeedSequenceOutOfOrder(fmt.Errorf("export failed: %w", constants.ErrPublicFeedSequenceOutOfOrder)))
	assert.False(t, isCampaignFeedSequenceOutOfOrder(nil))
	assert.False(t, isCampaignFeedSequenceOutOfOrder(fmt.Errorf("other failure")))
}

func TestBuildCampaignPublicFeedRecord(t *testing.T) {
	t.Parallel()
	record := buildCampaignPublicFeedRecord(7, []byte(`{"sequence":7}`))
	assert.Equal(t, int64(7), record.Sequence)
	assert.NotEmpty(t, record.RecordHash)
	assert.Equal(t, `{"sequence":7}`, record.RecordBytes)
}

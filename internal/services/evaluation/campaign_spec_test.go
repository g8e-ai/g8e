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
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestMaterializeCampaignSpec_BindsCatalogAndInventory(t *testing.T) {
	t.Parallel()
	catalog, _, err := LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)

	spec, err := MaterializeCampaignSpec("north-star-smoke", catalog, inventory, 0)
	require.NoError(t, err)
	assert.Equal(t, CampaignSchemaVersion, spec.GetSchemaVersion())
	assert.Equal(t, "north-star-smoke", spec.GetCampaignId())
	assert.Equal(t, uint32(1), spec.GetRepetitionCount())
	assert.Equal(t, catalog.GetCatalogDigest(), spec.GetCatalogDigest())
	assert.Equal(t, inventory.RegistryDigest, spec.GetModelRegistryDigest())
	assert.NotEmpty(t, spec.GetCampaignDigest())
	assert.Equal(t, uint32(len(catalog.GetScenarios())), spec.GetScenarioCount())
}

func TestMaterializeCampaignSpec_RejectsMissingInputs(t *testing.T) {
	t.Parallel()
	catalog, _, err := LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)

	tests := []struct {
		name        string
		campaignID  string
		catalog     *evalv1.EvaluationScenarioCatalog
		inventory   *ModelInventoryFreeze
	}{
		{name: "empty campaign id", campaignID: "", catalog: catalog, inventory: inventory},
		{name: "nil catalog", campaignID: "north-star-smoke", catalog: nil, inventory: inventory},
		{name: "nil inventory", campaignID: "north-star-smoke", catalog: catalog, inventory: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := MaterializeCampaignSpec(test.campaignID, test.catalog, test.inventory, 1)
			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
		})
	}
}

func TestMemoryCampaignPublicationStateStore_LoadSaveCloneIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCampaignPublicationStateStore()

	loaded, err := store.Load(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, campaignPublicationStateSchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, "run-1", loaded.RunID)
	assert.Empty(t, loaded.PublishedIdempotency)

	state := &CampaignPublicationState{
		SchemaVersion:         campaignPublicationStateSchemaVersion,
		RunID:                 "run-1",
		PublishedIdempotency:  []string{"key-1"},
		LastPublishedSequence: 7,
	}
	require.NoError(t, store.Save(ctx, state))

	reloaded, err := store.Load(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, state.PublishedIdempotency, reloaded.PublishedIdempotency)
	reloaded.PublishedIdempotency[0] = "mutated"
	again, err := store.Load(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"key-1"}, again.PublishedIdempotency)
}

func TestMemoryCampaignPublicationStateStore_RejectsMissingFields(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCampaignPublicationStateStore()

	_, err := store.Load(ctx, "")
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)

	err = store.Save(ctx, nil)
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)

	err = store.Save(ctx, &CampaignPublicationState{SchemaVersion: campaignPublicationStateSchemaVersion})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

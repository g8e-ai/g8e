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

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignController_GenerateHeterogeneousStackSet_PersistsStacks(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })

	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, testHeterogeneousVariants())
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)

	stackSet, err := controller.GenerateHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)
	require.NotNil(t, stackSet)
	assert.NotEmpty(t, stackSet.Stacks)

	loaded, err := store.LoadHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	assert.Equal(t, stackSet.SetDigest, loaded.SetDigest)
}

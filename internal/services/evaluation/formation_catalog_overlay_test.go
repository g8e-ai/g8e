// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestMarshalAndLoadFormationCatalogOverlayRoundTrips(t *testing.T) {
	overlay := FormationCatalogOverlay{
		Formations:          []Formation{newTestFormation("test-new-formation")},
		RemovedFormationIDs: []string{"gemma-cascade"},
	}
	data, err := MarshalFormationCatalogOverlay(overlay)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "formation-catalog-overlay.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := LoadFormationCatalogOverlayFile(path)
	require.NoError(t, err)
	require.Len(t, loaded.Formations, 1)
	assert.Equal(t, "test-new-formation", loaded.Formations[0].ID)
	assert.Equal(t, "TestProvider", loaded.Formations[0].Primary.Provider)
	assert.Equal(t, []string{"gemma-cascade"}, loaded.RemovedFormationIDs)
}

func TestLoadFormationCatalogOverlayFileMissingWrapsNotExist(t *testing.T) {
	_, err := LoadFormationCatalogOverlayFile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestApplyFormationCatalogOverlayAddsAndRemoves(t *testing.T) {
	overlay := &FormationCatalogOverlay{
		Formations:          []Formation{newTestFormation("test-new-formation")},
		RemovedFormationIDs: []string{"gemma-cascade"},
	}
	topologies, err := ApplyFormationCatalogOverlay(overlay)
	require.NoError(t, err)
	require.Len(t, topologies.Formations(), 5)

	_, err = topologies.Formation("test-new-formation")
	assert.NoError(t, err)
	_, err = topologies.Formation("gemma-cascade")
	assert.ErrorIs(t, err, constants.ErrFormationInvalid)
}

func TestApplyFormationCatalogOverlayNilReturnsDefaults(t *testing.T) {
	topologies, err := ApplyFormationCatalogOverlay(nil)
	require.NoError(t, err)
	assert.Len(t, topologies.Formations(), 5)
}

func TestApplyFormationCatalogOverlayEmptyingTheCatalogStillErrors(t *testing.T) {
	defaults, err := NewExecutionTopologies()
	require.NoError(t, err)
	overlay := &FormationCatalogOverlay{}
	for _, formation := range defaults.Formations() {
		overlay.RemovedFormationIDs = append(overlay.RemovedFormationIDs, formation.ID)
	}
	_, err = ApplyFormationCatalogOverlay(overlay)
	assert.Error(t, err, "an overlay that removes every default formation must still trip the last-formation guard")
}

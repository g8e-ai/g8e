// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

package gateway

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestProofMirrorPendingArtifactIDs_ReturnsOnlyMirrorMissing(t *testing.T) {
	manifest := &models.PublicProofManifest{
		Artifacts: []models.PublicProofCatalogEntry{
			{ArtifactID: "aaa"},
			{ArtifactID: "bbb"},
			{ArtifactID: "ccc"},
		},
	}
	mirror := map[string]struct{}{
		"aaa": {},
		"ccc": {},
	}
	pending := proofMirrorPendingArtifactIDs(manifest, mirror)
	assert.Equal(t, []string{"bbb"}, pending)
}

func TestProofMirrorPendingArtifactIDs_EmptyMirrorRequiresAll(t *testing.T) {
	manifest := &models.PublicProofManifest{
		Artifacts: []models.PublicProofCatalogEntry{
			{ArtifactID: "one"},
			{ArtifactID: "two"},
		},
	}
	pending := proofMirrorPendingArtifactIDs(manifest, map[string]struct{}{})
	assert.Equal(t, []string{"one", "two"}, pending)
}

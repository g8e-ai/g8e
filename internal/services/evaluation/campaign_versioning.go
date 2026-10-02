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

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// campaignVersionDigestLen is how many leading catalog-digest characters a
// versioned campaign ID carries.
const campaignVersionDigestLen = 8

// ResolveCampaignIDForCatalog returns the campaign ID to freeze when a lineage
// of campaigns is bound to baseID and the scenario catalog now has the given
// digest.
//
// A frozen campaign is immutable, and the evidence of runs bound to it must stay
// verifiable, so a catalog change never rewrites it. baseID is returned while no
// campaign exists under it or the one that does froze the same catalog. A
// campaign frozen on a different catalog resolves to baseID plus the leading
// characters of the current catalog digest, which is stable for a given catalog:
// repeating the call finds the campaign created the first time instead of
// minting another.
func ResolveCampaignIDForCatalog(ctx context.Context, store CampaignStore, baseID, catalogDigest string) (string, error) {
	if store == nil || baseID == "" || len(catalogDigest) < campaignVersionDigestLen {
		return "", fmt.Errorf("evaluation: resolve campaign id: %w", constants.ErrMissingRequiredField)
	}
	existing, err := store.LoadCampaignSpec(ctx, baseID)
	switch {
	case err == nil:
		if existing.GetCatalogDigest() == catalogDigest {
			return baseID, nil
		}
		return baseID + "-" + catalogDigest[:campaignVersionDigestLen], nil
	case isNotFound(err):
		return baseID, nil
	default:
		return "", fmt.Errorf("evaluation: resolve campaign id %q: %w", baseID, err)
	}
}

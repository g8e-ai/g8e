// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// MaterializeNorthStarCampaignSpec binds the frozen catalog and model registry
// into one immutable campaign spec for Phase 4 controller initialization.
func MaterializeNorthStarCampaignSpec(campaignID string, catalog *evalv1.EvaluationScenarioCatalog, inventory *ModelInventoryFreeze, repetitionCount uint32) (*evalv1.EvaluationCampaignSpec, error) {
	if campaignID == "" || catalog == nil || inventory == nil || len(inventory.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: materialize north star campaign spec: %w", constants.ErrMissingRequiredField)
	}
	if err := ValidateNorthStarModelRegistry(inventory); err != nil {
		return nil, fmt.Errorf("evaluation: materialize north star campaign spec: %w", err)
	}
	if err := ValidateScenarioCatalogDigest(catalog); err != nil {
		return nil, fmt.Errorf("evaluation: materialize north star campaign spec: %w", err)
	}
	if repetitionCount == 0 {
		repetitionCount = 1
	}
	spec := &evalv1.EvaluationCampaignSpec{
		SchemaVersion:       CampaignSchemaVersion,
		CampaignId:          campaignID,
		CatalogRef:          catalog.GetCatalogRef(),
		CatalogDigest:       catalog.GetCatalogDigest(),
		ModelRegistry:       append([]*evalv1.ModelVariant(nil), inventory.Variants...),
		ModelRegistryDigest: inventory.RegistryDigest,
		GovernancePosture:   evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE,
		ScenarioCount:       uint32(len(catalog.GetScenarios())),
		RepetitionCount:     repetitionCount,
	}
	digest, err := ComputeCampaignSpecDigest(spec)
	if err != nil {
		return nil, fmt.Errorf("evaluation: materialize north star campaign spec: %w", err)
	}
	spec.CampaignDigest = digest
	return spec, nil
}

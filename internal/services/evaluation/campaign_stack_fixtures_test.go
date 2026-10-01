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
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// HeterogeneousStackGenerationRequest carries the inputs of the stack-set test
// fixture.
type HeterogeneousStackGenerationRequest struct {
	CampaignID string
	Seed       uint64
	Variants   []*evalv1.ModelVariant
}

// GenerateHeterogeneousStackSet builds a coverage-complete stack set over any
// three or more variants: stack i binds variant i, i+1, i+2 (mod n) to primary,
// assistant, and lite, so every variant serves every role. It stands in for
// production stack sets in tests that exercise scheduling, execution, and
// verification without needing the formation catalog's model tags. Production
// no longer generates these sets; the persisted rule still validates.
func GenerateHeterogeneousStackSet(req HeterogeneousStackGenerationRequest) (*HeterogeneousStackSet, error) {
	if req.CampaignID == "" || len(req.Variants) < 3 {
		return nil, fmt.Errorf("evaluation: generate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
	}
	variants := append([]*evalv1.ModelVariant(nil), req.Variants...)
	sort.Slice(variants, func(i, j int) bool { return variants[i].GetVariantId() < variants[j].GetVariantId() })
	stacks := make([]*evalv1.HeterogeneousStackDefinition, 0, len(variants))
	variantIDs := make([]string, 0, len(variants))
	for i, variant := range variants {
		variantIDs = append(variantIDs, variant.GetVariantId())
		stack, err := materializeStack(req.CampaignID, fmt.Sprintf("stack-%02d", i), variant, variants[(i+1)%len(variants)], variants[(i+2)%len(variants)])
		if err != nil {
			return nil, err
		}
		stacks = append(stacks, stack)
	}
	coverage := roleCoverageFromStacks(stacks)
	public := make(map[string][]string, len(coverage))
	for variantID, roles := range coverage {
		for role := range roles {
			public[variantID] = append(public[variantID], role)
		}
		sort.Strings(public[variantID])
	}
	set := &HeterogeneousStackSet{
		CampaignID:     req.CampaignID,
		GenerationRule: HeterogeneousStackGenerationRule,
		Seed:           req.Seed,
		VariantIDs:     variantIDs,
		Stacks:         stacks,
		Coverage:       &HeterogeneousCoverageMatrix{TotalStackCount: len(stacks), VariantRoleCoverage: public},
	}
	digest, err := ComputeHeterogeneousStackSetDigest(set)
	if err != nil {
		return nil, err
	}
	set.SetDigest = digest
	if err := ValidateHeterogeneousStackSet(set); err != nil {
		return nil, err
	}
	return set, nil
}

// GenerateHeterogeneousStackSet persists the fixture stack set for one frozen
// campaign registry.
func (c *CampaignController) GenerateHeterogeneousStackSet(ctx context.Context, campaignID string, seed uint64) (*HeterogeneousStackSet, error) {
	spec, err := c.store.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{CampaignID: campaignID, Seed: seed, Variants: spec.GetModelRegistry()})
	if err != nil {
		return nil, err
	}
	if err := c.store.SaveHeterogeneousStackSet(ctx, campaignID, stackSet); err != nil {
		return nil, err
	}
	return stackSet, nil
}

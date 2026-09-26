// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	// FormationCatalogStackGenerationRule identifies stack sets materialized from
	// the checked-in ExecutionTopologies catalog instead of registry hypotheses.
	FormationCatalogStackGenerationRule = FormationSchemaVersion
)

// FormationCatalogStackGenerationRequest carries frozen inputs for one
// deterministic formation-catalog stack set.
type FormationCatalogStackGenerationRequest struct {
	CampaignID string
	Seed       uint64
	Variants   []*evalv1.ModelVariant
}

// GenerateFormationCatalogStackSet materializes one heterogeneous stack per
// checked-in formation and validates that the frozen registry covers every
// catalog served tag before persisting the set.
func GenerateFormationCatalogStackSet(req FormationCatalogStackGenerationRequest) (*HeterogeneousStackSet, error) {
	if req.CampaignID == "" {
		return nil, fmt.Errorf("evaluation: generate formation catalog stack set: %w", constants.ErrMissingRequiredField)
	}
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil, fmt.Errorf("evaluation: generate formation catalog stack set: %w", err)
	}
	formations := topologies.Formations()
	if len(formations) == 0 {
		return nil, fmt.Errorf("evaluation: generate formation catalog stack set: catalog is empty")
	}
	registry := indexFormationVariants(req.Variants)
	stacks := make([]*evalv1.HeterogeneousStackDefinition, 0, len(formations))
	registryVariantIDs := make(map[string]struct{})
	for _, formation := range formations {
		if err := ensureFormationRegistryCoverage(formation, registry); err != nil {
			return nil, err
		}
		stack, err := formation.ToStackDefinition()
		if err != nil {
			return nil, fmt.Errorf("evaluation: generate formation catalog stack set: formation %q: %w", formation.ID, err)
		}
		stacks = append(stacks, stack)
		for _, model := range formation.Models() {
			variant := lookupFormationVariant(registry, model.VariantID, model.ServedModelTag)
			if variant == nil {
				return nil, fmt.Errorf("evaluation: generate formation catalog stack set: formation %q role model %q: %w", formation.ID, model.ServedModelTag, constants.ErrFormationRegistryBinding)
			}
			registryVariantIDs[variant.GetVariantId()] = struct{}{}
		}
	}
	sort.Slice(stacks, func(i, j int) bool {
		return stacks[i].GetStackId() < stacks[j].GetStackId()
	})
	variantIDs := make([]string, 0, len(registryVariantIDs))
	for variantID := range registryVariantIDs {
		variantIDs = append(variantIDs, variantID)
	}
	sort.Strings(variantIDs)
	set := &HeterogeneousStackSet{
		CampaignID:     req.CampaignID,
		GenerationRule: FormationCatalogStackGenerationRule,
		Seed:           req.Seed,
		VariantIDs:     variantIDs,
		Stacks:         stacks,
		Coverage:       buildFormationCatalogCoverageMatrix(stacks),
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

func ensureFormationRegistryCoverage(formation Formation, registry formationVariantRegistry) error {
	for _, role := range formation.Roles() {
		model, err := formation.Model(role)
		if err != nil {
			return err
		}
		if lookupFormationVariant(registry, model.VariantID, model.ServedModelTag) == nil {
			return fmt.Errorf("evaluation: generate formation catalog stack set: formation %q role %s served tag %q: %w", formation.ID, role, model.ServedModelTag, constants.ErrFormationRegistryBinding)
		}
		if model.Trust == FormationTrustSovereign {
			variant := lookupFormationVariant(registry, model.VariantID, model.ServedModelTag)
			if variant.GetModelDigest() == "" {
				return fmt.Errorf("evaluation: generate formation catalog stack set: formation %q role %s: %w", formation.ID, role, constants.ErrFormationAttestationRequired)
			}
		}
	}
	return nil
}

func buildFormationCatalogCoverageMatrix(stacks []*evalv1.HeterogeneousStackDefinition) *HeterogeneousCoverageMatrix {
	coverage := roleCoverageFromStacks(stacks)
	public := make(map[string][]string, len(coverage))
	for variantID, roles := range coverage {
		roleList := make([]string, 0, len(roles))
		for role := range roles {
			roleList = append(roleList, role)
		}
		sort.Strings(roleList)
		public[variantID] = roleList
	}
	return &HeterogeneousCoverageMatrix{
		HypothesisStackCount: len(stacks),
		CoverageStackCount:   0,
		TotalStackCount:      len(stacks),
		VariantRoleCoverage:  public,
	}
}

func validateFormationCatalogStackSet(set *HeterogeneousStackSet) error {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return err
	}
	expectedFormations := make(map[string]struct{})
	for _, formation := range topologies.Formations() {
		expectedFormations[formation.ID] = struct{}{}
	}
	if len(set.Stacks) != len(expectedFormations) {
		return fmt.Errorf("evaluation: validate formation catalog stack set: expected %d stacks, got %d", len(expectedFormations), len(set.Stacks))
	}
	seenStackIDs := make(map[string]struct{}, len(set.Stacks))
	for _, stack := range set.Stacks {
		if stack == nil || stack.GetStackId() == "" {
			return fmt.Errorf("evaluation: validate formation catalog stack set: %w", constants.ErrMissingRequiredField)
		}
		if _, exists := seenStackIDs[stack.GetStackId()]; exists {
			return fmt.Errorf("evaluation: validate formation catalog stack set: duplicate stack_id %q", stack.GetStackId())
		}
		seenStackIDs[stack.GetStackId()] = struct{}{}
		if _, ok := expectedFormations[stack.GetStackId()]; !ok {
			return fmt.Errorf("evaluation: validate formation catalog stack set: unknown stack_id %q", stack.GetStackId())
		}
		if err := ValidateHeterogeneousStackDigest(stack); err != nil {
			return err
		}
	}
	return nil
}

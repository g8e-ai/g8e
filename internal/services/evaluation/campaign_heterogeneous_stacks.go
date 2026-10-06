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

const (
	// HeterogeneousStackGenerationRule names stack sets that earlier releases
	// generated from the registry rather than from the formation catalog. New
	// campaigns never produce one; the rule stays so a persisted set still
	// validates, resumes, and verifies.
	HeterogeneousStackGenerationRule = "heterogeneous-v1"
)

// HeterogeneousCoverageMatrix records which variant-role placements are
// satisfied by a stack set. Its field names are the persisted JSON keys.
type HeterogeneousCoverageMatrix struct {
	HypothesisStackCount int                 `json:"hypothesis_stack_count,omitempty"`
	CoverageStackCount   int                 `json:"coverage_stack_count,omitempty"`
	TotalStackCount      int                 `json:"total_stack_count,omitempty"`
	VariantRoleCoverage  map[string][]string `json:"variant_role_coverage,omitempty"`
}

// HeterogeneousStackSet is the frozen stack set of one system-lane campaign:
// one stack per formation, bound to the campaign's frozen model registry.
type HeterogeneousStackSet struct {
	CampaignID     string                                 `json:"campaign_id,omitempty"`
	GenerationRule string                                 `json:"generation_rule,omitempty"`
	Seed           uint64                                 `json:"seed,omitempty"`
	SetDigest      string                                 `json:"set_digest,omitempty"`
	VariantIDs     []string                               `json:"variant_ids,omitempty"`
	Stacks         []*evalv1.HeterogeneousStackDefinition `json:"stacks,omitempty"`
	Coverage       *HeterogeneousCoverageMatrix           `json:"coverage,omitempty"`
}

// ValidateHeterogeneousStackSet verifies stack digests, coverage, and set digest.
func ValidateHeterogeneousStackSet(set *HeterogeneousStackSet) error {
	if set == nil || set.CampaignID == "" || set.GenerationRule == "" || len(set.Stacks) == 0 || set.Coverage == nil {
		return fmt.Errorf("evaluation: validate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
	}
	expectedDigest, err := ComputeHeterogeneousStackSetDigest(set)
	if err != nil {
		return err
	}
	if set.SetDigest != expectedDigest {
		return fmt.Errorf("evaluation: validate heterogeneous stack set: set digest: %w", constants.ErrChecksumMismatch)
	}
	seenStackIDs := make(map[string]struct{}, len(set.Stacks))
	for _, stack := range set.Stacks {
		if stack == nil || stack.GetStackId() == "" {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
		}
		if _, exists := seenStackIDs[stack.GetStackId()]; exists {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: duplicate stack_id %q", stack.GetStackId())
		}
		seenStackIDs[stack.GetStackId()] = struct{}{}
		if err := ValidateHeterogeneousStackDigest(stack); err != nil {
			return err
		}
	}
	if set.GenerationRule == FormationCatalogStackGenerationRule {
		return validateFormationCatalogStackSet(set)
	}
	return validateVariantRoleCoverageComplete(set.VariantIDs, set.Stacks)
}

func validateVariantRoleCoverageComplete(variantIDs []string, stacks []*evalv1.HeterogeneousStackDefinition) error {
	coverage := roleCoverageFromStacks(stacks)
	for _, stack := range stacks {
		for _, slot := range []struct {
			role      evalv1.ModelCampaignRole
			variantID string
		}{
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, stack.GetPrimarySlot().GetVariantId()},
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT, stack.GetAssistantSlot().GetVariantId()},
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE, stack.GetLiteSlot().GetVariantId()},
		} {
			if slot.variantID == "" {
				return fmt.Errorf("evaluation: validate heterogeneous stack set: stack %s missing role binding", stack.GetStackId())
			}
		}
	}
	for _, variantID := range variantIDs {
		roles := coverage[variantID]
		if len(roles) != 3 {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: variant %s missing role coverage (%d/3)", variantID, len(roles))
		}
	}
	return nil
}

func roleCoverageFromStacks(stacks []*evalv1.HeterogeneousStackDefinition) map[string]map[string]struct{} {
	coverage := make(map[string]map[string]struct{})
	for _, stack := range stacks {
		recordRoleCoverage(coverage, stack.GetPrimarySlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY))
		recordRoleCoverage(coverage, stack.GetAssistantSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT))
		recordRoleCoverage(coverage, stack.GetLiteSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE))
	}
	return coverage
}

func recordRoleCoverage(coverage map[string]map[string]struct{}, variantID, role string) {
	if variantID == "" || role == "" {
		return
	}
	if coverage[variantID] == nil {
		coverage[variantID] = make(map[string]struct{})
	}
	coverage[variantID][role] = struct{}{}
}

func materializeStack(campaignID, stackID string, primary, assistant, lite *evalv1.ModelVariant) (*evalv1.HeterogeneousStackDefinition, error) {
	if primary == nil || assistant == nil || lite == nil {
		return nil, fmt.Errorf("evaluation: materialize heterogeneous stack: %w", constants.ErrMissingRequiredField)
	}
	if primary.GetVariantId() == assistant.GetVariantId() || primary.GetVariantId() == lite.GetVariantId() || assistant.GetVariantId() == lite.GetVariantId() {
		return nil, fmt.Errorf("evaluation: materialize heterogeneous stack %s: roles must bind distinct variants", stackID)
	}
	stack := &evalv1.HeterogeneousStackDefinition{
		StackId: stackID,
		PrimarySlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			VariantId:      primary.GetVariantId(),
		},
		AssistantSlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT,
			VariantId:      assistant.GetVariantId(),
		},
		LiteSlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE,
			VariantId:      lite.GetVariantId(),
		},
	}
	digest, err := ComputeHeterogeneousStackDigest(stack)
	if err != nil {
		return nil, err
	}
	stack.StackDigest = digest
	return stack, nil
}

func roleShortName(role evalv1.ModelCampaignRole) string {
	switch role {
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY:
		return "primary"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
		return "assistant"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
		return "lite"
	default:
		return role.String()
	}
}

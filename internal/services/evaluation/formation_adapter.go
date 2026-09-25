// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// FormationBindingRequest binds one campaign stack to frozen registry digests
// before FormationRunner.Run. Catalog metadata supplies display estimates;
// sovereign digests and variant IDs must come from the frozen inventory.
type FormationBindingRequest struct {
	FormationID string
	Stack       *evalv1.HeterogeneousStackDefinition
	Variants    []*evalv1.ModelVariant
}

// FormationRunContext carries opaque campaign identifiers into governed role
// execution. Checkpoint persistence remains an adapter responsibility.
type FormationRunContext struct {
	CampaignID          string
	RunID               string
	AssignmentID        string
	EvaluationAttemptID string
	ModelRegistryDigest string
	InitialState        []byte
	InferenceSessionID  string
	DataSessionID       string
}

// FormationBindingFromCatalog materializes the canonical stack for one catalog
// formation and prepares a binding request against frozen registry variants.
func FormationBindingFromCatalog(formationID string, variants []*evalv1.ModelVariant) (FormationBindingRequest, error) {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return FormationBindingRequest{}, err
	}
	formation, err := topologies.Formation(formationID)
	if err != nil {
		return FormationBindingRequest{}, err
	}
	stack, err := formation.ToStackDefinition()
	if err != nil {
		return FormationBindingRequest{}, err
	}
	return FormationBindingRequest{
		FormationID: formationID,
		Stack:       stack,
		Variants:    variants,
	}, nil
}

// BindFormation resolves catalog metadata and frozen-registry digests for one
// governed formation run. It fails closed when the stack digest is invalid, a
// sovereign role is missing from the registry, served tags diverge, or any
// required digest is empty.
func BindFormation(req FormationBindingRequest) (Formation, error) {
	if req.Stack == nil {
		return Formation{}, fmt.Errorf("formation: bind: %w", constants.ErrMissingRequiredField)
	}
	if err := ValidateHeterogeneousStackDigest(req.Stack); err != nil {
		return Formation{}, err
	}
	formationID := req.FormationID
	if formationID == "" {
		formationID = req.Stack.GetStackId()
	}
	if formationID == "" {
		return Formation{}, fmt.Errorf("formation %q: %w", formationID, constants.ErrFormationInvalid)
	}
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return Formation{}, err
	}
	formation, err := topologies.Formation(formationID)
	if err != nil {
		return Formation{}, err
	}
	registry := indexFormationVariants(req.Variants)
	primary, err := bindFormationRole(formation, FormationRolePrimary, req.Stack.GetPrimarySlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	assistant, err := bindFormationRole(formation, FormationRoleAssistant, req.Stack.GetAssistantSlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	lite, err := bindFormationRole(formation, FormationRoleLite, req.Stack.GetLiteSlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	formation.Primary = primary
	formation.Assistant = assistant
	formation.Lite = lite
	if err := formation.Validate(); err != nil {
		return Formation{}, err
	}
	return formation, nil
}

func indexFormationVariants(variants []*evalv1.ModelVariant) map[string]*evalv1.ModelVariant {
	index := make(map[string]*evalv1.ModelVariant, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetVariantId() == "" {
			continue
		}
		index[variant.GetVariantId()] = variant
	}
	return index
}

func bindFormationRole(formation Formation, role FormationRole, slot *evalv1.RoleAssignment, registry map[string]*evalv1.ModelVariant) (FormationModel, error) {
	catalogModel, err := formation.Model(role)
	if err != nil {
		return FormationModel{}, err
	}
	if slot == nil || slot.GetVariantId() == "" {
		return FormationModel{}, fmt.Errorf("formation %q role %s: %w", formation.ID, role, constants.ErrFormationRegistryBinding)
	}
	if slot.GetVariantId() != catalogModel.VariantID {
		return FormationModel{}, fmt.Errorf("formation %q role %s: variant %q does not match catalog %q: %w", formation.ID, role, slot.GetVariantId(), catalogModel.VariantID, constants.ErrFormationStackMismatch)
	}
	variant := registry[slot.GetVariantId()]
	if variant == nil {
		return FormationModel{}, fmt.Errorf("formation %q role %s: variant %q: %w", formation.ID, role, slot.GetVariantId(), constants.ErrFormationRegistryBinding)
	}
	if variant.GetServedModelTag() != catalogModel.ServedModelTag {
		return FormationModel{}, fmt.Errorf("formation %q role %s: served tag %q does not match catalog %q: %w", formation.ID, role, variant.GetServedModelTag(), catalogModel.ServedModelTag, constants.ErrFormationStackMismatch)
	}
	if catalogModel.Trust == FormationTrustSovereign {
		if variant.GetProviderClass() != catalogModel.ProviderClass {
			return FormationModel{}, fmt.Errorf("formation %q role %s: provider class %q does not match catalog %q: %w", formation.ID, role, variant.GetProviderClass(), catalogModel.ProviderClass, constants.ErrFormationStackMismatch)
		}
		if variant.GetModelDigest() == "" {
			return FormationModel{}, fmt.Errorf("formation %q role %s: %w", formation.ID, role, constants.ErrFormationAttestationRequired)
		}
	}
	bound := catalogModel
	bound.VariantID = variant.GetVariantId()
	bound.ModelDigest = variant.GetModelDigest()
	return bound, nil
}

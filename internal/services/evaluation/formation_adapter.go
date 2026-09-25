// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"fmt"
	"strings"

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
	ScenarioID          string
	ModelRegistryDigest string
	InitialState        []byte
	InferenceSessionID  string
	DataSessionID       string
	OnRoleProgress      func(context.Context, *FormationRunResult) error
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

// IsCatalogFormationID reports whether one stable ID matches the checked-in
// ExecutionTopologies catalog.
func IsCatalogFormationID(id string) bool {
	if id == "" {
		return false
	}
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return false
	}
	_, err = topologies.Formation(id)
	return err == nil
}

// ResolveFormationBinding binds one heterogeneous stack using catalog metadata
// when the stack ID matches ExecutionTopologies, otherwise falling back to the
// scheduler-derived heterogeneous binding path.
func ResolveFormationBinding(req FormationBindingRequest) (Formation, error) {
	formationID := req.FormationID
	if formationID == "" && req.Stack != nil {
		formationID = req.Stack.GetStackId()
	}
	if IsCatalogFormationID(formationID) {
		req.FormationID = formationID
		return BindFormation(req)
	}
	return BindHeterogeneousStack(req)
}

// BindHeterogeneousStack resolves one campaign heterogeneous stack against the
// frozen registry without requiring a catalog formation entry.
func BindHeterogeneousStack(req FormationBindingRequest) (Formation, error) {
	if req.Stack == nil {
		return Formation{}, fmt.Errorf("formation: bind heterogeneous stack: %w", constants.ErrMissingRequiredField)
	}
	if err := ValidateHeterogeneousStackDigest(req.Stack); err != nil {
		return Formation{}, err
	}
	stackID := req.Stack.GetStackId()
	if stackID == "" {
		return Formation{}, fmt.Errorf("formation: bind heterogeneous stack: %w", constants.ErrFormationInvalid)
	}
	registry := indexFormationVariants(req.Variants)
	primary, err := bindHeterogeneousRole(FormationRolePrimary, req.Stack.GetPrimarySlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	assistant, err := bindHeterogeneousRole(FormationRoleAssistant, req.Stack.GetAssistantSlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	lite, err := bindHeterogeneousRole(FormationRoleLite, req.Stack.GetLiteSlot(), registry)
	if err != nil {
		return Formation{}, err
	}
	formation := Formation{
		ID:                stackID,
		DisplayName:       stackID,
		Description:       "heterogeneous campaign stack",
		MaxVRAMMiB:        FormationMaxVRAMMiB,
		Primary:           primary,
		Assistant:         assistant,
		Lite:              lite,
		RelaxedValidation: true,
	}
	if err := formation.Validate(); err != nil {
		return Formation{}, err
	}
	return formation, nil
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

type formationVariantRegistry struct {
	byVariantID      map[string]*evalv1.ModelVariant
	byServedModelTag map[string]*evalv1.ModelVariant
}

func indexFormationVariants(variants []*evalv1.ModelVariant) formationVariantRegistry {
	registry := formationVariantRegistry{
		byVariantID:      make(map[string]*evalv1.ModelVariant, len(variants)),
		byServedModelTag: make(map[string]*evalv1.ModelVariant, len(variants)),
	}
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		if variant.GetVariantId() != "" {
			registry.byVariantID[variant.GetVariantId()] = variant
		}
		if tag := variant.GetServedModelTag(); tag != "" {
			registry.byServedModelTag[tag] = variant
		}
	}
	return registry
}

func lookupFormationVariant(registry formationVariantRegistry, variantID, servedModelTag string) *evalv1.ModelVariant {
	if tag := strings.TrimSpace(servedModelTag); tag != "" {
		if variant := registry.byServedModelTag[tag]; variant != nil {
			return variant
		}
	}
	return registry.byVariantID[variantID]
}

func bindFormationRole(formation Formation, role FormationRole, slot *evalv1.RoleAssignment, registry formationVariantRegistry) (FormationModel, error) {
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
	variant := lookupFormationVariant(registry, slot.GetVariantId(), catalogModel.ServedModelTag)
	if variant == nil {
		return FormationModel{}, fmt.Errorf("formation %q role %s: variant %q served tag %q: %w", formation.ID, role, slot.GetVariantId(), catalogModel.ServedModelTag, constants.ErrFormationRegistryBinding)
	}
	if variant.GetServedModelTag() != catalogModel.ServedModelTag {
		return FormationModel{}, fmt.Errorf("formation %q role %s: served tag %q does not match catalog %q: %w", formation.ID, role, variant.GetServedModelTag(), catalogModel.ServedModelTag, constants.ErrFormationStackMismatch)
	}
	if variant.GetProviderClass() != catalogModel.ProviderClass {
		return FormationModel{}, fmt.Errorf("formation %q role %s: provider class %q does not match catalog %q: %w", formation.ID, role, variant.GetProviderClass(), catalogModel.ProviderClass, constants.ErrFormationStackMismatch)
	}
	if catalogModel.Trust == FormationTrustSovereign {
		if variant.GetModelDigest() == "" {
			return FormationModel{}, fmt.Errorf("formation %q role %s: %w", formation.ID, role, constants.ErrFormationAttestationRequired)
		}
	}
	bound := catalogModel
	bound.VariantID = variant.GetVariantId()
	bound.ModelDigest = variant.GetModelDigest()
	return bound, nil
}

func bindHeterogeneousRole(role FormationRole, slot *evalv1.RoleAssignment, registry formationVariantRegistry) (FormationModel, error) {
	if slot == nil || slot.GetVariantId() == "" {
		return FormationModel{}, fmt.Errorf("formation role %s: %w", role, constants.ErrFormationRegistryBinding)
	}
	variant := registry.byVariantID[slot.GetVariantId()]
	if variant == nil {
		return FormationModel{}, fmt.Errorf("formation role %s: variant %q: %w", role, slot.GetVariantId(), constants.ErrFormationRegistryBinding)
	}
	model := formationModelFromVariant(variant)
	if model.Trust == FormationTrustSovereign && model.ModelDigest == "" {
		return FormationModel{}, fmt.Errorf("formation role %s: %w", role, constants.ErrFormationAttestationRequired)
	}
	return model, nil
}

func formationModelFromVariant(variant *evalv1.ModelVariant) FormationModel {
	if variant == nil {
		return FormationModel{}
	}
	trust := FormationTrustSovereign
	providerClass := variant.GetProviderClass()
	if providerClass == "gemini" {
		trust = FormationTrustDelegated
	}
	provider := variant.GetModelFamily()
	if provider == "" {
		provider = variant.GetVariantId()
	}
	family := variant.GetModelFamily()
	if family == "" {
		family = variant.GetVariantId()
	}
	quantization := variant.GetQuantization()
	if quantization == "" && trust == FormationTrustSovereign {
		quantization = "Q4_K_M"
	}
	return FormationModel{
		VariantID:      variant.GetVariantId(),
		DisplayName:    variant.GetServedModelTag(),
		Provider:       provider,
		Family:         family,
		ProviderClass:  providerClass,
		ServedModelTag: variant.GetServedModelTag(),
		Trust:          trust,
		Quantization:   quantization,
		ParameterCount: variant.GetParameterCount(),
		ModelDigest:    variant.GetModelDigest(),
	}
}

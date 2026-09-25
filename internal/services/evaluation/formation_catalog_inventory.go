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
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// FormationDelegatedRegistryDigestPlaceholder is the frozen-registry digest used
// for delegated catalog models that are not discovered from a sovereign provider
// inventory. Weight attestation is skipped at execution time.
const FormationDelegatedRegistryDigestPlaceholder = "0000000000000000000000000000000000000000000000000000000000000000"

// FormationCatalogServedTags returns the unique served model tags required by
// the checked-in ExecutionTopologies catalog, sorted lexicographically.
func FormationCatalogServedTags() []string {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, formation := range topologies.Formations() {
		for _, model := range formation.Models() {
			seen[model.ServedModelTag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// MaterializeFormationCatalogVariants selects the registry variants required by
// the checked-in catalog from a source inventory. Sovereign served tags must be
// present in the source; delegated catalog models missing from the source receive
// deterministic placeholder digests for registry binding.
func MaterializeFormationCatalogVariants(sourceVariants []*evalv1.ModelVariant) ([]*evalv1.ModelVariant, error) {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil, err
	}
	registry := indexFormationVariants(sourceVariants)
	selected := make(map[string]*evalv1.ModelVariant)
	for _, formation := range topologies.Formations() {
		for _, model := range formation.Models() {
			if _, exists := selected[model.ServedModelTag]; exists {
				continue
			}
			variant := lookupFormationVariant(registry, model.VariantID, model.ServedModelTag)
			if variant == nil {
				if model.Trust != FormationTrustDelegated {
					return nil, fmt.Errorf("evaluation: formation catalog inventory: served tag %q: %w", model.ServedModelTag, constants.ErrInferenceModelNotFound)
				}
				variant = formationDelegatedCatalogVariant(model)
			}
			if model.Trust == FormationTrustSovereign && variant.GetModelDigest() == "" {
				return nil, fmt.Errorf("evaluation: formation catalog inventory: served tag %q: %w", model.ServedModelTag, constants.ErrFormationAttestationRequired)
			}
			selected[model.ServedModelTag] = proto.Clone(variant).(*evalv1.ModelVariant)
		}
	}
	tags := FormationCatalogServedTags()
	picked := make([]*evalv1.ModelVariant, 0, len(tags))
	for _, tag := range tags {
		variant := selected[tag]
		if variant == nil {
			return nil, fmt.Errorf("evaluation: formation catalog inventory: served tag %q: %w", tag, constants.ErrInferenceModelNotFound)
		}
		picked = append(picked, variant)
	}
	return picked, nil
}

// FormationCatalogIntakeModels returns rollout-intake staging entries for every
// sovereign served tag in the checked-in ExecutionTopologies catalog.
func FormationCatalogIntakeModels() ([]RolloutIntakeModel, error) {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	models := make([]RolloutIntakeModel, 0, len(FormationCatalogServedTags()))
	for _, formation := range topologies.Formations() {
		for _, model := range formation.Models() {
			if model.Trust != FormationTrustSovereign {
				continue
			}
			tag := strings.TrimSpace(model.ServedModelTag)
			if tag == "" {
				continue
			}
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}
			models = append(models, RolloutIntakeModel{
				VariantID:      model.VariantID,
				ServedModelTag: tag,
				ModelFamily:    model.Family,
				Quantization:   model.Quantization,
				Staging: RolloutIntakeStaging{
					Method:     "ollama_library_pull",
					Status:     "ready",
					OllamaPull: tag,
				},
			})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ServedModelTag < models[j].ServedModelTag
	})
	if len(models) == 0 {
		return nil, fmt.Errorf("evaluation: formation catalog intake models: %w", constants.ErrMissingRequiredField)
	}
	return models, nil
}

// FormationCatalogFixtureVariants returns typed registry variants covering every
// served tag in the checked-in catalog. digestForTag supplies deterministic
// digests for hermetic tests.
func FormationCatalogFixtureVariants(digestForTag func(string) string) []*evalv1.ModelVariant {
	if digestForTag == nil {
		digestForTag = func(string) string { return FormationDelegatedRegistryDigestPlaceholder }
	}
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil
	}
	seen := make(map[string]*evalv1.ModelVariant)
	for _, formation := range topologies.Formations() {
		for _, model := range formation.Models() {
			if _, exists := seen[model.ServedModelTag]; exists {
				continue
			}
			variant := formationCatalogFixtureVariant(model, digestForTag(model.ServedModelTag))
			seen[model.ServedModelTag] = variant
		}
	}
	tags := FormationCatalogServedTags()
	variants := make([]*evalv1.ModelVariant, 0, len(tags))
	for _, tag := range tags {
		variants = append(variants, seen[tag])
	}
	return variants
}

func formationDelegatedCatalogVariant(model FormationModel) *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      "freeze-" + model.VariantID,
		ProviderClass:  model.ProviderClass,
		ServedModelTag: model.ServedModelTag,
		ModelDigest:    FormationDelegatedRegistryDigestPlaceholder,
		ModelFamily:    model.Family,
	}
}

func formationCatalogFixtureVariant(model FormationModel, digest string) *evalv1.ModelVariant {
	variant := &evalv1.ModelVariant{
		VariantId:      "freeze-" + model.VariantID,
		ProviderClass:  model.ProviderClass,
		ServedModelTag: model.ServedModelTag,
		ModelDigest:    digest,
		ModelFamily:    model.Family,
		ParameterCount: model.ParameterCount,
		Quantization:   model.Quantization,
	}
	if model.Trust == FormationTrustDelegated {
		variant.Quantization = ""
		variant.ParameterCount = 0
	}
	return variant
}


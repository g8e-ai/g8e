// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	HomogeneousRoleCount  = 3
	StandardScenarioCount = 25
)

// ModelInventoryFreeze is the immutable model registry derived from
// a complete provider inventory query and optional capability probes.
type ModelInventoryFreeze struct {
	CampaignID           string
	RegistryDigest       string
	Variants             []*evalv1.ModelVariant
	InferenceVariants    []*operatorv1.InferenceModelVariant
	HomogeneousCellCount uint64
}

// ModelInventoryOptions controls optional inventory freeze behavior.
type ModelInventoryOptions struct {
	RunCapabilityProbes    bool
	CapabilityProbeRunner  GovernedCapabilityProbeRunner
}

// GovernedCapabilityProbeRunner executes bounded non-scored capability probes
// through the exact Inference Operator session.
type GovernedCapabilityProbeRunner interface {
	RunCapabilityProbes(context.Context, *evalv1.ModelVariant) ([]*evalv1.ModelCapabilityObservation, error)
}

// FreezeModelInventoryFromProvider discovers the complete provider inventory
// through the governed Inference Operator session, optionally runs bounded
// capability probes, and freezes the eval model registry digest for campaignID.
func FreezeModelInventoryFromProvider(
	ctx context.Context,
	dispatcher OllamaModelCommandDispatcher,
	maintenance OllamaModelMaintenanceContext,
	campaignID string,
	opts ModelInventoryOptions,
) (*ModelInventoryFreeze, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("evaluation: freeze model inventory: %w", constants.ErrMissingRequiredField)
	}
	entries, err := ListOllamaProviderInventory(ctx, dispatcher, maintenance)
	if err != nil {
		return nil, fmt.Errorf("evaluation: freeze model inventory: %w", err)
	}
	variants, err := BuildModelVariantsFromProviderInventory(ctx, entries, opts)
	if err != nil {
		return nil, err
	}
	registry, err := MaterializeModelRegistry(campaignID, variants)
	if err != nil {
		return nil, err
	}
	if err := ValidateModelRegistry(registry); err != nil {
		return nil, err
	}
	return registry, nil
}

// BuildModelVariantsFromProviderInventory converts discovered provider entries
// into typed eval ModelVariant records without deduplicating served tags.
func BuildModelVariantsFromProviderInventory(ctx context.Context, entries []inference.ProviderModelInventoryEntry, opts ModelInventoryOptions) ([]*evalv1.ModelVariant, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("evaluation: build model variants: %w", constants.ErrMissingRequiredField)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ServedModelTag < entries[j].ServedModelTag
	})
	variants := make([]*evalv1.ModelVariant, 0, len(entries))
	seenTags := make(map[string]struct{}, len(entries))
	seenIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ServedModelTag == "" || entry.ModelDigest == "" {
			return nil, fmt.Errorf("evaluation: build model variants: %w", constants.ErrMissingRequiredField)
		}
		if _, exists := seenTags[entry.ServedModelTag]; exists {
			return nil, fmt.Errorf("evaluation: build model variants: duplicate served tag %q", entry.ServedModelTag)
		}
		seenTags[entry.ServedModelTag] = struct{}{}
		variant := modelVariantFromProviderEntry(entry)
		if _, exists := seenIDs[variant.GetVariantId()]; exists {
			return nil, fmt.Errorf("evaluation: build model variants: duplicate variant_id %q", variant.GetVariantId())
		}
		seenIDs[variant.GetVariantId()] = struct{}{}
		if opts.RunCapabilityProbes {
			if opts.CapabilityProbeRunner == nil {
				return nil, fmt.Errorf("evaluation: build model variants: capability probes requested without governed probe runner")
			}
			observations, err := opts.CapabilityProbeRunner.RunCapabilityProbes(ctx, variant)
			if err != nil {
				return nil, err
			}
			variant.CapabilityObservations = observations
		}
		variants = append(variants, variant)
	}
	return variants, nil
}

func modelVariantFromProviderEntry(entry inference.ProviderModelInventoryEntry) *evalv1.ModelVariant {
	providerClass := entry.ProviderClass
	if providerClass == "" {
		providerClass = "ollama"
	}
	return &evalv1.ModelVariant{
		VariantId:      inference.NormalizeProviderModelVariantID(entry.ServedModelTag),
		ProviderClass:  providerClass,
		ServedModelTag: entry.ServedModelTag,
		ModelDigest:    entry.ModelDigest,
		ModelFamily:    entry.ModelFamily,
		ParameterCount: entry.ParameterCount,
		Quantization:   entry.Quantization,
		ContextLimit:   entry.ContextLimit,
	}
}

// MaterializeModelRegistry binds the discovered variants to one campaign registry digest.
func MaterializeModelRegistry(campaignID string, variants []*evalv1.ModelVariant) (*ModelInventoryFreeze, error) {
	if campaignID == "" || len(variants) == 0 {
		return nil, fmt.Errorf("evaluation: materialize model registry: %w", constants.ErrMissingRequiredField)
	}
	registryDigest, err := ComputeModelVariantRegistryDigest(campaignID, variants)
	if err != nil {
		return nil, fmt.Errorf("evaluation: materialize model registry: %w", err)
	}
	inferenceVariants := make([]*operatorv1.InferenceModelVariant, 0, len(variants))
	for _, variant := range variants {
		inferenceVariants = append(inferenceVariants, &operatorv1.InferenceModelVariant{
			Model:  variant.GetServedModelTag(),
			Digest: variant.GetModelDigest(),
		})
	}
	return &ModelInventoryFreeze{
		CampaignID:           campaignID,
		RegistryDigest:       registryDigest,
		Variants:             append([]*evalv1.ModelVariant(nil), variants...),
		InferenceVariants:    inferenceVariants,
		HomogeneousCellCount: ComputeHomogeneousMatrixSize(uint64(len(variants))),
	}, nil
}

// ValidateModelRegistry verifies the Phase 3 inventory gate.
func ValidateModelRegistry(freeze *ModelInventoryFreeze) error {
	if freeze == nil || freeze.CampaignID == "" || freeze.RegistryDigest == "" || len(freeze.Variants) == 0 {
		return fmt.Errorf("evaluation: validate model registry: %w", constants.ErrMissingRequiredField)
	}
	expectedDigest, err := ComputeModelVariantRegistryDigest(freeze.CampaignID, freeze.Variants)
	if err != nil {
		return err
	}
	if freeze.RegistryDigest != expectedDigest {
		return fmt.Errorf("evaluation: validate model registry: registry digest mismatch")
	}
	seenTags := make(map[string]struct{}, len(freeze.Variants))
	seenIDs := make(map[string]struct{}, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		if variant == nil || variant.GetVariantId() == "" || variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" || variant.GetProviderClass() == "" {
			return fmt.Errorf("evaluation: validate model registry: %w", constants.ErrMissingRequiredField)
		}
		if _, exists := seenTags[variant.GetServedModelTag()]; exists {
			return fmt.Errorf("evaluation: validate model registry: duplicate served tag %q", variant.GetServedModelTag())
		}
		if _, exists := seenIDs[variant.GetVariantId()]; exists {
			return fmt.Errorf("evaluation: validate model registry: duplicate variant_id %q", variant.GetVariantId())
		}
		seenTags[variant.GetServedModelTag()] = struct{}{}
		seenIDs[variant.GetVariantId()] = struct{}{}
	}
	expectedCells := ComputeHomogeneousMatrixSize(uint64(len(freeze.Variants)))
	if freeze.HomogeneousCellCount != expectedCells {
		return fmt.Errorf("evaluation: validate model registry: homogeneous matrix size mismatch")
	}
	if expectedCells == 0 {
		return fmt.Errorf("evaluation: validate model registry: empty smoke matrix")
	}
	return nil
}

// ComputeHomogeneousMatrixSize returns model variants × roles × scenarios.
func ComputeHomogeneousMatrixSize(variantCount uint64) uint64 {
	return variantCount * HomogeneousRoleCount * StandardScenarioCount
}

// LookupModelVariant returns the frozen eval variant for a served model tag.
func (freeze *ModelInventoryFreeze) LookupModelVariant(servedModelTag string) (*evalv1.ModelVariant, error) {
	if freeze == nil {
		return nil, fmt.Errorf("evaluation: lookup model variant: %w", constants.ErrMissingRequiredField)
	}
	for _, variant := range freeze.Variants {
		if variant != nil && variant.GetServedModelTag() == servedModelTag {
			return variant, nil
		}
	}
	return nil, fmt.Errorf("evaluation: lookup model variant: %w: %s", constants.ErrInferenceModelNotFound, servedModelTag)
}

// LoadModelInventoryFreezeFromRuntime reads a freeze through RuntimeFileService.
func LoadModelInventoryFreezeFromRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string) (*ModelInventoryFreeze, error) {
	if fileSvc == nil || relPath == "" {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: %w", constants.ErrMissingRequiredField)
	}
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: %w", err)
	}
	return parseModelInventoryFreeze(data)
}

// LoadModelInventoryFreezeFile reads a Phase 3 inventory freeze JSON export.
func LoadModelInventoryFreezeFile(path string) (*ModelInventoryFreeze, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: %w", err)
	}
	var payload struct {
		CampaignID          string            `json:"campaign_id"`
		ModelRegistryDigest string            `json:"model_registry_digest"`
		Variants            []json.RawMessage `json:"variants"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: decode: %w", err)
	}
	if payload.ModelRegistryDigest == "" || len(payload.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: %w", constants.ErrMissingRequiredField)
	}
	variants := make([]*evalv1.ModelVariant, 0, len(payload.Variants))
	for _, raw := range payload.Variants {
		variant := &evalv1.ModelVariant{}
		if err := protojson.Unmarshal(raw, variant); err != nil {
			return nil, fmt.Errorf("evaluation: load model inventory freeze file: decode variant: %w", err)
		}
		variants = append(variants, variant)
	}
	return MaterializeModelRegistry(payload.CampaignID, variants)
}

func parseModelInventoryFreeze(data []byte) (*ModelInventoryFreeze, error) {
	var payload struct {
		CampaignID          string            `json:"campaign_id"`
		ModelRegistryDigest string            `json:"model_registry_digest"`
		Variants            []json.RawMessage `json:"variants"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: decode: %w", err)
	}
	if payload.ModelRegistryDigest == "" || len(payload.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: load model inventory freeze file: %w", constants.ErrMissingRequiredField)
	}
	variants := make([]*evalv1.ModelVariant, 0, len(payload.Variants))
	for _, raw := range payload.Variants {
		variant := &evalv1.ModelVariant{}
		if err := protojson.Unmarshal(raw, variant); err != nil {
			return nil, fmt.Errorf("evaluation: load model inventory freeze file: decode variant: %w", err)
		}
		variants = append(variants, variant)
	}
	return MaterializeModelRegistry(payload.CampaignID, variants)
}

// ToModelRegistryFreeze converts the eval inventory into the inference registry
// shape used by governed probe and chat acceptance commands.
func (freeze *ModelInventoryFreeze) ToModelRegistryFreeze() *ModelRegistryFreeze {
	if freeze == nil {
		return nil
	}
	return &ModelRegistryFreeze{
		CampaignID: freeze.CampaignID,
		Digest:     freeze.RegistryDigest,
		Variants:   freeze.InferenceVariants,
	}
}

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
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const StandardScenarioCount = 25

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
	RunCapabilityProbes   bool
	CapabilityProbeRunner GovernedCapabilityProbeRunner
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

// ComputeHomogeneousMatrixSize returns model variants × the standard catalog's
// (scenario, role) cells, counting each scenario once per role it declares
// eligible.
func ComputeHomogeneousMatrixSize(variantCount uint64) uint64 {
	var cellsPerVariant uint64
	for _, blueprint := range scenarioBlueprints() {
		cellsPerVariant += uint64(len(blueprint.EligibleRoles))
	}
	return variantCount * cellsPerVariant
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

// ParseParameterCount parses human parameter count strings such as "12b", "8B", "3.8b", "700m", "135M", or raw integers.
func ParseParameterCount(raw string) (uint64, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return 0, nil
	}
	multiplier := float64(1)
	switch {
	case strings.HasSuffix(s, "b"):
		multiplier = 1_000_000_000
		s = strings.TrimSuffix(s, "b")
	case strings.HasSuffix(s, "m"):
		multiplier = 1_000_000
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		multiplier = 1_000
		s = strings.TrimSuffix(s, "k")
	}
	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("evaluation: parse parameter count %q: %w", raw, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("evaluation: parse parameter count %q: must be non-negative", raw)
	}
	return uint64(math.Round(val * multiplier)), nil
}

// FormatParameterCount formats a parameter count into human readable shorthand (e.g. 12B, 3.8B, 700M).
func FormatParameterCount(count uint64) string {
	if count == 0 {
		return "-"
	}
	switch {
	case count >= 1_000_000_000:
		val := float64(count) / 1_000_000_000
		if val == math.Floor(val) {
			return fmt.Sprintf("%.0fB", val)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", val), "0"), ".") + "B"
	case count >= 1_000_000:
		val := float64(count) / 1_000_000
		if val == math.Floor(val) {
			return fmt.Sprintf("%.0fM", val)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", val), "0"), ".") + "M"
	case count >= 1_000:
		val := float64(count) / 1_000
		if val == math.Floor(val) {
			return fmt.Sprintf("%.0fK", val)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", val), "0"), ".") + "K"
	default:
		return fmt.Sprintf("%d", count)
	}
}

// FilterVariantsByMaxParameters returns variants with parameterCount <= maxParams.
// If maxParams is 0, all variants are returned.
func FilterVariantsByMaxParameters(variants []*evalv1.ModelVariant, maxParams uint64) []*evalv1.ModelVariant {
	if maxParams == 0 {
		return variants
	}
	filtered := make([]*evalv1.ModelVariant, 0, len(variants))
	for _, v := range variants {
		if v == nil {
			continue
		}
		if v.GetParameterCount() > 0 && v.GetParameterCount() > maxParams {
			continue
		}
		filtered = append(filtered, v)
	}
	return filtered
}

// FilterVariantsByFamily returns variants matching modelFamily (case-insensitive substring).
func FilterVariantsByFamily(variants []*evalv1.ModelVariant, family string) []*evalv1.ModelVariant {
	family = strings.TrimSpace(strings.ToLower(family))
	if family == "" {
		return variants
	}
	filtered := make([]*evalv1.ModelVariant, 0, len(variants))
	for _, v := range variants {
		if v == nil {
			continue
		}
		if strings.EqualFold(v.GetModelFamily(), family) || strings.Contains(strings.ToLower(v.GetModelFamily()), family) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// AddOrUpdateModelVariant adds a new model variant or replaces an existing one matching
// the served model tag or variant ID, preserving canonical tag sorting and valid registry digest.
func AddOrUpdateModelVariant(freeze *ModelInventoryFreeze, variant *evalv1.ModelVariant) (*ModelInventoryFreeze, error) {
	if freeze == nil || variant == nil {
		return nil, fmt.Errorf("evaluation: add model variant: %w", constants.ErrMissingRequiredField)
	}
	if variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" {
		return nil, fmt.Errorf("evaluation: add model variant: served model tag and digest required: %w", constants.ErrMissingRequiredField)
	}
	if variant.GetVariantId() == "" {
		variant.VariantId = inference.NormalizeProviderModelVariantID(variant.GetServedModelTag())
	}
	if variant.GetProviderClass() == "" {
		variant.ProviderClass = "ollama"
	}

	variants := make([]*evalv1.ModelVariant, 0, len(freeze.Variants)+1)
	replaced := false
	for _, existing := range freeze.Variants {
		if existing == nil {
			continue
		}
		if existing.GetServedModelTag() == variant.GetServedModelTag() || existing.GetVariantId() == variant.GetVariantId() {
			variants = append(variants, variant)
			replaced = true
		} else {
			variants = append(variants, existing)
		}
	}
	if !replaced {
		variants = append(variants, variant)
	}
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].GetServedModelTag() < variants[j].GetServedModelTag()
	})

	updatedFreeze, err := MaterializeModelRegistry(freeze.CampaignID, variants)
	if err != nil {
		return nil, err
	}
	if err := ValidateModelRegistry(updatedFreeze); err != nil {
		return nil, err
	}
	return updatedFreeze, nil
}

// MergeModelVariants merges source variants into target freeze, replacing duplicates by served tag.
func MergeModelVariants(targetFreeze *ModelInventoryFreeze, sourceVariants []*evalv1.ModelVariant) (*ModelInventoryFreeze, error) {
	if targetFreeze == nil {
		return nil, fmt.Errorf("evaluation: merge model variants: %w", constants.ErrMissingRequiredField)
	}
	if len(sourceVariants) == 0 {
		return targetFreeze, nil
	}
	byTag := make(map[string]*evalv1.ModelVariant, len(targetFreeze.Variants))
	for _, v := range targetFreeze.Variants {
		if v != nil {
			byTag[v.GetServedModelTag()] = v
		}
	}
	for _, v := range sourceVariants {
		if v == nil || v.GetServedModelTag() == "" {
			continue
		}
		if v.GetVariantId() == "" {
			v.VariantId = inference.NormalizeProviderModelVariantID(v.GetServedModelTag())
		}
		if v.GetProviderClass() == "" {
			v.ProviderClass = "ollama"
		}
		byTag[v.GetServedModelTag()] = v
	}
	merged := make([]*evalv1.ModelVariant, 0, len(byTag))
	for _, v := range byTag {
		merged = append(merged, v)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].GetServedModelTag() < merged[j].GetServedModelTag()
	})
	newFreeze, err := MaterializeModelRegistry(targetFreeze.CampaignID, merged)
	if err != nil {
		return nil, err
	}
	if err := ValidateModelRegistry(newFreeze); err != nil {
		return nil, err
	}
	return newFreeze, nil
}

// RemoveModelVariant removes a model variant matching served model tag or variant ID,
// recalculating the registry digest and homogeneous matrix size.
func RemoveModelVariant(freeze *ModelInventoryFreeze, identifier string) (*ModelInventoryFreeze, *evalv1.ModelVariant, error) {
	if freeze == nil {
		return nil, nil, fmt.Errorf("evaluation: remove model variant: %w", constants.ErrMissingRequiredField)
	}
	id := strings.TrimSpace(identifier)
	if id == "" {
		return nil, nil, fmt.Errorf("evaluation: remove model variant: identifier is required")
	}

	normalizedID := inference.NormalizeProviderModelVariantID(id)

	var removed *evalv1.ModelVariant
	remaining := make([]*evalv1.ModelVariant, 0, len(freeze.Variants))
	for _, v := range freeze.Variants {
		if v == nil {
			continue
		}
		if removed == nil && (strings.EqualFold(v.GetServedModelTag(), id) ||
			strings.EqualFold(v.GetVariantId(), id) ||
			strings.EqualFold(v.GetVariantId(), normalizedID)) {
			removed = v
			continue
		}
		remaining = append(remaining, v)
	}

	if removed == nil {
		return nil, nil, fmt.Errorf("evaluation: remove model variant: model %q not found in inventory", id)
	}

	if len(remaining) == 0 {
		return nil, nil, fmt.Errorf("evaluation: remove model variant: cannot remove last variant from inventory")
	}

	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].GetServedModelTag() < remaining[j].GetServedModelTag()
	})

	updatedFreeze, err := MaterializeModelRegistry(freeze.CampaignID, remaining)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateModelRegistry(updatedFreeze); err != nil {
		return nil, nil, err
	}
	return updatedFreeze, removed, nil
}

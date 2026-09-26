// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

// ProviderModelInventoryEntryFromProto converts one protocol inventory entry
// into the evaluation/inference domain shape.
func ProviderModelInventoryEntryFromProto(entry *operatorv1.ProviderModelInventoryEntry) ProviderModelInventoryEntry {
	if entry == nil {
		return ProviderModelInventoryEntry{}
	}
	return ProviderModelInventoryEntry{
		ProviderClass:          entry.GetProviderClass(),
		ServedModelTag:         entry.GetServedModelTag(),
		ModelDigest:            entry.GetModelDigest(),
		ModelFamily:            entry.GetModelFamily(),
		ParameterSize:          entry.GetParameterSize(),
		ParameterCount:         entry.GetParameterCount(),
		Quantization:           entry.GetQuantization(),
		Format:                 entry.GetFormat(),
		ContextLimit:           entry.GetContextLimit(),
		AdvertisedCapabilities: append([]string(nil), entry.GetAdvertisedCapabilities()...),
	}
}

// ProviderModelInventoryEntriesFromProto converts protocol inventory entries
// into the evaluation/inference domain shape.
func ProviderModelInventoryEntriesFromProto(entries []*operatorv1.ProviderModelInventoryEntry) []ProviderModelInventoryEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]ProviderModelInventoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, ProviderModelInventoryEntryFromProto(entry))
	}
	return out
}

// ProviderModelInventoryEntriesToProto converts domain inventory entries into
// the protocol shape returned by governed Ollama maintenance queries.
func ProviderModelInventoryEntriesToProto(entries []ProviderModelInventoryEntry) []*operatorv1.ProviderModelInventoryEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*operatorv1.ProviderModelInventoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, &operatorv1.ProviderModelInventoryEntry{
			ProviderClass:          entry.ProviderClass,
			ServedModelTag:         entry.ServedModelTag,
			ModelDigest:            entry.ModelDigest,
			ModelFamily:            entry.ModelFamily,
			ParameterSize:          entry.ParameterSize,
			ParameterCount:         entry.ParameterCount,
			Quantization:           entry.Quantization,
			Format:                 entry.Format,
			ContextLimit:           entry.ContextLimit,
			AdvertisedCapabilities: append([]string(nil), entry.AdvertisedCapabilities...),
		})
	}
	return out
}

// ProviderResidencyFromProto converts one protocol residency result into the
// inference domain shape.
func ProviderResidencyFromProto(result *operatorv1.OllamaModelResidencyResult) ProviderResidency {
	if result == nil {
		return ProviderResidency{}
	}
	models := make([]ProviderResidencyModel, 0, len(result.GetModels()))
	for _, model := range result.GetModels() {
		if model == nil || model.GetName() == "" {
			continue
		}
		models = append(models, ProviderResidencyModel{Name: model.GetName()})
	}
	return ProviderResidency{Models: models}
}

// ProviderResidencyToProto converts domain residency into the protocol shape.
func ProviderResidencyToProto(residency ProviderResidency) *operatorv1.OllamaModelResidencyResult {
	models := make([]*operatorv1.OllamaModelResidencyModel, 0, len(residency.Models))
	for _, model := range residency.Models {
		if model.Name == "" {
			continue
		}
		models = append(models, &operatorv1.OllamaModelResidencyModel{Name: model.Name})
	}
	return &operatorv1.OllamaModelResidencyResult{Models: models}
}

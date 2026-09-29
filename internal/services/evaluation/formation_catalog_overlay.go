// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const FormationCatalogOverlaySchemaVersion = "formation-catalog-overlay.v1"

// FormationCatalogOverlay is the user-editable delta applied on top of the
// checked-in default formation catalog: formations to add or replace, and
// default formation IDs to drop.
type FormationCatalogOverlay struct {
	Formations          []Formation
	RemovedFormationIDs []string
}

type persistedFormationCatalogOverlay struct {
	SchemaVersion       string                  `json:"schema_version"`
	Formations          []persistedCatalogEntry `json:"formations"`
	RemovedFormationIDs []string                `json:"removed_formation_ids,omitempty"`
}

type persistedCatalogEntry struct {
	ID                string                     `json:"id"`
	DisplayName       string                     `json:"display_name"`
	Description       string                     `json:"description"`
	MaxVRAMMiB        uint64                     `json:"max_vram_mib"`
	RelaxedValidation bool                       `json:"relaxed_validation,omitempty"`
	Primary           persistedCatalogEntryModel `json:"primary"`
	Assistant         persistedCatalogEntryModel `json:"assistant"`
	Lite              persistedCatalogEntryModel `json:"lite"`
}

type persistedCatalogEntryModel struct {
	VariantID             string `json:"variant_id"`
	DisplayName           string `json:"display_name"`
	Provider              string `json:"provider"`
	Family                string `json:"family"`
	ProviderClass         string `json:"provider_class"`
	ServedModelTag        string `json:"served_model_tag"`
	Trust                 string `json:"trust"`
	Quantization          string `json:"quantization,omitempty"`
	ParameterCount        uint64 `json:"parameter_count,omitempty"`
	EstimatedModelVRAMMiB uint64 `json:"estimated_model_vram_mib,omitempty"`
	EstimatedKVCacheMiB   uint64 `json:"estimated_kv_cache_mib,omitempty"`
	ModelDigest           string `json:"model_digest,omitempty"`
}

func persistedCatalogEntryFromFormation(formation Formation) persistedCatalogEntry {
	return persistedCatalogEntry{
		ID:                formation.ID,
		DisplayName:       formation.DisplayName,
		Description:       formation.Description,
		MaxVRAMMiB:        formation.MaxVRAMMiB,
		RelaxedValidation: formation.RelaxedValidation,
		Primary:           persistedCatalogEntryModelFromFormationModel(formation.Primary),
		Assistant:         persistedCatalogEntryModelFromFormationModel(formation.Assistant),
		Lite:              persistedCatalogEntryModelFromFormationModel(formation.Lite),
	}
}

func persistedCatalogEntryModelFromFormationModel(model FormationModel) persistedCatalogEntryModel {
	return persistedCatalogEntryModel{
		VariantID:             model.VariantID,
		DisplayName:           model.DisplayName,
		Provider:              model.Provider,
		Family:                model.Family,
		ProviderClass:         model.ProviderClass,
		ServedModelTag:        model.ServedModelTag,
		Trust:                 string(model.Trust),
		Quantization:          model.Quantization,
		ParameterCount:        model.ParameterCount,
		EstimatedModelVRAMMiB: model.EstimatedModelVRAMMiB,
		EstimatedKVCacheMiB:   model.EstimatedKVCacheMiB,
		ModelDigest:           model.ModelDigest,
	}
}

func (e persistedCatalogEntry) toFormation() Formation {
	return Formation{
		ID:                e.ID,
		DisplayName:       e.DisplayName,
		Description:       e.Description,
		MaxVRAMMiB:        e.MaxVRAMMiB,
		RelaxedValidation: e.RelaxedValidation,
		Primary:           e.Primary.toFormationModel(),
		Assistant:         e.Assistant.toFormationModel(),
		Lite:              e.Lite.toFormationModel(),
	}
}

func (m persistedCatalogEntryModel) toFormationModel() FormationModel {
	return FormationModel{
		VariantID:             m.VariantID,
		DisplayName:           m.DisplayName,
		Provider:              m.Provider,
		Family:                m.Family,
		ProviderClass:         m.ProviderClass,
		ServedModelTag:        m.ServedModelTag,
		Trust:                 FormationTrust(m.Trust),
		Quantization:          m.Quantization,
		ParameterCount:        m.ParameterCount,
		EstimatedModelVRAMMiB: m.EstimatedModelVRAMMiB,
		EstimatedKVCacheMiB:   m.EstimatedKVCacheMiB,
		ModelDigest:           m.ModelDigest,
	}
}

// MarshalFormationCatalogOverlay renders the overlay as its checked-in JSON form.
func MarshalFormationCatalogOverlay(overlay FormationCatalogOverlay) ([]byte, error) {
	payload := persistedFormationCatalogOverlay{
		SchemaVersion:       FormationCatalogOverlaySchemaVersion,
		Formations:          make([]persistedCatalogEntry, 0, len(overlay.Formations)),
		RemovedFormationIDs: overlay.RemovedFormationIDs,
	}
	for _, formation := range overlay.Formations {
		payload.Formations = append(payload.Formations, persistedCatalogEntryFromFormation(formation))
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal formation catalog overlay: %w", err)
	}
	return data, nil
}

// LoadFormationCatalogOverlayFile reads the checked-in formation catalog
// overlay. A missing file wraps the underlying os error so callers can test
// it with errors.Is(err, fs.ErrNotExist).
func LoadFormationCatalogOverlayFile(path string) (*FormationCatalogOverlay, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load formation catalog overlay: %w", err)
	}
	var payload persistedFormationCatalogOverlay
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: load formation catalog overlay: decode: %w", err)
	}
	overlay := FormationCatalogOverlay{
		Formations:          make([]Formation, 0, len(payload.Formations)),
		RemovedFormationIDs: payload.RemovedFormationIDs,
	}
	for _, entry := range payload.Formations {
		overlay.Formations = append(overlay.Formations, entry.toFormation())
	}
	return &overlay, nil
}

// ApplyFormationCatalogOverlay layers overlay on top of the checked-in default
// formation catalog: upserts run first, then removals, so an overlay that
// replaces every default before dropping them never trips the "cannot remove
// the last formation" guard.
func ApplyFormationCatalogOverlay(overlay *FormationCatalogOverlay) (*ExecutionTopologies, error) {
	topologies, err := NewExecutionTopologies()
	if err != nil {
		return nil, err
	}
	if overlay == nil {
		return topologies, nil
	}
	for _, formation := range overlay.Formations {
		topologies, err = AddOrUpdateFormation(topologies, formation)
		if err != nil {
			return nil, err
		}
	}
	for _, id := range overlay.RemovedFormationIDs {
		updated, _, err := RemoveFormation(topologies, id)
		if err != nil {
			if errors.Is(err, constants.ErrFormationInvalid) {
				continue
			}
			return nil, err
		}
		topologies = updated
	}
	return topologies, nil
}

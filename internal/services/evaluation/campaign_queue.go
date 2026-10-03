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
	"path"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	// Runtime paths are relative to RuntimeFileService. Checked-in templates use the repository root.
	DefaultInitCampaignQueueRelPath    = constants.EvaluationInitCampaignQueuePath
	DefaultModelInventoryRelPath       = constants.EvaluationModelInventoryPath
	DefaultCampaignInventoryRelDirname = constants.EvaluationDirname + "/" + constants.EvaluationInventoriesDirname

	// Checked-in genesis program inventory.
	DefaultBaseModelInventoryRelPath      = "eval/base-model-inventory.json"
	DefaultBaseInitCampaignQueueRelPath   = "eval/base-init-campaign-queue.json"
	DefaultRolloutIntakePriorityRelPath   = "eval/rollout-intake-priority.json"
	DefaultFormationCatalogOverlayRelPath = "eval/formation-catalog-overlay.json"
	DefaultGenesisHomogeneousCampaignID   = "eval-genesis-homogeneous"
)

// QueueLogDir returns the canonical runtime-relative directory for one rollout.
func QueueLogDir(runName string) string {
	runName = strings.TrimSpace(runName)
	if runName == "" || runName == "." || runName == ".." || strings.ContainsAny(runName, `/\\`) {
		return ""
	}
	return path.Join(constants.EvaluationQueueLogsDirname, runName)
}

func campaignInventoryPath(inventoryRelDir, campaignID string) string {
	return path.Join(inventoryRelDir, campaignID+constants.FileExtJSON)
}

// CampaignQueueModel summarizes one init-campaign queue entry.
type CampaignQueueModel struct {
	VariantID            string `json:"variant_id"`
	ServedModelTag       string `json:"served_model_tag"`
	CampaignID           string `json:"campaign_id"`
	InventoryFile        string `json:"inventory_file"`
	ModelRegistryDigest  string `json:"model_registry_digest,omitempty"`
	HomogeneousCellCount uint64 `json:"homogeneous_cell_count"`
	Status               string `json:"status"`
	VerifiedRunID        string `json:"verified_run_id,omitempty"`
	Notes                string `json:"notes,omitempty"`
}

// CampaignQueue is the init-campaign rollout manifest.
type CampaignQueue struct {
	Pattern         string               `json:"pattern"`
	CellsPerRun     uint64               `json:"cells_per_run"`
	InventoryDir    string               `json:"inventory_dir"`
	GenerateCommand string               `json:"generate_command"`
	Models          []CampaignQueueModel `json:"models"`
}

// LoadInitCampaignQueueFromRuntime reads the rollout manifest through the runtime file service.
func LoadInitCampaignQueueFromRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string) (*CampaignQueue, error) {
	if fileSvc == nil || relPath == "" {
		return nil, fmt.Errorf("evaluation: load init campaign queue: %w", constants.ErrMissingRequiredField)
	}
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load init campaign queue: %w", err)
	}
	queue := &CampaignQueue{}
	if err := json.Unmarshal(data, queue); err != nil {
		return nil, fmt.Errorf("evaluation: load init campaign queue: decode: %w", err)
	}
	if len(queue.Models) == 0 {
		return nil, fmt.Errorf("evaluation: load init campaign queue: %w", constants.ErrMissingRequiredField)
	}
	return queue, nil
}

// SaveInitCampaignQueueToRuntime writes the rollout manifest through the runtime file service.
func SaveInitCampaignQueueToRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string, queue *CampaignQueue) error {
	if fileSvc == nil || relPath == "" || queue == nil || len(queue.Models) == 0 {
		return fmt.Errorf("evaluation: save init campaign queue: %w", constants.ErrMissingRequiredField)
	}
	body, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: save init campaign queue: encode: %w", err)
	}
	if err := fileSvc.WriteFile(ctx, relPath, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: save init campaign queue: %w", err)
	}
	return nil
}

// NextPending returns the first queue entry with status pending.
func (queue *CampaignQueue) NextPending() (*CampaignQueueModel, error) {
	if queue == nil {
		return nil, fmt.Errorf("evaluation: init campaign queue next pending: %w", constants.ErrMissingRequiredField)
	}
	for _, entry := range queue.Models {
		if strings.EqualFold(entry.Status, QueueStatusPending) {
			selected := entry
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("evaluation: init campaign queue: no pending models: %w", constants.ErrNotFound)
}

// FindByTagOrVariantID resolves one queue entry by served tag or variant ID.
func (queue *CampaignQueue) FindByTagOrVariantID(query string) (*CampaignQueueModel, error) {
	if queue == nil || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("evaluation: init campaign queue lookup: %w", constants.ErrMissingRequiredField)
	}
	query = strings.TrimSpace(query)
	for _, entry := range queue.Models {
		if entry.ServedModelTag == query || entry.VariantID == query {
			selected := entry
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("evaluation: init campaign queue: model %q: %w", query, constants.ErrNotFound)
}

// FilterByStatus returns queue entries matching one status, or all entries when status is empty or "all".
func (queue *CampaignQueue) FilterByStatus(status string) []CampaignQueueModel {
	if queue == nil {
		return nil
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" || status == "all" {
		return append([]CampaignQueueModel(nil), queue.Models...)
	}
	filtered := make([]CampaignQueueModel, 0)
	for _, entry := range queue.Models {
		if strings.EqualFold(entry.Status, status) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// LoadFrozenVariantsFromRuntime reads a frozen inventory through RuntimeFileService.
func LoadFrozenVariantsFromRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string) ([]*evalv1.ModelVariant, error) {
	if fileSvc == nil || relPath == "" {
		return nil, fmt.Errorf("evaluation: load frozen variants: %w", constants.ErrMissingRequiredField)
	}
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load frozen variants: %w", err)
	}
	return parseFrozenVariants(data)
}

// LoadFrozenVariantsFromExternalSource reads a frozen model inventory export
// from an explicit external or checked-in source path.
func LoadFrozenVariantsFromExternalSource(path string) ([]*evalv1.ModelVariant, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load frozen variants: %w", err)
	}
	var payload struct {
		Variants []json.RawMessage `json:"variants"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: load frozen variants: decode: %w", err)
	}
	variants := make([]*evalv1.ModelVariant, 0, len(payload.Variants))
	for _, raw := range payload.Variants {
		variant := &evalv1.ModelVariant{}
		if err := protojson.Unmarshal(raw, variant); err != nil {
			return nil, fmt.Errorf("evaluation: load frozen variants: decode variant: %w", err)
		}
		variants = append(variants, variant)
	}
	SortModelVariantsForRollout(variants)
	return variants, nil
}

func parseFrozenVariants(data []byte) ([]*evalv1.ModelVariant, error) {
	var payload struct {
		Variants []json.RawMessage `json:"variants"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: load frozen variants: decode: %w", err)
	}
	variants := make([]*evalv1.ModelVariant, 0, len(payload.Variants))
	for _, raw := range payload.Variants {
		variant := &evalv1.ModelVariant{}
		if err := protojson.Unmarshal(raw, variant); err != nil {
			return nil, fmt.Errorf("evaluation: load frozen variants: decode variant: %w", err)
		}
		variants = append(variants, variant)
	}
	SortModelVariantsForRollout(variants)
	return variants, nil
}

func SortModelVariantsForRollout(variants []*evalv1.ModelVariant) {
	sort.SliceStable(variants, func(i, j int) bool {
		left := variants[i]
		right := variants[j]
		if left == nil || right == nil {
			return left != nil
		}
		leftParameterCount := left.GetParameterCount()
		rightParameterCount := right.GetParameterCount()
		if leftParameterCount == 0 || rightParameterCount == 0 {
			if leftParameterCount != rightParameterCount {
				return rightParameterCount == 0
			}
		} else if leftParameterCount != rightParameterCount {
			return leftParameterCount < rightParameterCount
		}
		if left.GetServedModelTag() != right.GetServedModelTag() {
			return left.GetServedModelTag() < right.GetServedModelTag()
		}
		return left.GetVariantId() < right.GetVariantId()
	})
}

// PrioritizeRolloutIntake moves configured intake variant IDs to the front while
// preserving size ordering for the remaining backlog.
func PrioritizeRolloutIntake(variants []*evalv1.ModelVariant, priorityIDs []string) []*evalv1.ModelVariant {
	if len(variants) == 0 || len(priorityIDs) == 0 {
		return variants
	}
	byID := make(map[string]*evalv1.ModelVariant, len(variants))
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		byID[variant.GetVariantId()] = variant
	}
	ordered := make([]*evalv1.ModelVariant, 0, len(variants))
	for _, id := range priorityIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if variant, ok := byID[id]; ok {
			ordered = append(ordered, variant)
			delete(byID, id)
		}
	}
	rest := make([]*evalv1.ModelVariant, 0, len(byID))
	for _, variant := range byID {
		rest = append(rest, variant)
	}
	SortModelVariantsForRollout(rest)
	return append(ordered, rest...)
}

// CampaignIDForVariant returns the canonical campaign ID for one model.
func CampaignIDForVariant(variant *evalv1.ModelVariant) string {
	if variant == nil {
		return ""
	}
	if variant.GetServedModelTag() == "gemma4:e4b" {
		return "init-campaign"
	}
	return "eval-init-" + variant.GetVariantId()
}

func marshalModelInventoryFreeze(freeze *ModelInventoryFreeze) ([]byte, error) {
	if freeze == nil {
		return nil, fmt.Errorf("evaluation: marshal model inventory freeze: %w", constants.ErrMissingRequiredField)
	}
	variantBodies := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		body, err := protojson.Marshal(variant)
		if err != nil {
			return nil, fmt.Errorf("evaluation: marshal model inventory freeze: marshal variant: %w", err)
		}
		variantBodies = append(variantBodies, body)
	}
	payload, err := json.MarshalIndent(struct {
		CampaignID           string            `json:"campaign_id"`
		ModelRegistryDigest  string            `json:"model_registry_digest"`
		ModelCount           int               `json:"model_count"`
		HomogeneousCellCount uint64            `json:"homogeneous_cell_count"`
		Variants             []json.RawMessage `json:"variants"`
	}{
		CampaignID:           freeze.CampaignID,
		ModelRegistryDigest:  freeze.RegistryDigest,
		ModelCount:           len(freeze.Variants),
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		Variants:             variantBodies,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal model inventory freeze: %w", err)
	}
	return payload, nil
}

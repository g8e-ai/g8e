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
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const initCampaignQueueGenerateCommand = "./g8e eval rollout init --materialize --merge"

// MaterializeInitCampaignInventoryRequest writes one single-model inventory file.
type MaterializeInitCampaignInventoryRequest struct {
	Context         context.Context
	FileService     fs.RuntimeFileService
	ProjectRoot     string
	InventoryRelDir string
	Variant         *evalv1.ModelVariant
}

// MaterializeInitCampaignInventory freezes one model variant to a per-campaign inventory file.
func MaterializeInitCampaignInventory(req MaterializeInitCampaignInventoryRequest) (CampaignQueueModel, error) {
	if req.Variant == nil || req.FileService == nil {
		return CampaignQueueModel{}, fmt.Errorf("evaluation: materialize init campaign inventory: %w", constants.ErrMissingRequiredField)
	}
	inventoryRelDir := req.InventoryRelDir
	if inventoryRelDir == "" {
		inventoryRelDir = DefaultCampaignInventoryRelDirname
	}
	campaignID := CampaignIDForVariant(req.Variant)
	freeze, err := MaterializeModelRegistry(campaignID, []*evalv1.ModelVariant{req.Variant})
	if err != nil {
		return CampaignQueueModel{}, err
	}
	if err := ValidateModelRegistry(freeze); err != nil {
		return CampaignQueueModel{}, err
	}

	inventoryRelPath := filepath.ToSlash(filepath.Join(inventoryRelDir, campaignID+".json"))
	if req.Context == nil {
		req.Context = context.Background()
	}
	payload, err := marshalModelInventoryFreeze(freeze)
	if err != nil {
		return CampaignQueueModel{}, err
	}
	if err := req.FileService.WriteFile(req.Context, inventoryRelPath, payload, constants.PermFileReadOnly); err != nil {
		return CampaignQueueModel{}, fmt.Errorf("evaluation: materialize init campaign inventory: write: %w", err)
	}

	return CampaignQueueModel{
		VariantID:            req.Variant.GetVariantId(),
		ServedModelTag:       req.Variant.GetServedModelTag(),
		CampaignID:           campaignID,
		InventoryFile:        filepath.ToSlash(inventoryRelPath),
		ModelRegistryDigest:  freeze.RegistryDigest,
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		Status:               "pending",
	}, nil
}

// MaterializeCampaignInventoryRequest writes a multi-model inventory freeze file.
type MaterializeCampaignInventoryRequest struct {
	Context     context.Context
	FileService fs.RuntimeFileService
	ProjectRoot string
	CampaignID  string
	OutputPath  string
	Variants    []*evalv1.ModelVariant
}

// MaterializeCampaignInventory freezes one or more variants into a single inventory file.
func MaterializeCampaignInventory(req MaterializeCampaignInventoryRequest) (*ModelInventoryFreeze, string, error) {
	if req.FileService == nil || req.CampaignID == "" || len(req.Variants) == 0 {
		return nil, "", fmt.Errorf("evaluation: materialize campaign inventory: %w", constants.ErrMissingRequiredField)
	}
	freeze, err := MaterializeModelRegistry(req.CampaignID, req.Variants)
	if err != nil {
		return nil, "", err
	}
	if err := ValidateModelRegistry(freeze); err != nil {
		return nil, "", err
	}

	outputPath := req.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(DefaultCampaignInventoryRelDirname, req.CampaignID+".json")
	}
	outputPath = filepath.ToSlash(outputPath)
	if req.Context == nil {
		req.Context = context.Background()
	}
	payload, err := marshalModelInventoryFreeze(freeze)
	if err != nil {
		return nil, "", err
	}
	if err := req.FileService.WriteFile(req.Context, outputPath, payload, constants.PermFileReadOnly); err != nil {
		return nil, "", fmt.Errorf("evaluation: materialize campaign inventory: write: %w", err)
	}
	return freeze, outputPath, nil
}

// InitCampaignQueueRequest builds the init-campaign rollout queue manifest.
type InitCampaignQueueRequest struct {
	Context             context.Context
	FileService         fs.RuntimeFileService
	ProjectRoot         string
	SourceInventoryPath string
	InventoryRelDir     string
	OutputQueuePath     string
	Tags                []string
	Materialize         bool
	MergeExisting       bool
}

// InitCampaignQueueResult summarizes queue initialization output.
type InitCampaignQueueResult struct {
	QueuePath    string `json:"queue_path"`
	InventoryDir string `json:"inventory_dir"`
	ModelCount   int    `json:"model_count"`
	Materialized int    `json:"materialized"`
	Preserved    int    `json:"preserved_verified"`
	Queue        *CampaignQueue
}

// InitCampaignQueue materializes per-model inventories and writes the rollout queue manifest.
func InitCampaignQueue(req InitCampaignQueueRequest) (*InitCampaignQueueResult, error) {
	if req.FileService == nil || req.ProjectRoot == "" {
		return nil, fmt.Errorf("evaluation: init campaign queue: %w", constants.ErrMissingRequiredField)
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	var inventoryPath string
	var variants []*evalv1.ModelVariant
	var err error
	runtimeInventory := false
	if req.SourceInventoryPath == "" {
		inventoryPath = ResolveBaseModelInventoryPath(req.ProjectRoot)
		variants, err = LoadFrozenVariants(inventoryPath)
	} else if !filepath.IsAbs(req.SourceInventoryPath) && strings.HasPrefix(filepath.ToSlash(req.SourceInventoryPath), constants.RuntimeDirname+"/") {
		relPath := strings.TrimPrefix(filepath.ToSlash(req.SourceInventoryPath), constants.RuntimeDirname+"/")
		inventoryPath = relPath
		runtimeInventory = true
		variants, err = LoadFrozenVariantsFromRuntime(req.Context, req.FileService, relPath)
	} else {
		inventoryPath = ResolveModelInventoryPath(req.ProjectRoot, req.SourceInventoryPath)
		variants, err = LoadFrozenVariants(inventoryPath)
	}
	if err != nil {
		return nil, err
	}
	tags := req.Tags
	if len(tags) == 0 && runtimeInventory {
		tags, err = BaseModelInventoryTags(req.ProjectRoot)
		if err != nil {
			return nil, err
		}
	}
	if len(tags) > 0 {
		variants, err = VariantsByTags(variants, tags)
		if err != nil {
			return nil, err
		}
	}
	SortModelVariantsForRollout(variants)

	inventoryRelDir := req.InventoryRelDir
	if inventoryRelDir == "" {
		inventoryRelDir = DefaultCampaignInventoryRelDirname
	}

	entries := make([]CampaignQueueModel, 0, len(variants))
	materialized := 0
	for _, variant := range variants {
		entry := CampaignQueueModel{
			VariantID:            variant.GetVariantId(),
			ServedModelTag:       variant.GetServedModelTag(),
			CampaignID:           CampaignIDForVariant(variant),
			InventoryFile:        filepath.ToSlash(filepath.Join(inventoryRelDir, CampaignIDForVariant(variant)+".json")),
			HomogeneousCellCount: HomogeneousRoleCount * StandardScenarioCount,
			Status:               "pending",
		}
		if req.Materialize {
			materializedEntry, err := MaterializeInitCampaignInventory(MaterializeInitCampaignInventoryRequest{
				Context:         req.Context,
				FileService:     req.FileService,
				InventoryRelDir: inventoryRelDir,
				Variant:         variant,
			})
			if err != nil {
				return nil, err
			}
			entry = materializedEntry
			materialized++
		}
		entries = append(entries, entry)
	}

	queue := BuildInitCampaignQueue(inventoryRelDir, entries)
	preserved := 0
	if req.MergeExisting {
		mergeQueuePath := req.OutputQueuePath
		if mergeQueuePath == "" {
			mergeQueuePath = DefaultInitCampaignQueueRelPath
		}
		if existing, loadErr := LoadInitCampaignQueueFromRuntime(req.Context, req.FileService, mergeQueuePath); loadErr == nil {
			preserved = queue.MergePreservingVerifiedStatusFromQueue(existing)
		}
	}

	queuePath := req.OutputQueuePath
	if queuePath == "" {
		queuePath = DefaultInitCampaignQueueRelPath
	}
	if err := SaveInitCampaignQueueToRuntime(req.Context, req.FileService, queuePath, queue); err != nil {
		return nil, err
	}

	return &InitCampaignQueueResult{
		QueuePath:    filepath.ToSlash(queuePath),
		InventoryDir: filepath.ToSlash(inventoryRelDir),
		ModelCount:   len(entries),
		Materialized: materialized,
		Preserved:    preserved,
		Queue:        queue,
	}, nil
}

// BuildInitCampaignQueue constructs a rollout queue manifest from entries.
func BuildInitCampaignQueue(inventoryRelDir string, entries []CampaignQueueModel) *CampaignQueue {
	if inventoryRelDir == "" {
		inventoryRelDir = DefaultCampaignInventoryRelDirname
	}
	return &CampaignQueue{
		Pattern:         "eval-init-<variant_id> (legacy init-campaign for gemma4:e4b)",
		CellsPerRun:     HomogeneousRoleCount * StandardScenarioCount,
		InventoryDir:    filepath.ToSlash(inventoryRelDir),
		GenerateCommand: initCampaignQueueGenerateCommand,
		Models:          append([]CampaignQueueModel(nil), entries...),
	}
}

// MergePreservingVerifiedStatus copies verified run metadata from an existing queue file when present.
func (queue *CampaignQueue) MergePreservingVerifiedStatus(existingQueuePath string) int {
	if queue == nil {
		return 0
	}
	existing, err := LoadInitCampaignQueue(existingQueuePath)
	if err != nil {
		return 0
	}
	return queue.MergePreservingVerifiedStatusFromQueue(existing)
}

// MergePreservingVerifiedStatusFromQueue copies verified metadata from an already loaded queue.
func (queue *CampaignQueue) MergePreservingVerifiedStatusFromQueue(existing *CampaignQueue) int {
	if queue == nil || existing == nil {
		return 0
	}
	byVariant := make(map[string]CampaignQueueModel, len(existing.Models))
	for _, entry := range existing.Models {
		byVariant[entry.VariantID] = entry
	}
	preserved := 0
	for i, entry := range queue.Models {
		prev, ok := byVariant[entry.VariantID]
		if !ok || !strings.EqualFold(prev.Status, "verified") {
			continue
		}
		queue.Models[i].Status = prev.Status
		queue.Models[i].VerifiedRunID = prev.VerifiedRunID
		queue.Models[i].Notes = prev.Notes
		if prev.ModelRegistryDigest != "" {
			queue.Models[i].ModelRegistryDigest = prev.ModelRegistryDigest
		}
		preserved++
	}
	return preserved
}

// MarkCampaignQueueEntryRequest updates one queue entry after verification.
type MarkCampaignQueueEntryRequest struct {
	Context        context.Context
	FileService    fs.RuntimeFileService
	QueuePath      string
	VariantID      string
	ServedModelTag string
	Status         string
	VerifiedRunID  string
	Notes          string
}

// MarkCampaignQueueEntry updates and persists one queue entry.
func MarkCampaignQueueEntry(req MarkCampaignQueueEntryRequest) (*CampaignQueueModel, error) {
	if req.FileService == nil || req.Status == "" {
		return nil, fmt.Errorf("evaluation: mark campaign queue entry: missing required field")
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	queuePath := req.QueuePath
	if queuePath == "" {
		queuePath = DefaultInitCampaignQueueRelPath
	}
	queue, err := LoadInitCampaignQueueFromRuntime(req.Context, req.FileService, queuePath)
	if err != nil {
		return nil, err
	}
	entry, err := queue.markEntry(req)
	if err != nil {
		return nil, err
	}
	err = SaveInitCampaignQueueToRuntime(req.Context, req.FileService, queuePath, queue)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (queue *CampaignQueue) markEntry(req MarkCampaignQueueEntryRequest) (*CampaignQueueModel, error) {
	queryVariant := strings.TrimSpace(req.VariantID)
	queryTag := strings.TrimSpace(req.ServedModelTag)
	if queryVariant == "" && queryTag == "" {
		return nil, fmt.Errorf("evaluation: mark campaign queue entry: specify variant_id or served model tag")
	}
	for i, entry := range queue.Models {
		if queryVariant != "" && entry.VariantID != queryVariant {
			continue
		}
		if queryTag != "" && entry.ServedModelTag != queryTag {
			continue
		}
		queue.Models[i].Status = req.Status
		if req.VerifiedRunID != "" {
			queue.Models[i].VerifiedRunID = req.VerifiedRunID
		}
		if req.Notes != "" {
			queue.Models[i].Notes = req.Notes
		}
		selected := queue.Models[i]
		return &selected, nil
	}
	return nil, fmt.Errorf("evaluation: mark campaign queue entry: model not found")
}

// SaveInitCampaignQueue writes the rollout queue manifest.
func SaveInitCampaignQueue(path string, queue *CampaignQueue) error {
	if queue == nil || len(queue.Models) == 0 {
		return fmt.Errorf("evaluation: save init campaign queue: missing required field")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("evaluation: save init campaign queue: create dir: %w", err)
	}
	body, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: save init campaign queue: encode: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("evaluation: save init campaign queue: %w", err)
	}
	return nil
}

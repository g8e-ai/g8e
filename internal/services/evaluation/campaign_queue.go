// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	// Runtime operator data lives under .g8e/ (gitignored). Checked-in templates: eval/examples/.
	DefaultInitCampaignQueueRelPath    = ".g8e/eval/init-campaign-queue.json"
	DefaultModelInventoryRelPath       = ".g8e/eval/model-inventory.json"
	DefaultCampaignInventoryRelDirname = ".g8e/eval/inventories"

	// Checked-in genesis program inventory.
	DefaultBaseModelInventoryRelPath    = "eval/base-model-inventory.json"
	DefaultBaseInitCampaignQueueRelPath = "eval/base-init-campaign-queue.json"
	DefaultGenesisHomogeneousCampaignID = "eval-genesis-homogeneous"
)

var priorityModelTags = []string{
	"granite4.2:3b",
	"granite4.2:8b",
	"qwen3.5:0.8b",
	"qwen3.5:2b",
	"qwen3.5:4b",
	"qwen3.5:9b",
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

// CampaignStartPlan is the resolved input for one homogeneous campaign start.
type CampaignStartPlan struct {
	CampaignID           string
	RunID                string
	InventoryPath        string
	ModelTags            []string
	RegistryDigest       string
	HomogeneousCellCount uint64
	QueueEntry           *CampaignQueueModel
}

// CampaignStartPlanRequest carries user intent for campaign start resolution.
type CampaignStartPlanRequest struct {
	ProjectRoot   string
	ModelTag      string
	ModelTags     []string
	QueueRef      string
	CampaignID    string
	InventoryFile string
	RunID         string
	Now           time.Time
}

// LoadInitCampaignQueue reads the init-campaign rollout manifest.
func LoadInitCampaignQueue(path string) (*CampaignQueue, error) {
	data, err := os.ReadFile(path)
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

// NextPending returns the first queue entry with status pending.
func (queue *CampaignQueue) NextPending() (*CampaignQueueModel, error) {
	if queue == nil {
		return nil, fmt.Errorf("evaluation: init campaign queue next pending: %w", constants.ErrMissingRequiredField)
	}
	for _, entry := range queue.Models {
		if strings.EqualFold(entry.Status, "pending") {
			selected := entry
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("evaluation: init campaign queue: no pending models")
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
	return nil, fmt.Errorf("evaluation: init campaign queue: model %q not found", query)
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

// ResolveBaseModelInventoryPath returns the checked-in genesis program inventory.
func ResolveBaseModelInventoryPath(projectRoot string) string {
	return ResolveEvalPath(projectRoot, DefaultBaseModelInventoryRelPath)
}

// ResolveModelInventoryPath picks an inventory file for rollout queue initialization.
// When explicitPath is empty, use the checked-in genesis base inventory.
func ResolveModelInventoryPath(projectRoot, explicitPath string) string {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath != "" {
		return ResolveEvalPath(projectRoot, explicitPath)
	}
	return ResolveBaseModelInventoryPath(projectRoot)
}

// ResolveRuntimeModelInventoryPath picks an inventory file for ad-hoc campaign starts.
// When explicitPath is empty, prefer the runtime provider freeze when present,
// otherwise fall back to the checked-in genesis base inventory.
func ResolveRuntimeModelInventoryPath(projectRoot, explicitPath string) string {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath != "" {
		return ResolveEvalPath(projectRoot, explicitPath)
	}
	runtimePath := ResolveEvalPath(projectRoot, DefaultModelInventoryRelPath)
	if _, err := os.Stat(runtimePath); err == nil {
		return runtimePath
	}
	return ResolveBaseModelInventoryPath(projectRoot)
}

// BaseModelInventoryTags returns served model tags from the checked-in base inventory.
func BaseModelInventoryTags(projectRoot string) ([]string, error) {
	variants, err := LoadFrozenVariants(ResolveEvalPath(projectRoot, DefaultBaseModelInventoryRelPath))
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetServedModelTag() == "" {
			continue
		}
		tags = append(tags, variant.GetServedModelTag())
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("evaluation: base model inventory tags: %w", constants.ErrMissingRequiredField)
	}
	return tags, nil
}

// LoadFrozenVariants reads a frozen model inventory export.
func LoadFrozenVariants(path string) ([]*evalv1.ModelVariant, error) {
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

func SortModelVariantsForRollout(variants []*evalv1.ModelVariant) {
	priority := make(map[string]int, len(priorityModelTags))
	for index, tag := range priorityModelTags {
		priority[tag] = index
	}
	sort.SliceStable(variants, func(i, j int) bool {
		leftPriority, leftPrioritized := priority[variants[i].GetServedModelTag()]
		rightPriority, rightPrioritized := priority[variants[j].GetServedModelTag()]
		if leftPrioritized != rightPrioritized {
			return leftPrioritized
		}
		if leftPrioritized {
			return leftPriority < rightPriority
		}
		return variants[i].GetVariantId() < variants[j].GetVariantId()
	})
}

// VariantsByTags returns frozen variants for the requested served model tags.
func VariantsByTags(variants []*evalv1.ModelVariant, tags []string) ([]*evalv1.ModelVariant, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("evaluation: variants by tags: %w", constants.ErrMissingRequiredField)
	}
	picked := make([]*evalv1.ModelVariant, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		found := false
		for _, variant := range variants {
			if variant != nil && variant.GetServedModelTag() == tag {
				picked = append(picked, variant)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("evaluation: variants by tags: %w: %s", constants.ErrInferenceModelNotFound, tag)
		}
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("evaluation: variants by tags: %w", constants.ErrMissingRequiredField)
	}
	return picked, nil
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

// ResolveCampaignStartPlan resolves one homogeneous campaign start plan.
func ResolveCampaignStartPlan(req CampaignStartPlanRequest) (*CampaignStartPlan, error) {
	if req.ProjectRoot == "" {
		return nil, fmt.Errorf("evaluation: resolve campaign start plan: %w", constants.ErrMissingRequiredField)
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	switch strings.TrimSpace(req.QueueRef) {
	case "":
		tags := normalizeModelTags(req.ModelTag, req.ModelTags)
		if len(tags) == 0 {
			return nil, fmt.Errorf("evaluation: resolve campaign start plan: specify --model, --models, or --queue")
		}
		return resolveCampaignStartPlanForTags(req, tags, now)
	case "next":
		queuePath := filepath.Join(req.ProjectRoot, DefaultInitCampaignQueueRelPath)
		queue, err := LoadInitCampaignQueue(queuePath)
		if err != nil {
			return nil, err
		}
		entry, err := queue.NextPending()
		if err != nil {
			return nil, err
		}
		return planFromQueueEntry(req, entry, now)
	default:
		queuePath := filepath.Join(req.ProjectRoot, DefaultInitCampaignQueueRelPath)
		queue, err := LoadInitCampaignQueue(queuePath)
		if err != nil {
			return nil, err
		}
		entry, err := queue.FindByTagOrVariantID(req.QueueRef)
		if err != nil {
			return nil, err
		}
		return planFromQueueEntry(req, entry, now)
	}
}

func resolveCampaignStartPlanForTags(req CampaignStartPlanRequest, tags []string, now time.Time) (*CampaignStartPlan, error) {
	if req.InventoryFile != "" {
		inventoryPath := req.InventoryFile
		if !filepath.IsAbs(inventoryPath) {
			inventoryPath = filepath.Join(req.ProjectRoot, inventoryPath)
		}
		freeze, err := LoadModelInventoryFreezeFile(inventoryPath)
		if err != nil {
			return nil, err
		}
		campaignID := req.CampaignID
		if campaignID == "" {
			campaignID = freeze.CampaignID
		}
		runID := req.RunID
		if runID == "" {
			runID = campaignID + "-" + fmt.Sprintf("%d", now.Unix())
		}
		return &CampaignStartPlan{
			CampaignID:           campaignID,
			RunID:                runID,
			InventoryPath:        inventoryPath,
			ModelTags:            tags,
			RegistryDigest:       freeze.RegistryDigest,
			HomogeneousCellCount: freeze.HomogeneousCellCount,
		}, nil
	}

	queuePath := filepath.Join(req.ProjectRoot, DefaultInitCampaignQueueRelPath)
	if len(tags) == 1 {
		if queue, err := LoadInitCampaignQueue(queuePath); err == nil {
			if entry, err := queue.FindByTagOrVariantID(tags[0]); err == nil {
				return planFromQueueEntry(req, entry, now)
			}
		}
	}

	variants, err := LoadFrozenVariants(ResolveRuntimeModelInventoryPath(req.ProjectRoot, ""))
	if err != nil {
		return nil, err
	}
	picked, err := VariantsByTags(variants, tags)
	if err != nil {
		return nil, err
	}

	campaignID := req.CampaignID
	if campaignID == "" {
		if len(picked) == 1 {
			campaignID = CampaignIDForVariant(picked[0])
		} else {
			campaignID = "eval-batch-" + fmt.Sprintf("%d", now.Unix())
		}
	}
	freeze, err := MaterializeModelRegistry(campaignID, picked)
	if err != nil {
		return nil, err
	}
	if err := ValidateModelRegistry(freeze); err != nil {
		return nil, err
	}
	inventoryDir := filepath.Join(req.ProjectRoot, DefaultCampaignInventoryRelDirname)
	if err := os.MkdirAll(inventoryDir, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("evaluation: resolve campaign start plan: create inventory dir: %w", err)
	}
	inventoryPath := filepath.Join(inventoryDir, campaignID+".json")
	if err := writeModelInventoryFreezeFile(inventoryPath, freeze); err != nil {
		return nil, err
	}
	runID := req.RunID
	if runID == "" {
		runID = campaignID + "-" + fmt.Sprintf("%d", now.Unix())
	}
	return &CampaignStartPlan{
		CampaignID:           campaignID,
		RunID:                runID,
		InventoryPath:        inventoryPath,
		ModelTags:            tags,
		RegistryDigest:       freeze.RegistryDigest,
		HomogeneousCellCount: freeze.HomogeneousCellCount,
	}, nil
}

func planFromQueueEntry(req CampaignStartPlanRequest, entry *CampaignQueueModel, now time.Time) (*CampaignStartPlan, error) {
	if entry == nil {
		return nil, fmt.Errorf("evaluation: resolve campaign start plan: %w", constants.ErrMissingRequiredField)
	}
	inventoryPath := entry.InventoryFile
	if !filepath.IsAbs(inventoryPath) {
		inventoryPath = filepath.Join(req.ProjectRoot, inventoryPath)
	}
	campaignID := req.CampaignID
	if campaignID == "" {
		campaignID = entry.CampaignID
	}
	runID := req.RunID
	if runID == "" {
		runID = campaignID + "-" + fmt.Sprintf("%d", now.Unix())
	}
	selected := *entry
	return &CampaignStartPlan{
		CampaignID:           campaignID,
		RunID:                runID,
		InventoryPath:        inventoryPath,
		ModelTags:            []string{entry.ServedModelTag},
		RegistryDigest:       entry.ModelRegistryDigest,
		HomogeneousCellCount: entry.HomogeneousCellCount,
		QueueEntry:           &selected,
	}, nil
}

func normalizeModelTags(modelTag string, modelTags []string) []string {
	tags := make([]string, 0, len(modelTags)+1)
	if strings.TrimSpace(modelTag) != "" {
		tags = append(tags, strings.TrimSpace(modelTag))
	}
	for _, tag := range modelTags {
		for _, part := range strings.Split(tag, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = append(tags, part)
			}
		}
	}
	return tags
}

func writeModelInventoryFreezeFile(path string, freeze *ModelInventoryFreeze) error {
	if freeze == nil {
		return fmt.Errorf("evaluation: write model inventory freeze file: %w", constants.ErrMissingRequiredField)
	}
	variantBodies := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		body, err := protojson.Marshal(variant)
		if err != nil {
			return fmt.Errorf("evaluation: write model inventory freeze file: marshal variant: %w", err)
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
		return err
	}
	if err := os.WriteFile(path, payload, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: write model inventory freeze file: %w", err)
	}
	return nil
}

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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// Rollout queue entry statuses.
const (
	QueueStatusPending  = "pending"
	QueueStatusVerified = "verified"
	QueueStatusFailed   = "failed"
	QueueStatusSkipped  = "skipped"
)

const rolloutQueueGenerateCommand = "./g8e eval rollout add <selector>"

// LoadRolloutQueue reads the rollout queue. A queue that has not been written
// yet is empty rather than an error.
func LoadRolloutQueue(ctx context.Context, fileSvc fs.RuntimeFileService) (*CampaignQueue, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("evaluation: load rollout queue: %w", constants.ErrMissingRequiredField)
	}
	exists, err := fileSvc.FileExists(ctx, DefaultInitCampaignQueueRelPath)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load rollout queue: %w", err)
	}
	if !exists {
		return BuildInitCampaignQueue("", nil), nil
	}
	return LoadInitCampaignQueueFromRuntime(ctx, fileSvc, DefaultInitCampaignQueueRelPath)
}

// SaveRolloutQueue writes the rollout queue. An empty queue removes the file.
func SaveRolloutQueue(ctx context.Context, fileSvc fs.RuntimeFileService, queue *CampaignQueue) error {
	if fileSvc == nil || queue == nil {
		return fmt.Errorf("evaluation: save rollout queue: %w", constants.ErrMissingRequiredField)
	}
	if len(queue.Models) == 0 {
		if err := fileSvc.Remove(ctx, DefaultInitCampaignQueueRelPath); err != nil && !errors.Is(err, constants.ErrNotFound) {
			return fmt.Errorf("evaluation: save rollout queue: %w", err)
		}
		return nil
	}
	return SaveInitCampaignQueueToRuntime(ctx, fileSvc, DefaultInitCampaignQueueRelPath, queue)
}

// LoadRolloutIntakePriorityIDs reads the rollout ordering from the project
// root. A missing file means no priority. A malformed file returns an error.
func LoadRolloutIntakePriorityIDs(projectRoot string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, DefaultRolloutIntakePriorityRelPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("evaluation: load rollout intake priority: %w", err)
	}
	var priority []string
	if err := json.Unmarshal(data, &priority); err != nil {
		return nil, fmt.Errorf("evaluation: load rollout intake priority: %w", err)
	}
	return priority, nil
}

// NewCampaignQueueModel builds the pending queue entry for one registry model.
func NewCampaignQueueModel(variant *evalv1.ModelVariant) (CampaignQueueModel, error) {
	if variant == nil {
		return CampaignQueueModel{}, fmt.Errorf("evaluation: new campaign queue model: %w", constants.ErrMissingRequiredField)
	}
	campaignID := CampaignIDForVariant(variant)
	freeze, err := MaterializeModelRegistry(campaignID, []*evalv1.ModelVariant{variant})
	if err != nil {
		return CampaignQueueModel{}, err
	}
	return CampaignQueueModel{
		VariantID:            variant.GetVariantId(),
		ServedModelTag:       variant.GetServedModelTag(),
		CampaignID:           campaignID,
		InventoryFile:        campaignInventoryPath(DefaultCampaignInventoryRelDirname, campaignID),
		ModelRegistryDigest:  freeze.RegistryDigest,
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		Status:               QueueStatusPending,
	}, nil
}

// AddVariants appends the variants that are not queued yet, ordered by the
// rollout ordering (priority intake first, then smallest models first), and
// returns the entries it added. Queued entries keep their place and status.
func (queue *CampaignQueue) AddVariants(variants []*evalv1.ModelVariant, priorityIDs []string) ([]CampaignQueueModel, error) {
	if queue == nil {
		return nil, fmt.Errorf("evaluation: rollout queue add: %w", constants.ErrMissingRequiredField)
	}
	queued := make(map[string]struct{}, len(queue.Models))
	for _, entry := range queue.Models {
		queued[entry.VariantID] = struct{}{}
	}
	fresh := make([]*evalv1.ModelVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		if _, ok := queued[variant.GetVariantId()]; ok {
			continue
		}
		queued[variant.GetVariantId()] = struct{}{}
		fresh = append(fresh, variant)
	}
	SortModelVariantsForRollout(fresh)
	fresh = PrioritizeRolloutIntake(fresh, priorityIDs)
	added := make([]CampaignQueueModel, 0, len(fresh))
	for _, variant := range fresh {
		entry, err := NewCampaignQueueModel(variant)
		if err != nil {
			return nil, err
		}
		added = append(added, entry)
	}
	queue.Models = append(queue.Models, added...)
	return added, nil
}

// SelectEntries returns the queue entries whose served tag or variant ID is
// named in ids. An unknown ID is an error.
func (queue *CampaignQueue) SelectEntries(ids []string) ([]CampaignQueueModel, error) {
	selected := make([]CampaignQueueModel, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		entry, err := queue.FindByTagOrVariantID(id)
		if err != nil {
			return nil, fmt.Errorf("evaluation: rollout queue select: %w: %s", constants.ErrInferenceModelNotFound, id)
		}
		if _, dup := seen[entry.VariantID]; dup {
			continue
		}
		seen[entry.VariantID] = struct{}{}
		selected = append(selected, *entry)
	}
	return selected, nil
}

// EntriesForVariants returns the queue entries of the given variants, in queue
// order. Variants that are not queued are ignored.
func (queue *CampaignQueue) EntriesForVariants(variants []*evalv1.ModelVariant) []CampaignQueueModel {
	want := make(map[string]struct{}, len(variants))
	for _, variant := range variants {
		if variant != nil {
			want[variant.GetVariantId()] = struct{}{}
		}
	}
	entries := make([]CampaignQueueModel, 0, len(want))
	for _, entry := range queue.Models {
		if _, ok := want[entry.VariantID]; ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// RemoveEntries drops the entries with the given variant IDs.
func (queue *CampaignQueue) RemoveEntries(variantIDs []string) {
	drop := make(map[string]struct{}, len(variantIDs))
	for _, id := range variantIDs {
		drop[id] = struct{}{}
	}
	kept := queue.Models[:0:0]
	for _, entry := range queue.Models {
		if _, gone := drop[entry.VariantID]; !gone {
			kept = append(kept, entry)
		}
	}
	queue.Models = kept
}

// SetEntryStatus sets the status and note of the entries with the given variant
// IDs. A note replaces the previous note; an empty note clears it.
func (queue *CampaignQueue) SetEntryStatus(variantIDs []string, status, notes string) {
	want := make(map[string]struct{}, len(variantIDs))
	for _, id := range variantIDs {
		want[id] = struct{}{}
	}
	for i, entry := range queue.Models {
		if _, ok := want[entry.VariantID]; !ok {
			continue
		}
		queue.Models[i].Status = status
		queue.Models[i].Notes = strings.TrimSpace(notes)
		if status != QueueStatusVerified {
			queue.Models[i].VerifiedRunID = ""
		}
	}
}

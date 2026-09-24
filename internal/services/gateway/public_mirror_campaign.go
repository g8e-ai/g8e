// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

const campaignMirrorDatasetPrefix = "ds-live-"

func campaignMirrorDatasetID(runID string) string {
	return campaignMirrorDatasetPrefix + runID
}

func mirrorProjectionMatchesCampaignRun(item models.PublicFeedObject, runID string) bool {
	if runID == "" {
		return false
	}
	datasetID := campaignMirrorDatasetID(runID)
	if id, ok := item.StringField("dataset_id"); ok && id == datasetID {
		return true
	}
	if id, ok := item.StringField("run_id"); ok && id == runID {
		return true
	}
	if key, ok := item.StringField("idempotency_key"); ok && strings.HasPrefix(key, runID+":") {
		return true
	}
	if recordBytes, ok := item["record"]; ok {
		var record map[string]json.RawMessage
		if err := json.Unmarshal(recordBytes, &record); err == nil {
			if raw, ok := record["run_id"]; ok {
				var nestedRunID string
				if err := json.Unmarshal(raw, &nestedRunID); err == nil && nestedRunID == runID {
					return true
				}
			}
			if raw, ok := record["dataset_id"]; ok {
				var nestedDatasetID string
				if err := json.Unmarshal(raw, &nestedDatasetID); err == nil && nestedDatasetID == datasetID {
					return true
				}
			}
		}
	}
	return false
}

func mirrorProjectionWithdrawn(state *PublicMirrorSourceState, item models.PublicFeedObject) bool {
	if state == nil || len(state.WithdrawnDatasetIDs) == 0 {
		return false
	}
	if datasetID, ok := item.StringField("dataset_id"); ok && state.WithdrawnDatasetIDs[datasetID] {
		return true
	}
	if runID, ok := item.StringField("run_id"); ok && state.WithdrawnDatasetIDs[campaignMirrorDatasetID(runID)] {
		return true
	}
	for withdrawnDatasetID := range state.WithdrawnDatasetIDs {
		if !strings.HasPrefix(withdrawnDatasetID, campaignMirrorDatasetPrefix) {
			continue
		}
		runID := strings.TrimPrefix(withdrawnDatasetID, campaignMirrorDatasetPrefix)
		if mirrorProjectionMatchesCampaignRun(item, runID) {
			return true
		}
	}
	return false
}

func mirrorRecordWithdrawn(state *PublicMirrorSourceState, record models.PublicFeedRecord) bool {
	if state == nil || len(state.WithdrawnDatasetIDs) == 0 {
		return false
	}
	var item models.PublicFeedObject
	if err := json.Unmarshal([]byte(record.RecordBytes), &item); err != nil {
		return false
	}
	return mirrorProjectionWithdrawn(state, item)
}

// WithdrawCampaignDataset hides one live campaign dataset from anonymous mirror
// reads without mutating the signed batch chain.
func (m *PublicMirrorServer) WithdrawCampaignDataset(ctx context.Context, runID string) error {
	if m == nil || runID == "" {
		return fmt.Errorf("public mirror: withdraw campaign dataset: %w", constants.ErrMissingRequiredField)
	}
	datasetID := campaignMirrorDatasetID(runID)
	return m.mutateState(ctx, func(storeState *PublicMirrorStoreState) error {
		sourceID := activePublicMirrorSourceID(*storeState)
		if sourceID == "" {
			return nil
		}
		state, ok := storeState.Sources[sourceID]
		if !ok || state == nil {
			state = newPublicMirrorSourceState()
			storeState.Sources[sourceID] = state
		}
		if state.WithdrawnDatasetIDs == nil {
			state.WithdrawnDatasetIDs = make(map[string]bool)
		}
		state.WithdrawnDatasetIDs[datasetID] = true
		return nil
	})
}

// CampaignDatasetWithdrawn reports whether one live campaign dataset is hidden
// from anonymous mirror reads.
func (m *PublicMirrorServer) CampaignDatasetWithdrawn(runID string) bool {
	if m == nil || runID == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	sourceID := activePublicMirrorSourceID(m.state)
	state := m.state.Sources[sourceID]
	if state == nil {
		return false
	}
	return state.WithdrawnDatasetIDs[campaignMirrorDatasetID(runID)]
}


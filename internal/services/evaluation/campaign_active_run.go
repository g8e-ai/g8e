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
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// ActiveCampaignRun tracks the most recently started campaign run.
type ActiveCampaignRun struct {
	RunID         string    `json:"run_id"`
	CampaignID    string    `json:"campaign_id"`
	InventoryFile string    `json:"inventory_file"`
	ModelTags     []string  `json:"model_tags"`
	StartedAt     time.Time `json:"started_at"`
}

// LoadActiveCampaignRun reads the active campaign run marker from runtime state.
func LoadActiveCampaignRunFromRuntime(ctx context.Context, fileSvc fs.RuntimeFileService) (*ActiveCampaignRun, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("evaluation: load active campaign run: %w", constants.ErrMissingRequiredField)
	}
	data, err := fileSvc.ReadFile(ctx, constants.EvaluationActiveRunPath)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load active campaign run: %w", err)
	}
	run := &ActiveCampaignRun{}
	if err := json.Unmarshal(data, run); err != nil {
		return nil, fmt.Errorf("evaluation: load active campaign run: decode: %w", err)
	}
	if run.RunID == "" {
		return nil, fmt.Errorf("evaluation: load active campaign run: %w", constants.ErrMissingRequiredField)
	}
	return run, nil
}

// SaveActiveCampaignRun writes the active campaign run marker to runtime state.
func SaveActiveCampaignRunToRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, run ActiveCampaignRun) error {
	if fileSvc == nil || run.RunID == "" {
		return fmt.Errorf("evaluation: save active campaign run: %w", constants.ErrMissingRequiredField)
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: save active campaign run: encode: %w", err)
	}
	if err := fileSvc.WriteFile(ctx, constants.EvaluationActiveRunPath, payload, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: save active campaign run: %w", err)
	}
	return nil
}

// ActiveCampaignRunPath is retained for external fixture compatibility; runtime callers use constants.EvaluationActiveRunPath.
func ActiveCampaignRunPath(projectRoot string) string {
	return filepath.Join(projectRoot, constants.RuntimeDirname, constants.EvaluationActiveRunPath)
}

// LoadActiveCampaignRun reads an explicitly supplied external fixture path.
func LoadActiveCampaignRun(projectRoot string) (*ActiveCampaignRun, error) {
	data, err := os.ReadFile(ActiveCampaignRunPath(projectRoot))
	if err != nil {
		return nil, fmt.Errorf("evaluation: load active campaign run: %w", err)
	}
	run := &ActiveCampaignRun{}
	if err := json.Unmarshal(data, run); err != nil {
		return nil, fmt.Errorf("evaluation: load active campaign run: decode: %w", err)
	}
	return run, nil
}

// SaveActiveCampaignRun writes an explicitly supplied external fixture path.
func SaveActiveCampaignRun(projectRoot string, run ActiveCampaignRun) error {
	path := ActiveCampaignRunPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("evaluation: save active campaign run: create dir: %w", err)
	}
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: save active campaign run: encode: %w", err)
	}
	if err := os.WriteFile(path, payload, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: save active campaign run: %w", err)
	}
	return nil
}

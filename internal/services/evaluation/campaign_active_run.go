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
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const activeCampaignRunFilename = "active-run.json"

// ActiveCampaignRun tracks the most recently started campaign run.
type ActiveCampaignRun struct {
	RunID         string    `json:"run_id"`
	CampaignID    string    `json:"campaign_id"`
	InventoryFile string    `json:"inventory_file"`
	ModelTags     []string  `json:"model_tags"`
	StartedAt     time.Time `json:"started_at"`
}

// ActiveCampaignRunPath returns the active-run file path for one project root.
func ActiveCampaignRunPath(projectRoot string) string {
	return filepath.Join(projectRoot, constants.DataDirname, constants.EvaluationDirname, activeCampaignRunFilename)
}

// LoadActiveCampaignRun reads the active campaign run marker.
func LoadActiveCampaignRun(projectRoot string) (*ActiveCampaignRun, error) {
	path := ActiveCampaignRunPath(projectRoot)
	data, err := os.ReadFile(path)
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

// SaveActiveCampaignRun writes the active campaign run marker.
func SaveActiveCampaignRun(projectRoot string, run ActiveCampaignRun) error {
	if projectRoot == "" || run.RunID == "" {
		return fmt.Errorf("evaluation: save active campaign run: %w", constants.ErrMissingRequiredField)
	}
	dir := filepath.Join(projectRoot, constants.DataDirname, constants.EvaluationDirname)
	if err := os.MkdirAll(dir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("evaluation: save active campaign run: create dir: %w", err)
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: save active campaign run: encode: %w", err)
	}
	path := filepath.Join(dir, activeCampaignRunFilename)
	if err := os.WriteFile(path, payload, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: save active campaign run: %w", err)
	}
	return nil
}

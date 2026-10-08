// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// OperatorDeploymentRecorder persists the Operator's non-secret enrollment and
// startup progress so a deploying CLI discovers it without parsing process
// output. Writes are serialized so a late "ready" cannot be overwritten by an
// earlier phase.
type OperatorDeploymentRecorder struct {
	fileSvc fs.RuntimeFileService
	mu      sync.Mutex
}

func NewOperatorDeploymentRecorder(fileSvc fs.RuntimeFileService) *OperatorDeploymentRecorder {
	return &OperatorDeploymentRecorder{fileSvc: fileSvc}
}

func operatorDeploymentStateRelPath() string {
	return filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator)
}

// Reset removes any record left by a previous launch of this working directory.
func (r *OperatorDeploymentRecorder) Reset(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.fileSvc.Remove(ctx, operatorDeploymentStateRelPath()); err != nil {
		return fmt.Errorf("operator deployment state: reset: %w", err)
	}
	return nil
}

// Record atomically replaces the persisted state.
func (r *OperatorDeploymentRecorder) Record(ctx context.Context, state models.OperatorDeploymentState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("operator deployment state: marshal: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rel := operatorDeploymentStateRelPath()
	if err := r.fileSvc.MkdirAll(ctx, filepath.Dir(rel), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("operator deployment state: create dir: %w", err)
	}
	if err := r.fileSvc.WriteFile(ctx, rel, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator deployment state: write: %w", err)
	}
	return nil
}

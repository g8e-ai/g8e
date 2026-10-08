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
// output. Writes preserve enrollment identity across subscription changes,
// and terminal startup failures take precedence over late readiness callbacks.
type OperatorDeploymentRecorder struct {
	fileSvc  fs.RuntimeFileService
	launchID string
	state    models.OperatorDeploymentState
	mu       sync.Mutex
}

func NewOperatorDeploymentRecorder(fileSvc fs.RuntimeFileService, launchID string) (*OperatorDeploymentRecorder, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("operator deployment state: %w", constants.ErrFileServiceInit)
	}
	return &OperatorDeploymentRecorder{fileSvc: fileSvc, launchID: launchID}, nil
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
	r.state = models.OperatorDeploymentState{}
	return nil
}

// Record atomically replaces the persisted state.
func (r *OperatorDeploymentRecorder) Record(ctx context.Context, state models.OperatorDeploymentState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A startup failure is terminal for this process, including late callbacks.
	if r.state.Phase == models.OperatorDeploymentPhaseFailed {
		return nil
	}
	state.LaunchID = r.launchID
	state.UpdatedAt = time.Now().UTC()
	if state.RequestID == "" {
		state.RequestID = r.state.RequestID
	}
	if state.OperatorSessionID == "" {
		state.OperatorSessionID = r.state.OperatorSessionID
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("operator deployment state: marshal: %w", err)
	}
	rel := operatorDeploymentStateRelPath()
	if err := r.fileSvc.MkdirAll(ctx, filepath.Dir(rel), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("operator deployment state: create dir: %w", err)
	}
	if err := r.fileSvc.WriteFile(ctx, rel, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator deployment state: write: %w", err)
	}
	r.state = state
	return nil
}

// CommandSubscriptionChanged records transport readiness for this process.
// This observation neither grants authority nor replaces Gateway session checks.
func (r *OperatorDeploymentRecorder) CommandSubscriptionChanged(ctx context.Context, sessionID string, connected bool) error {
	phase := models.OperatorDeploymentPhaseEnrolled
	if connected {
		phase = models.OperatorDeploymentPhaseReady
	}
	return r.Record(ctx, models.OperatorDeploymentState{Phase: phase, OperatorSessionID: sessionID})
}

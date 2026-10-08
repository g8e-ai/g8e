// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package operatorcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func operatorDeploymentStateCmd() *cobra.Command {
	return operatorDeploymentStateCmdWithFactory(shared.NewFileSvc)
}

func operatorDeploymentStateCmdWithFactory(factory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use: "deployment-state", Short: "Read local non-secret Operator deployment progress as JSON", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fileSvc, err := factory(dir, slog.Default())
			if err != nil {
				return fmt.Errorf("operator deployment state: %w: %w", constants.ErrFileServiceInit, err)
			}
			state, err := readOperatorDeploymentState(cmd.Context(), fileSvc)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(state)
		},
	}
	cmd.Flags().StringVar(&dir, "working-dir", "", "Operator working directory (default: current directory)")
	return cmd
}

func readOperatorDeploymentState(ctx context.Context, fileSvc fs.RuntimeFileService) (*models.OperatorDeploymentState, error) {
	data, err := fileSvc.ReadFile(ctx, filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator))
	if errors.Is(err, constants.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read operator deployment state: %w", err)
	}
	return decodeOperatorDeploymentState(data)
}

func decodeOperatorDeploymentState(data []byte) (*models.OperatorDeploymentState, error) {
	var state *models.OperatorDeploymentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%w: decode deployment state: %w", constants.ErrOperatorDeployFailed, err)
	}
	if state == nil {
		return nil, nil
	}
	valid := !state.UpdatedAt.IsZero()
	switch state.Phase {
	case models.OperatorDeploymentPhasePendingApproval:
		valid = valid && state.RequestID != ""
	case models.OperatorDeploymentPhaseEnrolled, models.OperatorDeploymentPhaseReady:
		valid = valid && state.OperatorSessionID != ""
	case models.OperatorDeploymentPhaseFailed:
		valid = valid && state.Error != ""
	default:
		valid = false
	}
	if !valid {
		return nil, fmt.Errorf("%w: invalid deployment phase or missing progress fields", constants.ErrOperatorDeployFailed)
	}
	return state, nil
}

func awaitDeploymentState(ctx context.Context, target deployTarget, dir string, phase models.OperatorDeploymentPhase) (*models.OperatorDeploymentState, error) {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployEnrollTimeout)
	defer cancel()
	var result *models.OperatorDeploymentState
	err := pollUntil(ctx, func() (bool, error) {
		state, err := target.readDeploymentState(ctx, dir)
		if err != nil || state == nil {
			return false, err
		}
		if state.Phase == models.OperatorDeploymentPhaseFailed {
			return false, fmt.Errorf("%w: %s", constants.ErrOperatorDeployFailed, state.Error)
		}
		done := state.Phase == phase || state.Phase == models.OperatorDeploymentPhaseReady
		if phase == models.OperatorDeploymentPhasePendingApproval {
			done = true // Enrolled credentials require no new approval.
		}
		if done {
			result = state
		}
		return done, nil
	})
	if err != nil {
		return nil, fmt.Errorf("await deployment phase %s in %s: %w", phase, dir, err)
	}
	return result, nil
}

func awaitDeploymentRequestID(ctx context.Context, target deployTarget, dir string) (string, error) {
	state, err := awaitDeploymentState(ctx, target, dir, models.OperatorDeploymentPhasePendingApproval)
	if err != nil {
		return "", err
	}
	if state.Phase != models.OperatorDeploymentPhasePendingApproval {
		return "", nil
	}
	return state.RequestID, nil
}

func awaitDeploymentSessionID(ctx context.Context, target deployTarget, dir string) (string, error) {
	state, err := awaitDeploymentState(ctx, target, dir, models.OperatorDeploymentPhaseEnrolled)
	if err != nil {
		return "", err
	}
	return state.OperatorSessionID, nil
}

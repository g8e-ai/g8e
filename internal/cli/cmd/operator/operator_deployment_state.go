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
		Hidden: true,
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

// awaitStaged waits for the Gateway to announce the worker's pending request.
// It returns "" for a worker whose retained credentials were already enrolled,
// which becomes ready without a new approval.
func awaitStaged(ctx context.Context, watch *deploymentWatch, target deployTarget, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployEnrollTimeout)
	defer cancel()
	requestID, err := watch.awaitStaged(ctx)
	if err != nil {
		return "", fmt.Errorf("await staged request in %s: %w", dir, explainDeployWait(target, dir, err))
	}
	return requestID, nil
}

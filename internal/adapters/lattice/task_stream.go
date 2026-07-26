// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lattice

import (
	"context"
	"log/slog"

	taskmanagerv1 "github.com/g8e-ai/g8e/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/internal/constants"
)

// subscribeToTasks opens a streaming RPC to TaskManagerAPI and receives
// task assignments. On stream close, it reconnects with backoff.
// On receiving a task, it dispatches to the registered task handler.
func (a *Adapter) subscribeToTasks(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		stream, err := a.taskMgr.ListenAsAgent(ctx, &taskmanagerv1.ListenAsAgentRequest{
			AgentSelector: &taskmanagerv1.ListenAsAgentRequest_EntityIds{
				EntityIds: &taskmanagerv1.EntityIds{
					EntityIds: []string{a.entityID},
				},
			},
		})
		if err != nil {
			a.logger.Error("Lattice: failed to open task stream",
				"error", err,
				"entity_id", a.entityID)
			if ctx.Err() != nil {
				return
			}
			continue
		}

		a.processStream(ctx, stream)

		if ctx.Err() != nil {
			return
		}
	}
}

// processStream reads messages from the task stream and dispatches them.
func (a *Adapter) processStream(ctx context.Context, stream taskmanagerv1.TaskManagerAPI_ListenAsAgentClient) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Warn("Lattice: task stream closed, reconnecting",
				"error", err)
			return
		}

		if resp.GetHeartbeat() != nil {
			continue
		}

		executeReq := resp.GetExecuteRequest()
		if executeReq == nil {
			continue
		}

		task := executeReq.GetTask()
		if task == nil {
			continue
		}

		taskID := task.GetVersion().GetTaskId()
		if !a.isTaskAccepted(task) {
			a.logger.Debug("Lattice: task not in catalog, ignoring",
				"task_id", taskID)
			continue
		}

		if err := a.checkPostureFloor(); err != nil {
			a.logger.Warn("Lattice: task rejected: posture floor violated",
				"task_id", taskID,
				"active_posture", a.postureProvider(),
				"floor", a.config.PostureFloor)
			_ = a.reportTaskStatus(ctx, taskID, "rejected: posture floor violated")
			continue
		}

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.taskHandler(ctx, task); err != nil {
				a.logger.Error("Lattice: task handler failed",
					"task_id", taskID,
					"error", err)
				_ = a.reportTaskStatus(ctx, taskID, "failed")
			}
		}()
	}
}

// isTaskAccepted returns true if the task's specification URL is in the
// configured catalog. If the catalog is empty, all tasks are accepted.
func (a *Adapter) isTaskAccepted(task *taskmanagerv1.Task) bool {
	if len(a.config.TaskCatalog) == 0 {
		return true
	}

	spec := task.GetSpecification()
	if spec == nil {
		return len(a.config.TaskCatalog) == 0
	}
	taskSpecURL := spec.GetTypeUrl()
	for _, catalogURL := range a.config.TaskCatalog {
		if taskSpecURL == catalogURL {
			return true
		}
	}
	return false
}

// checkPostureFloor returns an error if the active posture is below the
// configured floor.
func (a *Adapter) checkPostureFloor() error {
	active := a.postureProvider()
	floor := a.config.PostureFloor
	if floor == "" {
		floor = "consensus"
	}
	if postureRank(active) < postureRank(floor) {
		return constants.ErrLatticePostureFloorViolated
	}
	return nil
}

// postureRank maps posture strings to comparable ranks. Unknown postures
// are treated as the least strict (fail-closed).
func postureRank(p string) int {
	switch p {
	case "doctrine":
		return 0
	case "consensus":
		return 1
	case "notary":
		return 2
	default:
		return 0
	}
}

// reportTaskStatus sends a task status update back to Lattice.
func (a *Adapter) reportTaskStatus(ctx context.Context, taskID string, statusMsg string) error {
	a.logger.Info("Lattice: reporting task status",
		"task_id", taskID,
		"status", statusMsg,
		slog.String("entity_id", a.entityID))
	return nil
}

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

package pubsub

import (
	"context"

	"github.com/g8e-ai/g8e/components/vsa/models"
)

// ResultsPublisher is the transport-agnostic interface for publishing results
// from the VSA Operator back to AI Agent Services (VSE).
// Implemented by PubSubResultsService (VSODB pub/sub via VSOD proxy).
type ResultsPublisher interface {
	PublishExecutionResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error
	PublishCancellationResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error
	PublishFileEditResult(ctx context.Context, result *models.FileEditResult, originalMsg PubSubCommandMessage) error
	PublishFsListResult(ctx context.Context, result *models.FsListResult, originalMsg PubSubCommandMessage) error
	PublishExecutionStatus(ctx context.Context, status *ExecutionStatusUpdate) error
	PublishResult(ctx context.Context, result *models.VSOMessage) error
	PublishHeartbeat(ctx context.Context, heartbeat *models.Heartbeat) error
}

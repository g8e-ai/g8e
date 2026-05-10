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

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
)

// ResultsPublisher is the transport-agnostic interface for publishing results
// from the g8eo Operator back to AI Agent Services (g8ee).
// Implemented by PubSubResultsService (operator pub/sub via g8ed proxy).
type ResultsPublisher interface {
	PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error
	PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error
	PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error
	PublishFsListResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error
	PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error
	PublishExecutionStatus(ctx context.Context, status proto.Message) error
	PublishResult(ctx context.Context, env *commonv1.UniversalEnvelope) error
	PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error
}

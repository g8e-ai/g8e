// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

//go:generate mockery --name ResultsPublisher --output ./mocks --dir .

// ResultsPublisher is the transport-agnostic interface for publishing results
// from the g8eo Operator back to g8e-Compliant Agentic Ensemble (agent).
// Implemented by PubSubResultsService (operator pub/sub via client proxy).
type ResultsPublisher interface {
	PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error
	PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error
	PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error
	PublishFsListResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error
	PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error
	PublishExecutionStatus(ctx context.Context, status proto.Message, originalMsg *PubSubCommandMessage) error
	PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error
	PublishActionReceipt(ctx context.Context, env *commonv1.GovernanceEnvelope, receipt *operatorv1.ActionReceipt) error
}

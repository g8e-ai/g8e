// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// PasskeyOrchestrator encapsulates the cross-cutting business concerns of the
// passkey approval flow: MCP service provision, suspended transaction management,
// SSE event publishing, and WebSocket broadcasting. PasskeyHandler delegates
// business logic here, retaining only HTTP-layer concerns.
type PasskeyOrchestrator struct {
	mcpSvc         MCPServiceProvider
	suspendedStore storage.SuspendedTransactionStore
	sseStore       *SSEEventService
	pubsub         *GatewayWebSocketHandler
	logger         *slog.Logger
}

// NewPasskeyOrchestrator creates a new PasskeyOrchestrator with all dependencies
// wired. SSEStore and PubSub are required: a nil SSE/pubsub dependency in a
// posture that requires passkey ceremonies is a wiring bug, not a no-op
// condition. Returns constants.ErrPasskeySSEDependenciesRequired when either
// is nil.
func NewPasskeyOrchestrator(mcpSvc MCPServiceProvider, suspendedStore storage.SuspendedTransactionStore, sseStore *SSEEventService, pubsub *GatewayWebSocketHandler, logger *slog.Logger) (*PasskeyOrchestrator, error) {
	if sseStore == nil || pubsub == nil {
		return nil, fmt.Errorf("passkey orchestrator: %w", constants.ErrPasskeySSEDependenciesRequired)
	}
	return &PasskeyOrchestrator{
		mcpSvc:         mcpSvc,
		suspendedStore: suspendedStore,
		sseStore:       sseStore,
		pubsub:         pubsub,
		logger:         logger,
	}, nil
}

// GetSuspendedTransaction retrieves a suspended transaction by hash from the MCP gateway.
func (o *PasskeyOrchestrator) GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	return o.mcpSvc.GetSuspendedTransaction(ctx, txHash)
}

// ResumeWithL3Proof re-submits a suspended transaction with an attached L3 proof.
func (o *PasskeyOrchestrator) ResumeWithL3Proof(ctx context.Context, txHash, userID string, proof *commonv1.L3Proof) (*operatorv1.ActionReceipt, error) {
	return o.mcpSvc.ResumeWithL3Proof(ctx, txHash, userID, proof)
}

// ListSuspendedTransactions retrieves all non-expired suspended transactions for a user.
func (o *PasskeyOrchestrator) ListSuspendedTransactions(ctx context.Context, userID string) ([]*models.SuspendedTransaction, error) {
	return o.suspendedStore.ListSuspendedTransactions(ctx, userID)
}

// EmitApprovalCompletedSSE publishes an approval.completed SSE event scoped to
// the specific CLI session that submitted the transaction, so the waiting CLI
// client receives real-time notification without polling.
func (o *PasskeyOrchestrator) EmitApprovalCompletedSSE(userID, cliSessionID, txHash string) {
	if userID == "" || cliSessionID == "" {
		o.logger.Warn("approval: skipping SSE emission due to missing parameters", "user_id", userID, "cli_session_id", cliSessionID, "tx_hash", txHash)
		return
	}

	eventPayload, err := json.Marshal(models.ApprovalCompletedEvent{
		Type:   constants.SSEEventTypeApprovalCompleted,
		UserID: userID,
		TxHash: txHash,
	})
	if err != nil {
		o.logger.Error("approval: failed to marshal SSE event", "error", err)
		return
	}

	envelope := models.SSEPushPayload{
		UserID:       userID,
		CliSessionID: cliSessionID,
		Event:        eventPayload,
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		o.logger.Error("approval: failed to marshal SSE envelope", "error", err)
		return
	}

	rowID, err := o.sseStore.SSEEventsAppend(
		SSERoute{UserID: userID, CLISessionID: cliSessionID},
		constants.SSEEventTypeApprovalCompleted,
		string(envelopeJSON),
		"g8eg",
	)
	if err != nil {
		o.logger.Error("approval: failed to append SSE event", "error", err)
		return
	}

	pubEvent := models.SSEPublishedEvent{ID: rowID, Payload: json.RawMessage(envelopeJSON)}
	pubJSON, err := json.Marshal(pubEvent)
	if err != nil {
		o.logger.Error("approval: failed to marshal published event", "error", err)
		return
	}
	o.pubsub.Publish("sse:cli:"+cliSessionID, pubJSON)
}

// EmitPasskeyRegisteredSSE publishes a passkey.registered SSE event scoped to the
// CLI session so the waiting CLI client receives real-time notification. Uses the
// models.SSEPushPayload wire format for compatibility with the SSE stream handler.
func (o *PasskeyOrchestrator) EmitPasskeyRegisteredSSE(userID, cliSessionID string) {
	eventPayload, err := json.Marshal(passkeyRegisteredEvent{
		Type:         "passkey.registered",
		UserID:       userID,
		CLISessionID: cliSessionID,
	})
	if err != nil {
		o.logger.Error("passkey: failed to marshal SSE event", string(constants.ConnectionStateError), err)
		return
	}

	envelope := models.SSEPushPayload{
		CliSessionID: cliSessionID,
		UserID:       userID,
		Event:        eventPayload,
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		o.logger.Error("passkey: failed to marshal SSE envelope", string(constants.ConnectionStateError), err)
		return
	}

	// R15/R1: persist and stamp the row ID into the SSEPublishedEvent envelope.
	rowID, err := o.sseStore.SSEEventsAppend(
		SSERoute{UserID: userID, CLISessionID: cliSessionID},
		"passkey.registered",
		string(envelopeJSON),
		"g8eg",
	)
	if err != nil {
		o.logger.Error("passkey: failed to append SSE event", string(constants.ConnectionStateError), err)
		return
	}

	pubEvent := models.SSEPublishedEvent{ID: rowID, Payload: json.RawMessage(envelopeJSON)}
	pubJSON, err := json.Marshal(pubEvent)
	if err != nil {
		o.logger.Error("passkey: failed to marshal published event", string(constants.ConnectionStateError), err)
		return
	}
	o.pubsub.Publish("sse:cli:"+cliSessionID, pubJSON)
}

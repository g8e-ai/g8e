// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// OllamaMaintenanceService owns typed governed Ollama provider inventory and
// residency queries on the Inference Operator.
type OllamaMaintenanceService struct {
	config     *config.Config
	logger     *slog.Logger
	client     PubSubClient
	auditStore AuditEventRecorder
	scrubbing  *scrubbing.ScrubbingService
}

// NewOllamaMaintenanceService creates a new OllamaMaintenanceService.
func NewOllamaMaintenanceService(cfg *config.Config, logger *slog.Logger, client PubSubClient) *OllamaMaintenanceService {
	return &OllamaMaintenanceService{
		config: cfg,
		logger: logger,
		client: client,
	}
}

// SetAuditStore sets the audit store for observed-state content evidence.
func (s *OllamaMaintenanceService) SetAuditStore(auditStore AuditEventRecorder) {
	s.auditStore = auditStore
}

// SetScrubbingService sets the scrubbing service for observed-state content evidence.
func (s *OllamaMaintenanceService) SetScrubbingService(scrubbingSvc *scrubbing.ScrubbingService) {
	s.scrubbing = scrubbingSvc
}

// HandleInventoryRequest lists installed provider models and returns a typed
// OllamaModelInventoryResult without routing through EXECUTE_BASH stdout.
func (s *OllamaMaintenanceService) HandleInventoryRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var request operatorv1.OllamaModelInventoryRequested
	if err := proto.Unmarshal(msg.Payload, &request); err != nil {
		s.logger.Error("Failed to decode Ollama inventory payload", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelInventory.Failed, "invalid request payload", s.auditStore, s.scrubbing)
		return
	}

	executionID := request.GetExecutionId()
	if executionID == "" {
		executionID = executionIDFromMessage(msg)
	}

	backend, err := s.ollamaBackend()
	if err != nil {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelInventory.Failed, err.Error(), s.auditStore, s.scrubbing)
		return
	}

	entries, err := backend.ListProviderModelInventory(ctx)
	if err != nil {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelInventory.Failed, err.Error(), s.auditStore, s.scrubbing)
		return
	}

	payload := &operatorv1.OllamaModelInventoryResult{
		ExecutionId: executionID,
		Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		Entries:     inference.ProviderModelInventoryEntriesToProto(entries),
	}
	publishLFAATypedResponseTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelInventory.Completed, payload, s.auditStore, s.scrubbing)
}

// HandleResidencyRequest reads typed /api/ps residency and returns a typed
// OllamaModelResidencyResult without routing through EXECUTE_BASH stdout.
func (s *OllamaMaintenanceService) HandleResidencyRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var request operatorv1.OllamaModelResidencyRequested
	if err := proto.Unmarshal(msg.Payload, &request); err != nil {
		s.logger.Error("Failed to decode Ollama residency payload", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Failed, "invalid request payload", s.auditStore, s.scrubbing)
		return
	}

	executionID := request.GetExecutionId()
	if executionID == "" {
		executionID = executionIDFromMessage(msg)
	}

	endpoint := s.ollamaEndpoint()
	if endpoint == "" {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Failed, constants.ErrInferenceEndpointInvalid.Error(), s.auditStore, s.scrubbing)
		return
	}

	residency, err := inference.ReadProviderResidency(ctx, inference.ProviderResidencyOptions{Endpoint: endpoint})
	if err != nil {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Failed, err.Error(), s.auditStore, s.scrubbing)
		return
	}

	payload := inference.ProviderResidencyToProto(residency)
	payload.ExecutionId = executionID
	payload.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	publishLFAATypedResponseTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Completed, payload, s.auditStore, s.scrubbing)
}

func (s *OllamaMaintenanceService) ollamaEndpoint() string {
	if s == nil || s.config == nil {
		return ""
	}
	return s.config.Inference.OllamaEndpoint
}

func (s *OllamaMaintenanceService) ollamaBackend() (*inference.OllamaBackend, error) {
	endpoint := s.ollamaEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("inference: Ollama endpoint: %w", constants.ErrInferenceEndpointInvalid)
	}
	backend, err := inference.NewOllamaBackend(endpoint, s.logger)
	if err != nil {
		return nil, err
	}
	return backend, nil
}

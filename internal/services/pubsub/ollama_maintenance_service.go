// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
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
		ExecutionId:  executionID,
		Status:       operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		Entries:      inference.ProviderModelInventoryEntriesToProto(entries),
		RoleBindings: s.inferenceRoleBindings(),
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

	backend, err := s.ollamaBackend()
	if err != nil {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Failed, err.Error(), s.auditStore, s.scrubbing)
		return
	}

	residency, err := backend.ReadResidency(ctx)
	if err != nil {
		publishLFAAErrorTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Failed, err.Error(), s.auditStore, s.scrubbing)
		return
	}

	payload := inference.ProviderResidencyToProto(residency)
	payload.ExecutionId = executionID
	payload.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	publishLFAATypedResponseTo(ctx, s.client, s.config, s.logger, msg, constants.Event.Operator.OllamaModelResidency.Completed, payload, s.auditStore, s.scrubbing)
}

// inferenceRoleBindings reports the models this Operator serves for each
// chat-tier role. The Operator is the sole authority for these bindings;
// callers learn them here and never choose them. Roles with no configured
// model are omitted.
func (s *OllamaMaintenanceService) inferenceRoleBindings() []*operatorv1.InferenceRoleBinding {
	cfg := s.config.Inference
	candidates := []struct {
		role  operatorv1.ModelRole
		model string
	}{
		{operatorv1.ModelRole_MODEL_ROLE_PRIMARY, cfg.PrimaryModel},
		{operatorv1.ModelRole_MODEL_ROLE_ASSISTANT, cfg.AssistantModel},
		{operatorv1.ModelRole_MODEL_ROLE_LITE, cfg.LiteModel},
	}
	bindings := make([]*operatorv1.InferenceRoleBinding, 0, len(candidates))
	for _, c := range candidates {
		if c.model == "" {
			continue
		}
		bindings = append(bindings, &operatorv1.InferenceRoleBinding{Role: c.role, ServedModelTag: c.model})
	}
	return bindings
}

func (s *OllamaMaintenanceService) ollamaBackend() (*inference.OllamaBackend, error) {
	return inference.NewOllamaBackend(s.config.Inference.OllamaEndpoint, s.logger)
}

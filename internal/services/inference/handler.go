// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

// InferenceExecutionHandler implements governance.ExecutionHandler. It
// decodes the governed InferenceRequestPayload from the protobuf
// InferenceRequested message, scrubs the prompt through the existing
// scrubbing.ScrubbingService, calls Backend.Generate, and returns the
// summary string the L5 actuator records in the receipt. The handler
// receives only scrubbed, tokenized prompts; it never receives raw vault
// material.
type InferenceExecutionHandler struct {
	backend   Backend
	cfg       *config.Config
	scrubbing *scrubbing.ScrubbingService
	logger    *slog.Logger
}

// NewInferenceExecutionHandler constructs an InferenceExecutionHandler with
// the given backend, config, scrubbing service, and logger.
func NewInferenceExecutionHandler(backend Backend, cfg *config.Config, scrubbingSvc *scrubbing.ScrubbingService, logger *slog.Logger) *InferenceExecutionHandler {
	return &InferenceExecutionHandler{
		backend:   backend,
		cfg:       cfg,
		scrubbing: scrubbingSvc,
		logger:    logger,
	}
}

// ExecuteVerifiedTransaction implements governance.ExecutionHandler. It is
// called by the L5 actuator after L1–L4 verification passes. It delegates to
// ExecuteInference and returns the canonical result digest as the receipt
// summary so the signed ActionReceipt binds the complete InferenceResult
// (see operator.proto InferenceResult.result_digest).
func (h *InferenceExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
	resp, err := h.ExecuteInference(ctx, cmdMsg)
	if err != nil {
		return "", err
	}
	digest, err := models.ComputeInferenceResultDigest(resp.ToProtoInferenceResult())
	if err != nil {
		return "", err
	}
	return digest, nil
}

// ExecuteInference decodes the protobuf InferenceRequested payload, scrubs the
// prompt through the existing scrubbing.ScrubbingService, resolves the
// default model for the role, calls Backend.Generate, and returns the full
// GenerateResponse. The caller (handleInferenceRequestSync) constructs and
// publishes the InferenceResult proto from this response. The handler
// receives only scrubbed, tokenized prompts; it never receives raw vault
// material.
func (h *InferenceExecutionHandler) ExecuteInference(ctx context.Context, cmdMsg governance.CommandMessage) (*models.GenerateResponse, error) {
	if h.backend == nil {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceBackendNotRegistered)
	}

	payloadBytes := cmdMsg.GetPayload()
	if len(payloadBytes) == 0 {
		return nil, fmt.Errorf("inference handler: empty payload: %w", constants.ErrPubSubEmptyPayload)
	}

	req := &operatorv1.InferenceRequested{}
	if err := proto.Unmarshal(payloadBytes, req); err != nil {
		return nil, fmt.Errorf("inference handler: unmarshal payload: %w", err)
	}

	infReq := models.FromProtoInferenceRequested(req)

	// Scrub the prompt before it crosses the execution boundary. The
	// handler receives only scrubbed, tokenized prompts; it never receives
	// raw vault material. This matches the existing scrubbing contract for
	// every other governed execution path.
	if h.scrubbing != nil && h.scrubbing.IsEnabled() {
		infReq.Prompt = h.scrubbing.ScrubText(infReq.Prompt)
	}

	// Resolve the approved model for the role. The configured role-to-model
	// mapping is the active typed authority: a request model is accepted
	// only when it names the configured model for the requested role; any
	// other value is an unauthorized override.
	if infReq.Role == models.InferenceModelRoleUnspecified {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceRoleInvalid)
	}
	approved := h.defaultModelForRole(infReq.Role)
	if approved == "" {
		return nil, fmt.Errorf("inference handler: %w: role %d", constants.ErrInferenceModelRefInvalid, infReq.Role)
	}
	if infReq.Model != "" && infReq.Model != approved {
		return nil, fmt.Errorf("inference handler: %w: role %d", constants.ErrInferenceModelOverrideDenied, infReq.Role)
	}
	genReq := infReq.ToGenerateRequest(approved)
	// Apply config default keep-alive when the request does not override it.
	if genReq.KeepAlive == "" {
		genReq.KeepAlive = h.cfg.Inference.KeepAlive
	}

	h.logger.Info("Dispatching governed inference request",
		"role", infReq.Role,
		"model", genReq.Model,
		"prompt_length", len(genReq.Prompt))

	resp, err := h.backend.Generate(ctx, genReq)
	if err != nil {
		return nil, fmt.Errorf("inference handler: %w", err)
	}

	return resp, nil
}

// defaultModelForRole returns the configured default Ollama model name for
// the given chat-tier role.
func (h *InferenceExecutionHandler) defaultModelForRole(role models.InferenceModelRole) string {
	switch role {
	case models.InferenceModelRolePrimary:
		return h.cfg.Inference.PrimaryModel
	case models.InferenceModelRoleAssistant:
		return h.cfg.Inference.AssistantModel
	case models.InferenceModelRoleLite:
		return h.cfg.Inference.LiteModel
	default:
		return ""
	}
}

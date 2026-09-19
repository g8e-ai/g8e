// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// AttestationResultPublisher publishes completed model provenance attestation
// windows to the provenance operator results channel.
type AttestationResultPublisher interface {
	PublishModelProvenanceObservationCompleted(ctx context.Context, originalMsgID string, completion *evalv1.ModelProvenanceObservationCompleted) error
}

// Handler processes pubsub provenance commands on the storage-side operator.
type Handler struct {
	tracker   *Tracker
	publisher AttestationResultPublisher
	logger    *slog.Logger
}

// NewHandler constructs a model provenance pubsub handler.
func NewHandler(tracker *Tracker, publisher AttestationResultPublisher, logger *slog.Logger) (*Handler, error) {
	if tracker == nil || publisher == nil || logger == nil {
		return nil, fmt.Errorf("model provenance handler: %w", constants.ErrMissingRequiredField)
	}
	return &Handler{tracker: tracker, publisher: publisher, logger: logger}, nil
}

// HandleCommand processes one ModelProvenanceObservationCommand delivered on
// the provenance operator cmd channel.
func (h *Handler) HandleCommand(ctx context.Context, msgID string, payload []byte) (string, error) {
	command := &evalv1.ModelProvenanceObservationCommand{}
	if err := proto.Unmarshal(payload, command); err != nil {
		return "", fmt.Errorf("model provenance handler: unmarshal command: %w", err)
	}
	switch command.GetPhase() {
	case evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN:
		if err := h.tracker.Begin(ctx, command); err != nil {
			return "", err
		}
		h.logger.Info("Model provenance observation started",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"served_model_tag", command.GetServedModelTag(),
			"expected_model_digest", command.GetExpectedModelDigest())
		return command.GetProviderAttemptId(), nil
	case evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE:
		window, err := h.tracker.Finalize(ctx, command)
		if err != nil {
			return "", err
		}
		completion := &evalv1.ModelProvenanceObservationCompleted{Window: window}
		if err := h.publisher.PublishModelProvenanceObservationCompleted(ctx, msgID, completion); err != nil {
			return "", fmt.Errorf("model provenance handler: publish completion: %w", err)
		}
		h.logger.Info("Model provenance observation completed",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"served_model_tag", window.GetServedModelTag(),
			"observed_model_digest", window.GetObservedModelDigest(),
			"attestation_digest", window.GetAttestationDigest())
		return window.GetAttestationDigest(), nil
	default:
		return "", fmt.Errorf("model provenance handler: unsupported phase")
	}
}

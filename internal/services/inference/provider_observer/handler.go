// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ObservationResultPublisher publishes completed provider-boundary observation
// windows to the observer operator results channel.
type ObservationResultPublisher interface {
	PublishProviderBoundaryObservationCompleted(ctx context.Context, originalMsgID string, completion *evalv1.ProviderBoundaryObservationCompleted) error
}

// Handler processes pubsub observation commands on the remote provider host.
type Handler struct {
	tracker   *Tracker
	publisher ObservationResultPublisher
	logger    *slog.Logger
}

// NewHandler constructs a provider-boundary observation pubsub handler.
func NewHandler(tracker *Tracker, publisher ObservationResultPublisher, logger *slog.Logger) (*Handler, error) {
	if tracker == nil || publisher == nil || logger == nil {
		return nil, fmt.Errorf("provider observer handler: %w", constants.ErrMissingRequiredField)
	}
	return &Handler{tracker: tracker, publisher: publisher, logger: logger}, nil
}

// HandleCommand processes one ProviderBoundaryObservationCommand delivered on
// the observer operator cmd channel.
func (h *Handler) HandleCommand(ctx context.Context, msgID string, payload []byte) (string, error) {
	command := &evalv1.ProviderBoundaryObservationCommand{}
	if err := proto.Unmarshal(payload, command); err != nil {
		return "", fmt.Errorf("provider observer handler: unmarshal command: %w", err)
	}
	switch command.GetPhase() {
	case evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN:
		if err := h.tracker.Begin(ctx, command); err != nil {
			return "", err
		}
		h.logger.Info("Provider-boundary observation started",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"inference_transaction_id", command.GetInferenceTransactionId())
		return command.GetProviderAttemptId(), nil
	case evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE:
		window, err := h.tracker.Finalize(ctx, command)
		if err != nil {
			return "", err
		}
		completion := &evalv1.ProviderBoundaryObservationCompleted{Window: window}
		if err := h.publisher.PublishProviderBoundaryObservationCompleted(ctx, msgID, completion); err != nil {
			return "", fmt.Errorf("provider observer handler: publish completion: %w", err)
		}
		h.logger.Info("Provider-boundary observation completed",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"sample_count", len(window.GetSamples()),
			"observation_digest", window.GetObservationDigest())
		return window.GetObservationDigest(), nil
	default:
		return "", fmt.Errorf("provider observer handler: unsupported phase")
	}
}

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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const SchemaVersion = "1.0.0"

// TrackerConfig configures the pubsub-command-driven model provenance tracker.
type TrackerConfig struct {
	OperatorID string
	Attestor   Attestor
	Now        func() time.Time
}

// Tracker maintains in-memory active attestation contexts keyed by
// provider_attempt_id between BEGIN and FINALIZE commands.
type Tracker struct {
	cfg    TrackerConfig
	mu     sync.Mutex
	active map[string]*trackedAttestation
}

type trackedAttestation struct {
	command *evalv1.ModelProvenanceObservationCommand
}

// NewTracker constructs a command-driven model provenance tracker.
func NewTracker(cfg TrackerConfig) (*Tracker, error) {
	if cfg.Attestor == nil {
		return nil, fmt.Errorf("model provenance tracker: %w", constants.ErrMissingRequiredField)
	}
	if cfg.OperatorID == "" {
		cfg.OperatorID = "g8e-model-provenance-operator"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Tracker{cfg: cfg, active: make(map[string]*trackedAttestation)}, nil
}

// Begin records the expected model binding for one provider attempt.
func (t *Tracker) Begin(_ context.Context, command *evalv1.ModelProvenanceObservationCommand) error {
	if command == nil || command.GetProviderAttemptId() == "" {
		return fmt.Errorf("model provenance tracker: begin: %w", constants.ErrMissingRequiredField)
	}
	if command.GetPhase() != evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN {
		return fmt.Errorf("model provenance tracker: begin: invalid phase")
	}
	if command.GetServedModelTag() == "" || command.GetExpectedModelDigest() == "" {
		return fmt.Errorf("model provenance tracker: begin: missing model binding")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.active[command.GetProviderAttemptId()] = &trackedAttestation{command: command}
	return nil
}

// Finalize attests model weights and returns the completed provenance window.
func (t *Tracker) Finalize(ctx context.Context, command *evalv1.ModelProvenanceObservationCommand) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if command == nil || command.GetProviderAttemptId() == "" {
		return nil, fmt.Errorf("model provenance tracker: finalize: %w", constants.ErrMissingRequiredField)
	}
	if command.GetPhase() != evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE {
		return nil, fmt.Errorf("model provenance tracker: finalize: invalid phase")
	}

	t.mu.Lock()
	tracked := t.active[command.GetProviderAttemptId()]
	if tracked != nil {
		delete(t.active, command.GetProviderAttemptId())
	}
	t.mu.Unlock()

	begin := tracked
	if begin == nil {
		begin = &trackedAttestation{command: command}
	}
	servedModelTag := begin.command.GetServedModelTag()
	expectedDigest := begin.command.GetExpectedModelDigest()
	if servedModelTag == "" {
		servedModelTag = command.GetServedModelTag()
	}
	if expectedDigest == "" {
		expectedDigest = command.GetExpectedModelDigest()
	}
	if servedModelTag == "" || expectedDigest == "" {
		return nil, fmt.Errorf("model provenance tracker: finalize: missing model binding")
	}

	window, err := t.cfg.Attestor.Attest(ctx, servedModelTag, expectedDigest, t.cfg.Now().UTC())
	if err != nil {
		return nil, err
	}
	window.ProviderAttemptId = command.GetProviderAttemptId()
	window.ProvenanceOperatorId = t.cfg.OperatorID
	window.AttemptStartedAtUnixMs = command.GetAttemptStartedAtUnixMs()
	window.AttemptCompletedAtUnixMs = command.GetAttemptCompletedAtUnixMs()
	if window.AttemptStartedAtUnixMs == 0 && begin.command != nil {
		window.AttemptStartedAtUnixMs = begin.command.GetAttemptStartedAtUnixMs()
	}
	if !window.DigestMatch {
		return nil, fmt.Errorf("model provenance tracker: finalize: %w", constants.ErrModelProvenanceDigestMismatch)
	}
	digest, err := ComputeAttestationDigest(window)
	if err != nil {
		return nil, err
	}
	window.AttestationDigest = digest
	return window, nil
}

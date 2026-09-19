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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// TrackerConfig configures the pubsub-command-driven provider-boundary
// observation tracker.
type TrackerConfig struct {
	ObserverID     string
	Collector      HardwareCollector
	SampleInterval time.Duration
	Now            func() time.Time
}

// Tracker maintains in-memory active observation windows keyed by
// provider_attempt_id and samples hardware between BEGIN and FINALIZE
// commands received over pubsub.
type Tracker struct {
	cfg    TrackerConfig
	mu     sync.Mutex
	active map[string]*trackedObservation
}

type trackedObservation struct {
	command    *evalv1.ProviderBoundaryObservationCommand
	samples    []*evalv1.ProviderBoundaryHardwareSample
	lastSample time.Time
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// NewTracker constructs a command-driven provider-boundary tracker.
func NewTracker(cfg TrackerConfig) (*Tracker, error) {
	if cfg.Collector == nil {
		return nil, fmt.Errorf("provider observer tracker: %w", constants.ErrMissingRequiredField)
	}
	if cfg.ObserverID == "" {
		cfg.ObserverID = "g8e-provider-boundary-observer"
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 250 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Tracker{cfg: cfg, active: make(map[string]*trackedObservation)}, nil
}

// Begin starts sampling for one provider attempt.
func (t *Tracker) Begin(ctx context.Context, command *evalv1.ProviderBoundaryObservationCommand) error {
	if command == nil || command.GetProviderAttemptId() == "" {
		return fmt.Errorf("provider observer tracker: begin: %w", constants.ErrMissingRequiredField)
	}
	if command.GetPhase() != evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN {
		return fmt.Errorf("provider observer tracker: begin: invalid phase")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if existing := t.active[command.GetProviderAttemptId()]; existing != nil {
		close(existing.stopCh)
		<-existing.doneCh
	}

	tracked := &trackedObservation{
		command: command,
		samples: make([]*evalv1.ProviderBoundaryHardwareSample, 0, 8),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	t.active[command.GetProviderAttemptId()] = tracked
	go t.sampleLoop(ctx, tracked)
	return nil
}

// Finalize stops sampling and returns the completed observation window.
func (t *Tracker) Finalize(ctx context.Context, command *evalv1.ProviderBoundaryObservationCommand) (*evalv1.ProviderBoundaryObservationWindow, error) {
	if command == nil || command.GetProviderAttemptId() == "" {
		return nil, fmt.Errorf("provider observer tracker: finalize: %w", constants.ErrMissingRequiredField)
	}
	if command.GetPhase() != evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE {
		return nil, fmt.Errorf("provider observer tracker: finalize: invalid phase")
	}

	t.mu.Lock()
	tracked := t.active[command.GetProviderAttemptId()]
	if tracked != nil {
		close(tracked.stopCh)
		<-tracked.doneCh
		delete(t.active, command.GetProviderAttemptId())
	}
	t.mu.Unlock()

	if tracked == nil {
		tracked = &trackedObservation{
			command: command,
			samples: nil,
		}
	}
	if len(tracked.samples) == 0 {
		sample, err := t.cfg.Collector.Collect(ctx, t.cfg.Now().UTC())
		if err != nil {
			return nil, fmt.Errorf("provider observer tracker: collect terminal sample: %w", err)
		}
		tracked.samples = []*evalv1.ProviderBoundaryHardwareSample{sample}
	}

	windowStarted := tracked.samples[0].GetObservedAtUnixNanos()
	windowCompleted := tracked.samples[len(tracked.samples)-1].GetObservedAtUnixNanos()
	if windowCompleted <= windowStarted {
		if command.GetAttemptCompletedAtUnixMs() > 0 {
			windowCompleted = uint64(command.GetAttemptCompletedAtUnixMs()) * uint64(time.Millisecond)
		} else {
			windowCompleted = uint64(t.cfg.Now().UTC().UnixNano())
		}
	}
	if windowCompleted < windowStarted {
		windowCompleted = windowStarted + 1
	}

	attemptStartedMs := command.GetAttemptStartedAtUnixMs()
	if attemptStartedMs == 0 && tracked.command != nil {
		attemptStartedMs = tracked.command.GetAttemptStartedAtUnixMs()
	}
	clockSkew := int64(windowStarted) - attemptStartedMs*int64(time.Millisecond)

	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              SchemaVersion,
		ProviderAttemptId:          command.GetProviderAttemptId(),
		ObserverId:                 t.cfg.ObserverID,
		ObserverClockSource:        DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   windowStarted,
		WindowCompletedAtUnixNanos: windowCompleted,
		AttemptStartedAtUnixMs:     attemptStartedMs,
		AttemptCompletedAtUnixMs:   command.GetAttemptCompletedAtUnixMs(),
		ClockSkewNanos:             clockSkew,
		Samples:                    tracked.samples,
	}
	digest, err := ComputeObservationDigest(window)
	if err != nil {
		return nil, err
	}
	window.ObservationDigest = digest
	return window, nil
}

func (t *Tracker) sampleLoop(ctx context.Context, tracked *trackedObservation) {
	defer close(tracked.doneCh)

	if err := t.collectSample(ctx, tracked); err != nil {
		return
	}
	ticker := time.NewTicker(t.cfg.SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tracked.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.collectSample(ctx, tracked); err != nil {
				return
			}
		}
	}
}

func (t *Tracker) collectSample(ctx context.Context, tracked *trackedObservation) error {
	if !tracked.lastSample.IsZero() && t.cfg.Now().Sub(tracked.lastSample) < t.cfg.SampleInterval {
		return nil
	}
	sample, err := t.cfg.Collector.Collect(ctx, t.cfg.Now().UTC())
	if err != nil {
		return err
	}
	tracked.samples = append(tracked.samples, sample)
	tracked.lastSample = t.cfg.Now().UTC()
	return nil
}

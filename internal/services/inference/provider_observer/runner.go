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
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// RunnerConfig configures the read-only provider-boundary observer loop.
type RunnerConfig struct {
	ObserverID      string
	Collector       HardwareCollector
	WindowStore     WindowStore
	FileSvc         fs.RuntimeFileService
	SampleInterval  time.Duration
	PollInterval    time.Duration
	Now             func() time.Time
}

// Runner watches inference provider-attempt records and records hardware
// samples for each in-progress attempt.
type Runner struct {
	cfg     RunnerConfig
	tracked map[string]*activeObservation
}

type activeObservation struct {
	record     *operatorv1.InferenceProviderAttemptRecord
	lastSample time.Time
	samples    []*evalv1.ProviderBoundaryHardwareSample
}

// NewRunner constructs a provider-boundary observer runner.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.Collector == nil || cfg.WindowStore == nil || cfg.FileSvc == nil {
		return nil, fmt.Errorf("provider observer runner: %w", constants.ErrMissingRequiredField)
	}
	if cfg.ObserverID == "" {
		cfg.ObserverID = "g8e-provider-boundary-observer"
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 250 * time.Millisecond
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 250 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Runner{cfg: cfg, tracked: make(map[string]*activeObservation)}, nil
}

// Run executes the observer loop until the context is canceled.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := r.poll(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) poll(ctx context.Context) error {
	attempts, err := r.listAttempts(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(attempts))
	for _, attempt := range attempts {
		seen[attempt.GetProviderAttemptId()] = struct{}{}
		switch attempt.GetStatus() {
		case operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS:
			if err := r.trackInProgress(ctx, attempt); err != nil {
				return err
			}
		case operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
			operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_FAILED:
			if err := r.finalizeAttempt(ctx, attempt); err != nil {
				return err
			}
		}
	}
	for attemptID, active := range r.tracked {
		if _, exists := seen[attemptID]; exists {
			continue
		}
		if err := r.finalizeAttempt(ctx, active.record); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) trackInProgress(ctx context.Context, attempt *operatorv1.InferenceProviderAttemptRecord) error {
	active := r.tracked[attempt.GetProviderAttemptId()]
	if active == nil {
		active = &activeObservation{
			record:  attempt,
			samples: make([]*evalv1.ProviderBoundaryHardwareSample, 0, 8),
		}
		r.tracked[attempt.GetProviderAttemptId()] = active
	}
	if !active.lastSample.IsZero() && r.cfg.Now().Sub(active.lastSample) < r.cfg.SampleInterval {
		return nil
	}
	sample, err := r.cfg.Collector.Collect(ctx, r.cfg.Now().UTC())
	if err != nil {
		return fmt.Errorf("provider observer runner: collect sample: %w", err)
	}
	active.samples = append(active.samples, sample)
	active.lastSample = r.cfg.Now().UTC()
	return nil
}

func (r *Runner) finalizeAttempt(ctx context.Context, attempt *operatorv1.InferenceProviderAttemptRecord) error {
	active := r.tracked[attempt.GetProviderAttemptId()]
	if active == nil {
		active = &activeObservation{record: attempt, samples: nil}
	}
	if len(active.samples) == 0 {
		sample, err := r.cfg.Collector.Collect(ctx, r.cfg.Now().UTC())
		if err != nil {
			return fmt.Errorf("provider observer runner: collect terminal sample: %w", err)
		}
		active.samples = []*evalv1.ProviderBoundaryHardwareSample{sample}
	}
	windowStarted := active.samples[0].GetObservedAtUnixNanos()
	windowCompleted := active.samples[len(active.samples)-1].GetObservedAtUnixNanos()
	if windowCompleted <= windowStarted {
		if attempt.GetCompletedAtUnixMs() > 0 {
			windowCompleted = uint64(attempt.GetCompletedAtUnixMs()) * uint64(time.Millisecond)
		} else {
			windowCompleted = uint64(r.cfg.Now().UTC().UnixNano())
		}
	}
	if windowCompleted < windowStarted {
		windowCompleted = windowStarted + 1
	}
	attemptStartedNanos := attempt.GetStartedAtUnixMs() * int64(time.Millisecond)
	clockSkew := int64(windowStarted) - attemptStartedNanos
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:               SchemaVersion,
		ProviderAttemptId:           attempt.GetProviderAttemptId(),
		ObserverId:                  r.cfg.ObserverID,
		ObserverClockSource:         DefaultObserverClockSource,
		WindowStartedAtUnixNanos:    windowStarted,
		WindowCompletedAtUnixNanos:  windowCompleted,
		AttemptStartedAtUnixMs:      attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:    attempt.GetCompletedAtUnixMs(),
		ClockSkewNanos:              clockSkew,
		Samples:                     active.samples,
	}
	digest, err := ComputeObservationDigest(window)
	if err != nil {
		return err
	}
	window.ObservationDigest = digest
	if err := r.cfg.WindowStore.Save(ctx, window); err != nil {
		return err
	}
	delete(r.tracked, attempt.GetProviderAttemptId())
	return nil
}

func (r *Runner) listAttempts(ctx context.Context) ([]*operatorv1.InferenceProviderAttemptRecord, error) {
	dir := filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname)
	entries, err := r.cfg.FileSvc.ReadDir(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("provider observer runner: list attempts: %w", err)
	}
	attempts := make([]*operatorv1.InferenceProviderAttemptRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), constants.FileExtJSON) {
			continue
		}
		body, err := r.cfg.FileSvc.ReadFile(ctx, filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		record := &operatorv1.InferenceProviderAttemptRecord{}
		if err := protojson.Unmarshal(body, record); err != nil {
			continue
		}
		attempts = append(attempts, record)
	}
	return attempts, nil
}

// DefaultCollector returns the standard provider-boundary hardware collector.
func DefaultCollector() HardwareCollector {
	return &CompositeCollector{
		GPU: NewNvidiaSMICollector(),
		RAM: NewProcMeminfoCollector(),
	}
}

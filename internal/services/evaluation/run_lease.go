// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
)

// RunLease is the durable handle for a process executing one run. It records
// who holds the run so that status, cancel, logs, and archive can tell a live
// run from one whose process is gone.
type RunLease struct {
	RunID             string     `json:"run_id"`
	PID               int        `json:"pid"`
	Host              string     `json:"host"`
	StartedAt         time.Time  `json:"started_at"`
	LogPath           string     `json:"log_path,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
}

// LeaseLiveness reports whether the process named by a lease is running.
type LeaseLiveness func(RunLease) bool

// CancelRequested reports whether cancel has been requested on the lease.
func (l RunLease) CancelRequested() bool {
	return l.CancelRequestedAt != nil
}

// LoadRunLease reads the lease of one run. A run without a lease wraps
// constants.ErrNotFound.
func (s *Store) LoadRunLease(ctx context.Context, runID string) (*RunLease, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, s.layout.runLeasePath(runID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read run lease: %w", err)
	}
	lease := &RunLease{}
	if err := json.Unmarshal(body, lease); err != nil {
		return nil, fmt.Errorf("%w: run lease: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if lease.RunID != runID {
		return nil, fmt.Errorf("%w: run lease run ID does not match requested run", constants.ErrEvidenceScopeMismatch)
	}
	return lease, nil
}

// AcquireRunLease writes the lease for a run. A lease held by a live process
// fails with constants.ErrEvaluationRunLeaseHeld. A stale lease is replaced and
// returned so the caller can report it.
func (s *Store) AcquireRunLease(ctx context.Context, lease RunLease, live LeaseLiveness) (*RunLease, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(lease.RunID) || lease.PID <= 0 {
		return nil, fmt.Errorf("%w: run lease requires a run ID and process ID", constants.ErrEvaluationReportPersistFailed)
	}
	var stale *RunLease
	existing, err := s.LoadRunLease(ctx, lease.RunID)
	switch {
	case err == nil:
		if live != nil && live(*existing) {
			return nil, fmt.Errorf("evaluation: run %q is held by process %d on %s: %w", lease.RunID, existing.PID, existing.Host, constants.ErrEvaluationRunLeaseHeld)
		}
		stale = existing
	case isNotFound(err):
	default:
		return nil, err
	}
	if err := s.writeRunLease(ctx, lease); err != nil {
		return nil, err
	}
	return stale, nil
}

// RequestRunCancel records a cancel request on the run's lease. The executing
// process observes it between assignments.
func (s *Store) RequestRunCancel(ctx context.Context, runID string, at time.Time) (*RunLease, error) {
	lease, err := s.LoadRunLease(ctx, runID)
	if err != nil {
		return nil, err
	}
	requested := at.UTC()
	lease.CancelRequestedAt = &requested
	if err := s.writeRunLease(ctx, *lease); err != nil {
		return nil, err
	}
	return lease, nil
}

// ReleaseRunLease removes a lease held by pid. A lease held by another process
// is left in place.
func (s *Store) ReleaseRunLease(ctx context.Context, runID string, pid int) error {
	lease, err := s.LoadRunLease(ctx, runID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if lease.PID != pid {
		return nil
	}
	return s.ClearRunLease(ctx, runID)
}

// ClearRunLease removes the lease of a run regardless of holder.
func (s *Store) ClearRunLease(ctx context.Context, runID string) error {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	if err := s.files.Remove(ctx, s.layout.runLeasePath(runID)); err != nil {
		return fmt.Errorf("evaluation: remove run lease: %w", err)
	}
	return nil
}

// isNotFound reports a missing file as the runtime file service or the
// standard library describe it.
func isNotFound(err error) bool {
	return errors.Is(err, constants.ErrNotFound) || errors.Is(err, iofs.ErrNotExist)
}

func (s *Store) writeRunLease(ctx context.Context, lease RunLease) error {
	body, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode run lease: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, s.layout.runLeasePath(lease.RunID), body, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: write run lease: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

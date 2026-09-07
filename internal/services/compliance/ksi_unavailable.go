// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// UnavailableInterval records a contiguous interval during which a KSI was
// unavailable (could not be evaluated) or not_satisfied. Preserving these
// intervals alongside satisfied snapshots gives consumers a complete
// operating-effectiveness history rather than only current snapshots.
type UnavailableInterval struct {
	KSIID       string            `json:"ksi_id"`
	ScopeID     string            `json:"scope_id"`
	RunID       string            `json:"run_id"`
	StartUnixMs int64             `json:"start_unix_ms"`
	EndUnixMs   int64             `json:"end_unix_ms"`
	Outcome     KSIOutcome        `json:"outcome"`
	Status      KSIStatus         `json:"status"`
	Reason      string            `json:"reason,omitempty"`
	Binding     EvaluationBinding `json:"binding"`
}

// Validate returns constants.ErrKSIBindingIncomplete when required fields are
// empty or the interval is inverted.
func (u UnavailableInterval) Validate() error {
	if u.KSIID == "" || u.ScopeID == "" || u.RunID == "" || u.StartUnixMs <= 0 || u.EndUnixMs <= 0 || u.StartUnixMs > u.EndUnixMs || !u.Outcome.Valid() || u.Status != KSIStatusNotSatisfied || u.Outcome.Status() != u.Status {
		return fmt.Errorf("%w: unavailable interval binding is incomplete", constants.ErrKSIBindingIncomplete)
	}
	if err := u.Binding.Validate(); err != nil {
		return fmt.Errorf("%w: unavailable interval: %w", constants.ErrKSIBindingIncomplete, err)
	}
	if u.ScopeID != u.Binding.ScopeID || u.RunID != u.Binding.RunID || u.StartUnixMs < u.Binding.WindowStartUnixMs || u.EndUnixMs > u.Binding.WindowEndUnixMs {
		return fmt.Errorf("%w: unavailable interval does not match its evaluation binding", constants.ErrKSIBindingMismatch)
	}
	return nil
}

// UnavailableIntervalStore persists UnavailableInterval records to the .g8e/
// runtime directory via RuntimeFileService. Each interval is a JSONL line in a
// single append-only file, enabling chronological retrieval of complete
// historical failures and unavailable intervals.
type UnavailableIntervalStore struct {
	fileSvc fs.RuntimeFileService
	dir     string
}

// NewUnavailableIntervalStore creates a store rooted at the given relative
// directory (typically constants.DataDirname/constants.ComplianceDirname).
func NewUnavailableIntervalStore(fileSvc fs.RuntimeFileService, dir string) *UnavailableIntervalStore {
	return &UnavailableIntervalStore{fileSvc: fileSvc, dir: dir}
}

// AppendInterval appends a single UnavailableInterval to the persistent JSONL
// file. The file is created if it does not exist.
func (s *UnavailableIntervalStore) AppendInterval(ctx context.Context, interval UnavailableInterval) error {
	if err := interval.Validate(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}

	if err := s.fileSvc.MkdirAll(ctx, s.dir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}

	data, err := json.Marshal(interval)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}
	data = append(data, '\n')

	relPath := filepath.Join(s.dir, constants.KSIUnavailableIntervalsFilename)
	existing, readErr := s.fileSvc.ReadFile(ctx, relPath)
	if readErr != nil && !errors.Is(readErr, constants.ErrNotFound) {
		return fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, readErr)
	}

	combined := append(existing, data...)
	if err := s.fileSvc.WriteFile(ctx, relPath, combined, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}
	return nil
}

// AppendFromResultSet extracts not_satisfied results from a KSIResultSet and
// appends each as an UnavailableInterval. This is the canonical way to
// preserve historical failures after every evaluation: satisfied KSIs are
// recorded as snapshots, while not_satisfied KSIs are also recorded as
// unavailable intervals so consumers can reconstruct complete
// operating-effectiveness history.
func (s *UnavailableIntervalStore) AppendFromResultSet(ctx context.Context, rs *KSIResultSet) error {
	if rs == nil {
		return fmt.Errorf("%w: nil result set", constants.ErrKSIUnavailableWriteFailed)
	}
	if err := rs.Binding.Validate(); err != nil {
		return fmt.Errorf("%w: result set binding: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}
	intervals := make([]UnavailableInterval, 0, len(rs.Results))
	for _, res := range rs.Results {
		if err := res.Binding.Validate(); err != nil {
			return fmt.Errorf("%w: result %s binding: %w", constants.ErrKSIUnavailableWriteFailed, res.ID, err)
		}
		if !res.Binding.Equal(rs.Binding) {
			return fmt.Errorf("%w: result %s binding does not match result set binding", constants.ErrKSIBindingMismatch, res.ID)
		}
		if !res.Outcome.Valid() || res.Status != res.Outcome.Status() {
			return fmt.Errorf("%w: result %s has inconsistent status and outcome", constants.ErrKSIUnavailableWriteFailed, res.ID)
		}
		if res.Status != KSIStatusNotSatisfied {
			continue
		}
		intervals = append(intervals, UnavailableInterval{
			KSIID:       res.ID,
			ScopeID:     rs.Binding.ScopeID,
			RunID:       rs.Binding.RunID,
			StartUnixMs: rs.Binding.WindowStartUnixMs,
			EndUnixMs:   rs.Binding.WindowEndUnixMs,
			Outcome:     res.Outcome,
			Status:      res.Status,
			Reason:      string(res.Outcome),
			Binding:     rs.Binding,
		})
	}
	for _, interval := range intervals {
		if err := s.AppendInterval(ctx, interval); err != nil {
			return err
		}
	}
	return nil
}

// ListIntervals returns all UnavailableInterval records from the persistent
// JSONL file, sorted by start timestamp (oldest first). Returns an empty slice
// if the file does not exist or contains no intervals.
func (s *UnavailableIntervalStore) ListIntervals(ctx context.Context) ([]UnavailableInterval, error) {
	relPath := filepath.Join(s.dir, constants.KSIUnavailableIntervalsFilename)
	data, err := s.fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableReadFailed, err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	intervals := make([]UnavailableInterval, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var interval UnavailableInterval
		if err := json.Unmarshal([]byte(line), &interval); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableParseFailed, err)
		}
		if err := interval.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableParseFailed, err)
		}
		intervals = append(intervals, interval)
	}

	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].StartUnixMs != intervals[j].StartUnixMs {
			return intervals[i].StartUnixMs < intervals[j].StartUnixMs
		}
		return intervals[i].KSIID < intervals[j].KSIID
	})

	return intervals, nil
}

// GetIntervalsForKSI returns the chronological series of UnavailableInterval
// records for the given KSI ID. Returns an empty slice if no intervals exist
// for the KSI.
func (s *UnavailableIntervalStore) GetIntervalsForKSI(ctx context.Context, ksiID string) ([]UnavailableInterval, error) {
	intervals, err := s.ListIntervals(ctx)
	if err != nil {
		return nil, err
	}
	var filtered []UnavailableInterval
	for _, interval := range intervals {
		if interval.KSIID == ksiID {
			filtered = append(filtered, interval)
		}
	}
	return filtered, nil
}

// TotalUnavailableMs returns the total duration in milliseconds that the given
// KSI was unavailable across all recorded intervals. Overlapping intervals are
// not merged; callers should ensure intervals are non-overlapping by recording
// one interval per evaluation.
func (s *UnavailableIntervalStore) TotalUnavailableMs(ctx context.Context, ksiID string) (int64, error) {
	intervals, err := s.GetIntervalsForKSI(ctx, ksiID)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, interval := range intervals {
		total += interval.EndUnixMs - interval.StartUnixMs
	}
	return total, nil
}

// PruneOlderThan removes interval records with EndUnixMs older than the given
// duration from now. Returns the number of records removed. A zero or negative
// duration prunes nothing.
func (s *UnavailableIntervalStore) PruneOlderThan(ctx context.Context, retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}

	relPath := filepath.Join(s.dir, constants.KSIUnavailableIntervalsFilename)
	data, err := s.fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableReadFailed, err)
	}

	cutoffMs := time.Now().Add(-retention).UnixMilli()
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	removed := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		var interval UnavailableInterval
		if err := json.Unmarshal([]byte(line), &interval); err != nil {
			return removed, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableParseFailed, err)
		}
		if err := interval.Validate(); err != nil {
			return removed, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableParseFailed, err)
		}
		if interval.EndUnixMs < cutoffMs {
			removed++
			continue
		}
		kept = append(kept, line)
	}

	if removed == 0 {
		return 0, nil
	}

	if len(kept) == 0 {
		if err := s.fileSvc.Remove(ctx, relPath); err != nil && !errors.Is(err, constants.ErrNotFound) {
			return removed, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
		}
		return removed, nil
	}

	newData := []byte(strings.Join(kept, "\n") + "\n")
	if err := s.fileSvc.WriteFile(ctx, relPath, newData, constants.PermFilePublic); err != nil {
		return removed, fmt.Errorf("%w: %w", constants.ErrKSIUnavailableWriteFailed, err)
	}
	return removed, nil
}

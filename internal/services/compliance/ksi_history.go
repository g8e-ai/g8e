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
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// KSIHistoryStore persists KSIResultSet snapshots to the .g8e/ runtime directory
// via RuntimeFileService. Each snapshot is a JSON file named with the evaluation
// timestamp, enabling chronological retrieval and retention-based pruning.
type KSIHistoryStore struct {
	fileSvc fs.RuntimeFileService
	dir     string
}

// NewKSIHistoryStore creates a history store rooted at the given relative
// directory (typically constants.DataDirname/constants.ComplianceDirname/constants.KSIHistoryDirname).
func NewKSIHistoryStore(fileSvc fs.RuntimeFileService, dir string) *KSIHistoryStore {
	return &KSIHistoryStore{
		fileSvc: fileSvc,
		dir:     dir,
	}
}

// SaveSnapshot writes a KSIResultSet snapshot as a JSON file named with the
// evaluation timestamp. The snapshot is serialized as KSIResultSet JSON.
func (s *KSIHistoryStore) SaveSnapshot(ctx context.Context, rs *KSIResultSet) error {
	if rs == nil {
		return fmt.Errorf("%w: nil result set", constants.ErrKSIHistoryWriteFailed)
	}

	if err := s.fileSvc.MkdirAll(ctx, s.dir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIHistoryWriteFailed, err)
	}

	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIHistoryWriteFailed, err)
	}

	filename := snapshotFilename(rs.EvaluatedAtMs)
	relPath := filepath.Join(s.dir, filename)

	if err := s.fileSvc.WriteFile(ctx, relPath, data, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKSIHistoryWriteFailed, err)
	}

	return nil
}

// ListSnapshots returns all KSIResultSet snapshots in the history directory,
// sorted by evaluation timestamp (oldest first). Returns an empty slice if
// the directory does not exist or contains no snapshots.
func (s *KSIHistoryStore) ListSnapshots(ctx context.Context) ([]KSIResultSet, error) {
	entries, err := s.fileSvc.ReadDir(ctx, s.dir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrKSIHistoryReadFailed, err)
	}

	var snapshots []KSIResultSet
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), constants.KSIHistoryFilenamePrefix) {
			continue
		}
		if !strings.HasSuffix(entry.Name(), constants.FileExtJSON) {
			continue
		}

		relPath := filepath.Join(s.dir, entry.Name())
		data, err := s.fileSvc.ReadFile(ctx, relPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", constants.ErrKSIHistoryReadFailed, entry.Name(), err)
		}

		var rs KSIResultSet
		if err := json.Unmarshal(data, &rs); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", constants.ErrKSIHistoryParseFailed, entry.Name(), err)
		}
		snapshots = append(snapshots, rs)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].EvaluatedAtMs < snapshots[j].EvaluatedAtMs
	})

	return snapshots, nil
}

// GetHistoryForKSI returns the chronological series of KSIResult entries for
// the given KSI ID across all snapshots. Only snapshots containing a result
// for the specified KSI ID are included.
func (s *KSIHistoryStore) GetHistoryForKSI(ctx context.Context, ksiID string) ([]KSIResult, error) {
	snapshots, err := s.ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%w: %s", constants.ErrKSIHistoryEmpty, ksiID)
	}

	var results []KSIResult
	for _, snap := range snapshots {
		for _, res := range snap.Results {
			if res.ID == ksiID {
				results = append(results, res)
				break
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("%w: %s", constants.ErrKSIHistoryEmpty, ksiID)
	}

	return results, nil
}

// PruneOlderThan removes snapshot files older than the given duration from now.
// Returns the number of files removed. A zero or negative duration prunes nothing.
func (s *KSIHistoryStore) PruneOlderThan(ctx context.Context, retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}

	entries, err := s.fileSvc.ReadDir(ctx, s.dir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("%w: %w", constants.ErrKSIHistoryReadFailed, err)
	}

	cutoffMs := time.Now().Add(-retention).UnixMilli()
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), constants.KSIHistoryFilenamePrefix) {
			continue
		}
		if !strings.HasSuffix(entry.Name(), constants.FileExtJSON) {
			continue
		}

		evaluatedAtMs, ok := parseSnapshotFilename(entry.Name())
		if !ok {
			continue
		}
		if evaluatedAtMs < cutoffMs {
			relPath := filepath.Join(s.dir, entry.Name())
			if err := s.fileSvc.Remove(ctx, relPath); err != nil {
				return removed, fmt.Errorf("%w: %s: %w", constants.ErrKSIHistoryWriteFailed, entry.Name(), err)
			}
			removed++
		}
	}

	return removed, nil
}

// snapshotFilename generates a deterministic filename from the evaluation
// timestamp in milliseconds. The format is ksi-result-<unix_ms>.json.
func snapshotFilename(evaluatedAtMs int64) string {
	return fmt.Sprintf("%s%d%s", constants.KSIHistoryFilenamePrefix, evaluatedAtMs, constants.FileExtJSON)
}

// parseSnapshotFilename extracts the evaluation timestamp (Unix ms) from a
// snapshot filename. Returns false if the filename does not match the
// expected format.
func parseSnapshotFilename(name string) (int64, bool) {
	if !strings.HasPrefix(name, constants.KSIHistoryFilenamePrefix) || !strings.HasSuffix(name, constants.FileExtJSON) {
		return 0, false
	}
	tsStr := strings.TrimSuffix(strings.TrimPrefix(name, constants.KSIHistoryFilenamePrefix), constants.FileExtJSON)
	ms, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return ms, true
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
)

func fetchReplayNonces(ctx context.Context, rs *storage.SQLReplayStore) ([]Row, error) {
	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		batch, err := rs.ListNonces(batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("reporting: replay_nonces: failed to list nonces: %w", err)
		}
		for _, n := range batch {
			rows = append(rows, ReplayNonceRow{
				Nonce:         n.Nonce,
				Status:        n.Status,
				ReservedAtUTC: utcRFC3339(n.ReservedAt),
				UsedAtUTC:     optionalTime(n.UsedAt),
				ExpiresAtUTC:  utcRFC3339(n.ExpiresAt),
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}
	return rows, nil
}

func writeReplayNoncesCSV(outDir string, rows []Row) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportReplayNoncesFilename)
	res, err := writeCSV(path, ReplayNonceRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: replay_nonces: failed to write CSV: %w", err)
	}
	res.Filename = constants.ReportReplayNoncesFilename
	return res, nil
}

func reportReplayNonces(ctx context.Context, outDir string, rs *storage.SQLReplayStore) (FileResult, error) {
	rows, err := fetchReplayNonces(ctx, rs)
	if err != nil {
		return FileResult{}, err
	}
	return writeReplayNoncesCSV(outDir, rows)
}

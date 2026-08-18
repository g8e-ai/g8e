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

func reportExecutions(ctx context.Context, outDir string, ev *storage.ExecutionVaultService) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportExecutionsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := ev.ListExecutions(ctx, batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("reporting: list executions: %w", err)
		}
		for _, e := range batch {
			rows = append(rows, ExecutionRow{
				ID:           e.ID,
				TimestampUTC: utcRFC3339(e.TimestampUTC),
				Command:      e.Command,
				ExitCode:     e.ExitCode,
				DurationMs:   e.DurationMs,
				StdoutHash:   e.StdoutHash,
				StdoutSize:   intStr(e.StdoutSize),
				StderrHash:   e.StderrHash,
				StderrSize:   intStr(e.StderrSize),
				CaseID:       e.CaseID,
				TaskID:       e.TaskID,
				OperatorID:   e.OperatorID,
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, ExecutionRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: write executions: %w", err)
	}
	res.Filename = constants.ReportExecutionsFilename
	return res, nil
}

func reportFileDiffs(ctx context.Context, outDir string, ev *storage.ExecutionVaultService) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportFileDiffsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := ev.ListFileDiffs(ctx, batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("reporting: list file diffs: %w", err)
		}
		for _, d := range batch {
			rows = append(rows, FileDiffRow{
				ID:               d.ID,
				TimestampUTC:     utcRFC3339(d.TimestampUTC),
				FilePath:         d.FilePath,
				Operation:        d.Operation,
				LedgerHashBefore: d.LedgerHashBefore,
				LedgerHashAfter:  d.LedgerHashAfter,
				DiffStat:         d.DiffStat,
				DiffHash:         d.DiffHash,
				DiffSize:         intStr(d.DiffSize),
				SessionID:        d.OperatorSessionID,
				CaseID:           d.CaseID,
				OperatorID:       d.OperatorID,
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, FileDiffRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: write file diffs: %w", err)
	}
	res.Filename = constants.ReportFileDiffsFilename
	return res, nil
}

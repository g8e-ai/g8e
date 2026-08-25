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
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
)

func reportLedgerMerkleRoot(ctx context.Context, outDir string, ledger *storage.GitLedgerService) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportLedgerMerkleRootFilename)

	if ctx.Err() != nil {
		return FileResult{}, ctx.Err()
	}

	merkleRoot, err := ledger.GetStateMerkleRoot()
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: ledger merkle root: %w", constants.ErrReportStoreUnavailable, err)
	}

	var rows []Row
	if merkleRoot != "" {
		rows = append(rows, LedgerMerkleRootRow{
			MerkleRoot:    merkleRoot,
			CapturedAtUTC: utcRFC3339(time.Now().UTC()),
		})
	}

	res, err := writeCSV(path, LedgerMerkleRootRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: ledger merkle root: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportLedgerMerkleRootFilename
	return res, nil
}

func reportLedgerCommits(ctx context.Context, outDir string, ledger *storage.GitLedgerService) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportLedgerCommitsFilename)

	if ctx.Err() != nil {
		return FileResult{}, ctx.Err()
	}

	commits, err := ledger.ListCommits("", 0)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: ledger commits: %w", constants.ErrReportStoreUnavailable, err)
	}

	var rows []Row
	for _, c := range commits {
		rows = append(rows, LedgerCommitRow{
			CommitHash:   c.CommitHash,
			ParentHash:   c.ParentHash,
			TimestampUTC: utcRFC3339(c.TimestampUTC),
			Message:      c.Message,
			FilesChanged: intStr(c.FilesChanged),
			DiffStat:     c.DiffStat,
		})
	}

	res, err := writeCSV(path, LedgerCommitRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: ledger commits: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportLedgerCommitsFilename
	return res, nil
}

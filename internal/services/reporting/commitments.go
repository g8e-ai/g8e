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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
)

func reportCommitments(ctx context.Context, outDir string, cl *storage.CommitmentLedger) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportCommitmentsFilename)

	if ctx.Err() != nil {
		return FileResult{}, ctx.Err()
	}

	records, err := cl.ListCommitments()
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: commitments: %w", constants.ErrReportStoreUnavailable, err)
	}

	var rows []Row
	for _, c := range records {
		rows = append(rows, CommitmentRow{
			Seq:                 c.Seq,
			CommittedAtUTC:      utcRFC3339(c.CommittedAt),
			TransactionID:       c.TransactionID,
			TransactionHash:     c.TransactionHash,
			PriorCommitmentHash: c.PriorCommitmentHash,
			Hash:                c.Hash,
			StateRootAtCommit:   c.StateRootAtCommit,
			ActionType:          c.ActionType,
			TargetResource:      c.TargetResource,
			AuditorKeyID:        c.AuditorKeyID,
			Signature:           c.Signature,
		})
	}

	res, err := writeCSV(path, CommitmentRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: commitments: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportCommitmentsFilename
	return res, nil
}

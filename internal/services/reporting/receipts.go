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

func reportReceipts(ctx context.Context, outDir string, store *storage.SQLAuditStore) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportReceiptsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := store.ListActionReceipts("", batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("%w: receipts: %w", constants.ErrReportStoreUnavailable, err)
		}
		for _, r := range batch {
			rows = append(rows, ReceiptRow{
				TransactionID:   r.TransactionID,
				TransactionHash: r.TransactionHash,
				OperatorID:      r.OperatorID,
				SessionID:       r.OperatorSessionID,
				ActionType:      string(r.ActionType),
				TargetResource:  r.TargetResource,
				Status:          string(r.Status),
				StateRootBefore: r.StateRootBefore,
				StateRootAfter:  r.StateRootAfter,
				SignerKeyID:     r.SignerKeyID,
				Signature:       r.Signature,
				ExecutedAtUTC:   utcRFC3339(r.ExecutedAt),
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, ReceiptRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: receipts: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportReceiptsFilename
	return res, nil
}

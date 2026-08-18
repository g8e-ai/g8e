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

func reportSuspendedTransactions(ctx context.Context, outDir string, sts storage.SuspendedTransactionStore) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportSuspendedTxFilename)

	if ctx.Err() != nil {
		return FileResult{}, ctx.Err()
	}

	active, err := sts.ListSuspendedTransactions(ctx, "")
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: suspended_transactions (active): %w", constants.ErrReportStoreUnavailable, err)
	}

	expired, err := sts.GetExpiredSuspendedTransactions(ctx)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: suspended_transactions (expired): %w", constants.ErrReportStoreUnavailable, err)
	}

	all := append(active, expired...)

	var rows []Row
	for _, tx := range all {
		status := constants.SuspendedTxStatusPending
		if tx.Approved {
			status = constants.SuspendedTxStatusApproved
		}

		rows = append(rows, SuspendedTxRow{
			TxHash:            tx.TransactionHash,
			UserID:            tx.UserID,
			ActionType:        tx.ToolName,
			TargetResource:    "",
			Status:            string(status),
			CreatedAtUTC:      utcRFC3339(tx.CreatedAt),
			ExpiresAtUTC:      utcRFC3339(tx.ExpiresAt),
			ApprovedBy:        tx.ApprovedBy,
			ApprovalSignature: tx.ApprovalSignature,
		})
	}

	res, err := writeCSV(path, SuspendedTxRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: suspended_transactions: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportSuspendedTxFilename
	return res, nil
}

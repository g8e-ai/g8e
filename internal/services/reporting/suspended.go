// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		status := "pending"
		if tx.Approved {
			status = "approved"
		}

		rows = append(rows, SuspendedTxRow{
			TxHash:            tx.TransactionHash,
			UserID:            tx.UserID,
			ActionType:        tx.ToolName,
			TargetResource:    "",
			Status:            status,
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

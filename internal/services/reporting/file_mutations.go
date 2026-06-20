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

func reportFileMutations(ctx context.Context, outDir string, store *storage.SQLAuditStore) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportFileMutationsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := store.ListFileMutations(batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("%w: file_mutations: %w", constants.ErrReportStoreUnavailable, err)
		}
		for _, m := range batch {
			rows = append(rows, FileMutationRow{
				ID:               int64Str(m.ID),
				EventID:          int64Str(m.EventID),
				Filepath:         m.Filepath,
				Operation:        string(m.Operation),
				LedgerHashBefore: m.LedgerHashBefore,
				LedgerHashAfter:  m.LedgerHashAfter,
				DiffStat:         m.DiffStat,
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, FileMutationRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("%w: file_mutations: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportFileMutationsFilename
	return res, nil
}

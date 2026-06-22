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
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
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

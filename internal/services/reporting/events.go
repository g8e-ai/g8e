// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
)

func reportSessions(ctx context.Context, outDir string, store *storage.SQLAuditStore) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportSessionsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := store.ListSessions(batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("reporting: sessions: %w", err)
		}
		for _, s := range batch {
			rows = append(rows, SessionRow{
				ID:           s.ID,
				Title:        s.Title,
				UserIdentity: s.UserIdentity,
				CreatedAtUTC: utcRFC3339(s.CreatedAt),
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, SessionRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: sessions: %w", err)
	}
	res.Filename = constants.ReportSessionsFilename
	return res, nil
}

func reportEvents(ctx context.Context, outDir string, store *storage.SQLAuditStore) (FileResult, error) {
	path := filepath.Join(outDir, constants.ReportEventsFilename)

	var rows []Row
	offset := 0
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return FileResult{}, ctx.Err()
		}
		batch, err := store.ListEvents("", batchSize, offset)
		if err != nil {
			return FileResult{}, fmt.Errorf("reporting: events: %w", err)
		}
		for _, e := range batch {
			contentSHA256 := ""
			contentSize := ""
			if e.ContentText != "" {
				h := sha256.Sum256([]byte(e.ContentText))
				contentSHA256 = hex.EncodeToString(h[:])
				contentSize = intStr(len(e.ContentText))
			}
			rows = append(rows, EventRow{
				ID:              e.ID,
				SessionID:       e.OperatorSessionID,
				TimestampUTC:    utcRFC3339(e.Timestamp),
				Type:            string(e.Type),
				CommandRaw:      e.CommandRaw,
				ExitCode:        e.CommandExitCode,
				DurationMs:      e.ExecutionDurationMs,
				ContentSHA256:   contentSHA256,
				ContentSize:     contentSize,
				ContentText:     e.ContentText,
				StdoutTruncated: boolStr(e.StdoutTruncated),
				StderrTruncated: boolStr(e.StderrTruncated),
			})
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	res, err := writeCSV(path, EventRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: events: %w", err)
	}
	res.Filename = constants.ReportEventsFilename
	return res, nil
}

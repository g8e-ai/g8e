// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// FileResult is returned by writeCSV with the file stats needed for the manifest.
type FileResult struct {
	Filename string
	SHA256   string
	RowCount int
}

// writeCSV writes rows to path, returning a FileResult with sha256 and row count.
// The header is derived from the first row's Columns() method.
// If rows is empty, only the header is written.
func writeCSV(path string, header []string, rows []Row) (FileResult, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return FileResult{}, fmt.Errorf("reporting: open %s: %w", path, constants.ErrFileOpenFailed)
	}
	defer f.Close()

	h := sha256.New()
	w := csv.NewWriter(io.MultiWriter(f, h))

	if err := w.Write(header); err != nil {
		return FileResult{}, fmt.Errorf("reporting: write header: %w", constants.ErrReportWriteFailed)
	}

	count := 0
	for _, row := range rows {
		if err := w.Write(row.Record()); err != nil {
			return FileResult{}, fmt.Errorf("reporting: write row: %w", constants.ErrReportWriteFailed)
		}
		count++
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return FileResult{}, fmt.Errorf("reporting: flush: %w", constants.ErrReportWriteFailed)
	}

	return FileResult{
		Filename: path,
		SHA256:   hex.EncodeToString(h.Sum(nil)),
		RowCount: count,
	}, nil
}

// utcRFC3339 formats a time.Time as UTC RFC3339 for deterministic output.
func utcRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// optionalTime formats a time pointer as UTC RFC3339, or empty string if nil.
func optionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return utcRFC3339(*t)
}

// intStr formats an int as string.
func intStr(n int) string {
	return fmt.Sprintf("%d", n)
}

// boolStr returns "true" or "false".
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

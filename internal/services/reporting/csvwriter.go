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
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
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
		return FileResult{}, fmt.Errorf("reporting: open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	w := csv.NewWriter(io.MultiWriter(f, h))

	if err := w.Write(header); err != nil {
		return FileResult{}, fmt.Errorf("reporting: write header: %w", err)
	}

	count := 0
	for _, row := range rows {
		if err := w.Write(row.Record()); err != nil {
			return FileResult{}, fmt.Errorf("reporting: write row: %w", err)
		}
		count++
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return FileResult{}, fmt.Errorf("reporting: flush: %w", err)
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

// boolStr returns "true" or "false".
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// optionalInt formats a pointer to int as string, empty if nil.
func optionalInt(n *int) string {
	if n == nil {
		return ""
	}
	return intStr(*n)
}

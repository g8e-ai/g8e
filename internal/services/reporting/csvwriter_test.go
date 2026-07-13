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
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestUTCRFC3339(t *testing.T) {
	t.Run("converts to UTC", func(t *testing.T) {
		loc, _ := time.LoadLocation("America/New_York")
		ts := time.Date(2026, 1, 15, 10, 30, 0, 0, loc)
		result := utcRFC3339(ts)
		assert.Equal(t, "2026-01-15T15:30:00Z", result)
	})

	t.Run("already UTC stays UTC", func(t *testing.T) {
		ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		result := utcRFC3339(ts)
		assert.Equal(t, "2026-06-01T12:00:00Z", result)
	})
}

func TestOptionalTime(t *testing.T) {
	t.Run("nil returns empty string", func(t *testing.T) {
		assert.Equal(t, "", optionalTime(nil))
	})

	t.Run("non-nil returns RFC3339", func(t *testing.T) {
		ts := time.Date(2026, 3, 10, 8, 45, 0, 0, time.UTC)
		result := optionalTime(&ts)
		assert.Equal(t, "2026-03-10T08:45:00Z", result)
	})
}

func TestIntStr(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{-1, "-1"},
		{1000000, "1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, intStr(tt.input))
		})
	}
}

func TestBoolStr(t *testing.T) {
	assert.Equal(t, "true", boolStr(true))
	assert.Equal(t, "false", boolStr(false))
}

func TestWriteCSV(t *testing.T) {
	t.Run("writes header and rows", func(t *testing.T) {
		dir := testutil.TempDir(t)
		path := filepath.Join(dir, "test.csv")

		rows := []Row{
			ReceiptRow{TransactionID: "tx-1", ActionType: "FS_READ", Status: "OK"},
			ReceiptRow{TransactionID: "tx-2", ActionType: "FS_WRITE", Status: "OK"},
		}

		res, err := writeCSV(path, ReceiptRow{}.Columns(), rows)
		require.NoError(t, err)
		assert.Equal(t, 2, res.RowCount)
		assert.NotEmpty(t, res.SHA256)

		f, err := os.Open(path)
		require.NoError(t, err)
		defer f.Close()

		r := csv.NewReader(f)
		records, err := r.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 3)
		assert.Equal(t, ReceiptRow{}.Columns(), records[0])
		assert.Equal(t, "tx-1", records[1][0])
		assert.Equal(t, "tx-2", records[2][0])
	})

	t.Run("writes only header when rows is empty", func(t *testing.T) {
		dir := testutil.TempDir(t)
		path := filepath.Join(dir, "empty.csv")

		res, err := writeCSV(path, SessionRow{}.Columns(), nil)
		require.NoError(t, err)
		assert.Equal(t, 0, res.RowCount)

		f, err := os.Open(path)
		require.NoError(t, err)
		defer f.Close()

		r := csv.NewReader(f)
		records, err := r.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, SessionRow{}.Columns(), records[0])
	})

	t.Run("returns error on invalid path", func(t *testing.T) {
		dir := testutil.TempDir(t)
		blocker := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))

		_, err := writeCSV(filepath.Join(blocker, "file.csv"), ReceiptRow{}.Columns(), nil)
		require.Error(t, err)
	})
}

func TestRecordTypeForFile(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"receipts.csv", "receipts"},
		{"events.csv", "events"},
		{"verification_summary.csv", "verification_summary"},
		{"/some/path/commitments.csv", "commitments"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, recordTypeForFile(tt.input))
		})
	}
}

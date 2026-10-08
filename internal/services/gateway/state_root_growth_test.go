// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

// seedHistoryDocuments inserts n receipt-sized documents in one transaction,
// standing in for accumulated audit/receipt history.
func seedHistoryDocuments(t *testing.T, db *CanonicalDBService, n int) {
	t.Helper()
	body := strings.Repeat("x", 4096)
	now := timesvc.FormatTimestamp(time.Now().UTC())
	require.NoError(t, db.db.ExecInTxWithRetry(t.Context(), func(tx *sql.Tx) error {
		stmt, err := tx.Prepare("INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES ('history', ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i := 0; i < n; i++ {
			if _, err := stmt.Exec(fmt.Sprintf("h-%06d", i), fmt.Sprintf(`{"seq":%d,"body":%q}`, i, body), now, now); err != nil {
				return err
			}
		}
		return nil
	}))
}

// TestStateRoot_OneWriteCostAtGrowingHistory records the cost of the root read
// that follows one ordinary write, at growing history sizes. Each governed
// mutation reads the root after a write (L5 state_after), so this is the
// per-envelope commitment cost. The test logs the measurements; the bounded-work
// assertions live in the incremental commitment tests.
func TestStateRoot_OneWriteCostAtGrowingHistory(t *testing.T) {
	for _, n := range []int{100, 1000, 10000} {
		t.Run(fmt.Sprintf("history=%d", n), func(t *testing.T) {
			db := newTestDB(t)
			seedHistoryDocuments(t, db, n)
			svc := db.GetStateRootSvc()
			_, err := svc.GetCurrentStateRoot(t.Context())
			require.NoError(t, err)

			const rounds = 20
			var total, worst time.Duration
			for i := 0; i < rounds; i++ {
				require.NoError(t, db.GetDocStore().DocSet("operators", "op-1", json.RawMessage(fmt.Sprintf(`{"heartbeat":%d}`, i))))
				start := time.Now()
				_, err := svc.GetCurrentStateRoot(t.Context())
				require.NoError(t, err)
				elapsed := time.Since(start)
				total += elapsed
				worst = max(worst, elapsed)
			}
			t.Logf("history=%d root-after-write mean=%s max=%s", n, total/rounds, worst)
		})
	}
}

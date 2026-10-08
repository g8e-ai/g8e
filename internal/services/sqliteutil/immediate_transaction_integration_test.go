// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package sqliteutil

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestImmediateTransactionCanceledCommitReleasesPooledConnection(t *testing.T) {
	db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "transaction.db")), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("CREATE TABLE records(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = db.ExecInImmediateTxWithRetry(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, "INSERT INTO records VALUES (1)"); err != nil {
			return err
		}
		cancel() // The callback succeeds, but COMMIT must fail and roll back.
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.ExecInImmediateTxWithRetry(t.Context(), func(conn *sql.Conn) error {
		_, err := conn.ExecContext(t.Context(), "INSERT INTO records VALUES (2)")
		return err
	}))
	var count, id int
	require.NoError(t, db.QueryRow("SELECT count(*), min(id) FROM records").Scan(&count, &id))
	require.Equal(t, 1, count)
	require.Equal(t, 2, id)
}

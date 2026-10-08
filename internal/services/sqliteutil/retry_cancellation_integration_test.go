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
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRetryHelpersCancellationWaitingForConnection(t *testing.T) {
	for name, run := range map[string]func(context.Context, *DB) error{
		"exec": func(ctx context.Context, db *DB) error {
			_, err := db.ExecWithRetry(ctx, "SELECT 1")
			return err
		},
		"query": func(ctx context.Context, db *DB) error {
			rows, err := db.QueryWithRetry(ctx, "SELECT 1")
			if rows != nil {
				defer rows.Close()
				return errors.Join(err, rows.Err())
			}
			return err
		},
		"query row": func(ctx context.Context, db *DB) error {
			var n int
			return db.QueryRowWithRetry(ctx, "SELECT 1").Scan(&n)
		},
		"transaction": func(ctx context.Context, db *DB) error {
			return db.ExecInTxWithRetry(ctx, func(*sql.Tx) error { return nil })
		},
		"immediate transaction": func(ctx context.Context, db *DB) error {
			return db.ExecInImmediateTxWithRetry(ctx, func(*sql.Conn) error { return nil })
		},
		"materialize": func(ctx context.Context, db *DB) error {
			_, err := MaterializeRows(ctx, db, "SELECT 1", nil, func(rows *sql.Rows) (int, error) {
				var n int
				err := rows.Scan(&n)
				return n, err
			})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "pool.db")), testutil.NewTestLogger())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			db.SetMaxOpenConns(1)
			conn, err := db.Conn(t.Context())
			require.NoError(t, err)
			defer conn.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			start := time.Now()
			require.ErrorIs(t, run(ctx, db), context.DeadlineExceeded)
			require.Less(t, time.Since(start), 500*time.Millisecond)
		})
	}
}

func TestRetryHelpersBusyCancellationAndExhaustion(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		for name, run := range map[string]func(context.Context, *DB) error{
			"exec": func(ctx context.Context, db *DB) error {
				_, err := db.ExecWithRetry(ctx, "INSERT INTO records VALUES (1)")
				return err
			},
			"transaction": func(ctx context.Context, db *DB) error {
				return db.ExecInTxWithRetry(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, "INSERT INTO records VALUES (1)")
					return err
				})
			},
			"immediate transaction": func(ctx context.Context, db *DB) error {
				return db.ExecInImmediateTxWithRetry(ctx, func(conn *sql.Conn) error {
					_, err := conn.ExecContext(ctx, "INSERT INTO records VALUES (1)")
					return err
				})
			},
		} {
			t.Run(name+map[bool]string{false: "/cancel", true: "/exhaust"}[exhausted], func(t *testing.T) {
				cfg := DefaultDBConfig(filepath.Join(testutil.TempDir(t), "busy.db"))
				cfg.BusyTimeoutMs = 1
				cfg.RetryBaseDelayMs = 1000
				if exhausted {
					cfg.MaxRetries = 1
				}
				db, err := OpenDB(cfg, testutil.NewTestLogger())
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				_, err = db.Exec("CREATE TABLE records(id INTEGER PRIMARY KEY)")
				require.NoError(t, err)
				writer, err := db.Conn(t.Context())
				require.NoError(t, err)
				defer writer.Close()
				spare, err := db.Conn(t.Context())
				require.NoError(t, err)
				require.NoError(t, spare.Close())
				_, err = writer.ExecContext(t.Context(), "BEGIN IMMEDIATE")
				require.NoError(t, err)
				defer writer.ExecContext(context.Background(), "ROLLBACK")
				ctx := t.Context()
				if !exhausted {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
					defer cancel()
				}
				started := time.Now()
				err = run(ctx, db)
				require.Less(t, time.Since(started), 500*time.Millisecond)
				if exhausted {
					require.ErrorIs(t, err, constants.ErrSQLiteBusy)
					require.True(t, IsBusyError(err))
				} else {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
				require.NoError(t, func() error { _, err := writer.ExecContext(t.Context(), "ROLLBACK"); return err }())
				// Every failed attempt rolled back; the same pool remains usable.
				require.NoError(t, db.ExecInImmediateTxWithRetry(t.Context(), func(conn *sql.Conn) error {
					_, err := conn.ExecContext(t.Context(), "INSERT INTO records VALUES (2)")
					return err
				}))
				var count, id int
				require.NoError(t, db.QueryRow("SELECT count(*), min(id) FROM records").Scan(&count, &id))
				require.Equal(t, 1, count)
				require.Equal(t, 2, id)
			})
		}
	}
}

func TestBusyErrorClassificationUsesSQLiteCodes(t *testing.T) {
	db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "codes.db")), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("CREATE TABLE records(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	writer, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer writer.Close()
	spare, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer spare.Close()
	_, err = spare.ExecContext(t.Context(), "PRAGMA busy_timeout=1")
	require.NoError(t, err)
	_, err = writer.ExecContext(t.Context(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	defer writer.ExecContext(context.Background(), "ROLLBACK")
	_, err = spare.ExecContext(t.Context(), "INSERT INTO records VALUES (1)")
	require.Error(t, err)
	require.True(t, IsBusyError(errors.Join(constants.ErrPlatformEnrollmentGovernanceRejected, err)))
	// Rendered text alone must never trigger a retry or change an HTTP status.
	require.False(t, IsBusyError(errors.New("database is locked (5) (SQLITE_BUSY)")))
}

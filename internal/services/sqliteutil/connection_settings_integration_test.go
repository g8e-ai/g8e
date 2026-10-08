// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package sqliteutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// Hold connections concurrently so each probe exercises a different physical
// connection, including connections opened after the initial Ping.
func TestOpenDB_SettingsApplyToEveryConnection(t *testing.T) {
	cfg := DefaultDBConfig(filepath.Join(testutil.TempDir(t), "connections.db"))
	cfg.BusyTimeoutMs = 1234
	cfg.CacheSizeMB = 8
	db, err := OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for i := 0; i < 3; i++ {
		conn, err := db.Conn(t.Context())
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, conn.Close()) })
		for _, setting := range []struct {
			name string
			want int
		}{
			{"synchronous", 1}, // NORMAL, not the new-connection default FULL.
			{"busy_timeout", cfg.BusyTimeoutMs},
			{"foreign_keys", 1},
			{"cache_size", -cfg.CacheSizeMB * 1024},
			{"temp_store", 2},
			{"auto_vacuum", 2},
		} {
			var got int
			require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA "+setting.name).Scan(&got))
			require.Equal(t, setting.want, got, "connection %d: %s", i, setting.name)
		}
		var journal string
		require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journal))
		require.Equal(t, "wal", journal)
	}
}

func TestOpenReadOnlyDB_SettingsApplyToEveryConnection(t *testing.T) {
	cfg := DefaultDBConfig(filepath.Join(testutil.TempDir(t), "readonly.db"))
	cfg.BusyTimeoutMs = 1234
	db, err := OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "CREATE TABLE records(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	readOnly, err := OpenReadOnlyDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	readOnly.SetMaxOpenConns(3)
	for i := 0; i < 3; i++ {
		conn, err := readOnly.Conn(t.Context())
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, conn.Close()) })
		for _, setting := range []struct {
			name string
			want int
		}{
			{"query_only", 1},
			{"busy_timeout", cfg.BusyTimeoutMs},
		} {
			var got int
			require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA "+setting.name).Scan(&got))
			require.Equal(t, setting.want, got, "connection %d: %s", i, setting.name)
		}
		_, err = conn.ExecContext(t.Context(), "INSERT INTO records VALUES (1)")
		require.Error(t, err)
		var count int
		require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT count(*) FROM records").Scan(&count))
		require.Zero(t, count)
	}
}

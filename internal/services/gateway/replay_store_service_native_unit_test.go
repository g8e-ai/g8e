// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newReplayStoreServiceWithDB creates a ReplayStoreService with a mock DB
// for unit testing.
func newReplayStoreServiceWithDB(db replayStoreDB, logger *slog.Logger) *ReplayStoreService {
	return &ReplayStoreService{
		db:     db,
		logger: logger,
	}
}

// mockReplayStoreDB is a mock implementation of replayStoreDB for unit testing.
type mockReplayStoreDB struct {
	queryRowWithRetryFunc func(query string, args ...any) rowScanner
	execWithRetryFunc     func(query string, args ...any) (sql.Result, error)
}

func (m *mockReplayStoreDB) QueryRowWithRetry(query string, args ...any) rowScanner {
	if m.queryRowWithRetryFunc != nil {
		return m.queryRowWithRetryFunc(query, args...)
	}
	return &mockRow{}
}

func (m *mockReplayStoreDB) ExecWithRetry(query string, args ...any) (sql.Result, error) {
	if m.execWithRetryFunc != nil {
		return m.execWithRetryFunc(query, args...)
	}
	return nil, nil
}

// mockRow is a mock row scanner for testing.
type mockRow struct {
	scanFunc func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	return nil
}

// mockResult is a mock sql.Result for testing.
type mockResult struct {
	rowsAffectedFunc func() (int64, error)
	lastInsertIdFunc func() (int64, error)
}

func (m *mockResult) RowsAffected() (int64, error) {
	if m.rowsAffectedFunc != nil {
		return m.rowsAffectedFunc()
	}
	return 1, nil
}

func (m *mockResult) LastInsertId() (int64, error) {
	if m.lastInsertIdFunc != nil {
		return m.lastInsertIdFunc()
	}
	return 1, nil
}

func TestReplayStoreService_ReserveNonce_Success(t *testing.T) {
	logger := testutil.NewTestLogger()

	calledQuery := false
	calledExec := false

	mockDB := &mockReplayStoreDB{
		queryRowWithRetryFunc: func(query string, args ...any) rowScanner {
			calledQuery = true
			assert.Equal(t, "SELECT nonce FROM nonces WHERE nonce = ?", query)
			assert.Equal(t, "test-nonce", args[0])

			// Return sql.ErrNoRows to simulate nonce not found
			row := &mockRow{
				scanFunc: func(dest ...any) error {
					return sql.ErrNoRows
				},
			}
			return row
		},
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			calledExec = true
			assert.Equal(t, "INSERT INTO nonces (nonce, expires_at, status) VALUES (?, ?, 'reserved')", query)
			assert.Equal(t, "test-nonce", args[0])

			return &mockResult{}, nil
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)
	expiresAt := time.Now().Add(1 * time.Hour)

	replayed, err := svc.ReserveNonce("test-nonce", expiresAt)

	require.NoError(t, err)
	assert.False(t, replayed, "first reservation should not detect replay")
	assert.True(t, calledQuery, "QueryRowWithRetry should be called")
	assert.True(t, calledExec, "ExecWithRetry should be called")
}

func TestReplayStoreService_ReserveNonce_ReplayDetected(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		queryRowWithRetryFunc: func(query string, args ...any) rowScanner {
			// Return a row that scans successfully to simulate existing nonce
			row := &mockRow{
				scanFunc: func(dest ...any) error {
					if len(dest) > 0 {
						if strPtr, ok := dest[0].(*string); ok {
							*strPtr = "test-nonce"
						}
					}
					return nil
				},
			}
			return row
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)
	expiresAt := time.Now().Add(1 * time.Hour)

	replayed, err := svc.ReserveNonce("test-nonce", expiresAt)

	require.NoError(t, err)
	assert.True(t, replayed, "should detect replay for existing nonce")
}

func TestReplayStoreService_ReserveNonce_QueryError(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		queryRowWithRetryFunc: func(query string, args ...any) rowScanner {
			row := &mockRow{
				scanFunc: func(dest ...any) error {
					return errors.New("database connection failed")
				},
			}
			return row
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)
	expiresAt := time.Now().Add(1 * time.Hour)

	replayed, err := svc.ReserveNonce("test-nonce", expiresAt)

	require.Error(t, err)
	assert.False(t, replayed)
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestReplayStoreService_ReserveNonce_InsertError(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		queryRowWithRetryFunc: func(query string, args ...any) rowScanner {
			row := &mockRow{
				scanFunc: func(dest ...any) error {
					return sql.ErrNoRows
				},
			}
			return row
		},
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			return nil, errors.New("insert failed")
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)
	expiresAt := time.Now().Add(1 * time.Hour)

	replayed, err := svc.ReserveNonce("test-nonce", expiresAt)

	require.Error(t, err)
	assert.False(t, replayed)
	assert.Contains(t, err.Error(), "insert failed")
}

func TestReplayStoreService_ReserveNonce_ConcurrentInsert(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		queryRowWithRetryFunc: func(query string, args ...any) rowScanner {
			row := &mockRow{
				scanFunc: func(dest ...any) error {
					return sql.ErrNoRows
				},
			}
			return row
		},
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			// Simulate unique constraint violation (concurrent insert)
			return nil, errors.New("UNIQUE constraint failed: nonces.nonce")
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)
	expiresAt := time.Now().Add(1 * time.Hour)

	replayed, err := svc.ReserveNonce("test-nonce", expiresAt)

	require.NoError(t, err)
	assert.True(t, replayed, "concurrent insert should be treated as replay")
}

func TestReplayStoreService_FinalizeNonce_Success(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			assert.Equal(t, "UPDATE nonces SET status = 'used' WHERE nonce = ? AND status = 'reserved'", query)
			assert.Equal(t, "test-nonce", args[0])
			return &mockResult{}, nil
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.FinalizeNonce("test-nonce")

	require.NoError(t, err)
}

func TestReplayStoreService_FinalizeNonce_Error(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			return nil, errors.New("update failed")
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.FinalizeNonce("test-nonce")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "finalize nonce")
	assert.Contains(t, err.Error(), "update failed")
}

func TestReplayStoreService_ReleaseNonce_Success(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			assert.Equal(t, "DELETE FROM nonces WHERE nonce = ? AND status = 'reserved'", query)
			assert.Equal(t, "test-nonce", args[0])
			return &mockResult{}, nil
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.ReleaseNonce("test-nonce")

	require.NoError(t, err)
}

func TestReplayStoreService_ReleaseNonce_Error(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			return nil, errors.New("delete failed")
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.ReleaseNonce("test-nonce")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "release nonce")
	assert.Contains(t, err.Error(), "delete failed")
}

func TestReplayStoreService_CleanupExpiredNonces_Success(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			assert.Equal(t, "DELETE FROM nonces WHERE expires_at < ?", query)
			return &mockResult{}, nil
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.CleanupExpiredNonces()

	require.NoError(t, err)
}

func TestReplayStoreService_CleanupExpiredNonces_Error(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{
		execWithRetryFunc: func(query string, args ...any) (sql.Result, error) {
			return nil, errors.New("cleanup failed")
		},
	}

	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.CleanupExpiredNonces()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup expired nonces")
	assert.Contains(t, err.Error(), "cleanup failed")
}

func TestReplayStoreService_Close(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{}
	svc := newReplayStoreServiceWithDB(mockDB, logger)

	err := svc.Close()

	require.NoError(t, err, "Close should be a no-op and not error")
}

func TestReplayStoreService_NewReplayStoreService(t *testing.T) {
	logger := testutil.NewTestLogger()

	mockDB := &mockReplayStoreDB{}
	svc := newReplayStoreServiceWithDB(mockDB, logger)

	assert.NotNil(t, svc)
	assert.Equal(t, mockDB, svc.db)
	assert.Equal(t, logger, svc.logger)
}

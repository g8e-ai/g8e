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

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

//go:generate mockery --name SuspendedTransactionStore --output ./mocks --dir .

// SuspendedTransactionStore defines the interface for L3 approval workflow storage.
// This service stores transactions awaiting human approval.
//
// All methods that return errors must wrap errors with context using
// fmt.Errorf("suspended_transaction_store: action: %w", err) to provide clear error attribution.
type SuspendedTransactionStore interface {
	// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
	// Returns an error if storage fails, wrapping the underlying error with context.
	StoreSuspendedTransaction(ctx context.Context, tx *models.SuspendedTransaction) error

	// GetSuspendedTransaction retrieves a suspended transaction by hash.
	// Returns (nil, false) if not found or expired.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error)

	// ListSuspendedTransactions retrieves all non-expired suspended transactions.
	// Optionally filters by user_id if provided.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	ListSuspendedTransactions(ctx context.Context, userID string) ([]*models.SuspendedTransaction, error)

	// ApproveSuspendedTransaction marks a suspended transaction as approved with cryptographic signature.
	// The approvalPublicKey is the hex-encoded Ed25519 public key of the approver, used for
	// cryptographic signature verification at L3 notary verification time.
	// Returns an error if approval fails, wrapping the underlying error with context.
	ApproveSuspendedTransaction(ctx context.Context, txHash, approvedBy, approvalSignature, expectedCertFingerprint, approvalPublicKey string) error

	// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
	// Returns an error if deletion fails, wrapping the underlying error with context.
	DeleteSuspendedTransaction(ctx context.Context, txHash string) error

	// CleanupExpiredSuspendedTransactions removes expired suspended transactions.
	// Returns the count of deleted transactions.
	// Returns an error if cleanup fails, wrapping the underlying error with context.
	CleanupExpiredSuspendedTransactions(ctx context.Context) (int64, error)

	// GetExpiredSuspendedTransactions retrieves expired suspended transactions for audit.
	// Returns the list of expired transactions with their full details.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetExpiredSuspendedTransactions(ctx context.Context) ([]*models.SuspendedTransaction, error)
}

// SuspendedTransactionConfig holds configuration for the suspended transaction store service.
type SuspendedTransactionConfig struct {
	DBPath               string
	MaxDBSizeMB          int64
	RetentionDays        int
	PruneIntervalMinutes int
}

// DefaultSuspendedTransactionConfig returns the default configuration.
func DefaultSuspendedTransactionConfig() *SuspendedTransactionConfig {
	return &SuspendedTransactionConfig{
		DBPath:               constants.SuspendedTransactionDBPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
}

// SuspendedTransactionService provides storage for L3 approval workflow transactions.
// This service stores transactions awaiting human approval.
type SuspendedTransactionService struct {
	db     *sqliteutil.DB
	config *SuspendedTransactionConfig
	logger *slog.Logger
	pruner *sqliteutil.Pruner

	wg sync.WaitGroup
}

// Ensure SuspendedTransactionService implements SuspendedTransactionStore.
var _ SuspendedTransactionStore = (*SuspendedTransactionService)(nil)

// NewSuspendedTransactionService creates a new suspended transaction store service.
func NewSuspendedTransactionService(config *SuspendedTransactionConfig, logger *slog.Logger) (*SuspendedTransactionService, error) {
	if config == nil {
		config = DefaultSuspendedTransactionConfig()
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if _, err := db.Exec(suspendedTransactionSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	sts := &SuspendedTransactionService{
		config: config,
		logger: logger,
		db:     db,
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	sts.pruner = sqliteutil.NewPruner(db, logger, interval, suspendedTransactionPrune(config))
	sts.pruner.Start()

	sts.logger.Info("Suspended transaction store initialized",
		"db_path", config.DBPath)
	return sts, nil
}

// suspendedTransactionSchema defines the initial schema for the suspended transaction database.
const suspendedTransactionSchema = `
CREATE TABLE IF NOT EXISTS suspended_transactions (
	transaction_hash TEXT PRIMARY KEY,
	envelope TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	tool_name TEXT,
	tool_arguments TEXT,
	user_id TEXT,
	operator_id TEXT,
	approved INTEGER DEFAULT 0,
	approved_at TEXT,
	approved_by TEXT,
	approval_signature TEXT,
	expected_cert_fingerprint TEXT,
	approval_public_key TEXT
);

CREATE INDEX IF NOT EXISTS idx_suspended_expires_at ON suspended_transactions(expires_at);
`

// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
func (sts *SuspendedTransactionService) StoreSuspendedTransaction(ctx context.Context, tx *models.SuspendedTransaction) error {
	if sts == nil || sts.db == nil {
		return fmt.Errorf("suspended_transaction_store: store transaction: store not initialized")
	}
	query := `
	INSERT INTO suspended_transactions (
		transaction_hash, envelope, created_at, expires_at,
		tool_name, tool_arguments, user_id, operator_id,
		approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint, approval_public_key
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(transaction_hash) DO UPDATE SET
		envelope = excluded.envelope,
		expires_at = excluded.expires_at,
		approved = excluded.approved,
		approved_at = excluded.approved_at,
		approved_by = excluded.approved_by,
		approval_signature = excluded.approval_signature,
		expected_cert_fingerprint = excluded.expected_cert_fingerprint,
		approval_public_key = excluded.approval_public_key
	`

	var approvedAtStr *string
	if tx.ApprovedAt != nil {
		ts := sqliteutil.FormatTimestamp(*tx.ApprovedAt)
		approvedAtStr = &ts
	}

	_, err := sts.db.ExecWithRetry(
		query,
		tx.TransactionHash,
		string(tx.Envelope),
		sqliteutil.FormatTimestamp(tx.CreatedAt),
		sqliteutil.FormatTimestamp(tx.ExpiresAt),
		tx.ToolName,
		string(tx.ToolArguments),
		tx.UserID,
		tx.OperatorID,
		tx.Approved,
		approvedAtStr,
		tx.ApprovedBy,
		tx.ApprovalSignature,
		tx.ExpectedCertFingerprint,
		tx.ApprovalPublicKey,
	)
	if err != nil {
		return fmt.Errorf("suspended_transaction_store: store transaction: %w", err)
	}
	return nil
}

// GetSuspendedTransaction retrieves a suspended transaction by hash.
// Returns (nil, false, nil) if not found or expired.
func (sts *SuspendedTransactionService) GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	if sts == nil || sts.db == nil {
		return nil, false, fmt.Errorf("suspended_transaction_store: get transaction: store not initialized")
	}
	var envelopeStr, createdAtStr, expiresAtStr, toolName, toolArgsStr, userID, operatorID, approvedBy, approvalSignature, expectedCertFingerprint, approvalPublicKey sql.NullString
	var approved int
	var approvedAtStr sql.NullString
	err := sts.db.QueryRowWithRetry(
		"SELECT envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint, approval_public_key FROM suspended_transactions WHERE transaction_hash = ? AND expires_at > ?",
		txHash, sqliteutil.NowTimestamp(),
	).Scan(&envelopeStr, &createdAtStr, &expiresAtStr, &toolName, &toolArgsStr, &userID, &operatorID, &approved, &approvedAtStr, &approvedBy, &approvalSignature, &expectedCertFingerprint, &approvalPublicKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		sts.logger.Error("Failed to query suspended transaction", "tx_hash", txHash, string(constants.ConnectionStateError), err)
		return nil, false, fmt.Errorf("suspended_transaction_store: get transaction: %w", err)
	}

	createdAt, _ := sqliteutil.ParseTimestamp(createdAtStr.String)
	expiresAt, _ := sqliteutil.ParseTimestamp(expiresAtStr.String)

	var toolArgs []byte
	if toolArgsStr.Valid {
		toolArgs = []byte(toolArgsStr.String)
	}

	var approvedAt *time.Time
	if approvedAtStr.Valid {
		ts, _ := sqliteutil.ParseTimestamp(approvedAtStr.String)
		approvedAt = &ts
	}

	return &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                []byte(envelopeStr.String),
		CreatedAt:               createdAt,
		ExpiresAt:               expiresAt,
		ToolName:                toolName.String,
		ToolArguments:           toolArgs,
		UserID:                  userID.String,
		OperatorID:              operatorID.String,
		Approved:                approved == 1,
		ApprovedAt:              approvedAt,
		ApprovedBy:              approvedBy.String,
		ApprovalSignature:       approvalSignature.String,
		ExpectedCertFingerprint: expectedCertFingerprint.String,
		ApprovalPublicKey:       approvalPublicKey.String,
	}, true, nil
}

// ListSuspendedTransactions retrieves all non-expired suspended transactions.
// Optionally filters by user_id if provided.
func (sts *SuspendedTransactionService) ListSuspendedTransactions(ctx context.Context, userID string) ([]*models.SuspendedTransaction, error) {
	if sts == nil || sts.db == nil {
		return nil, fmt.Errorf("suspended_transaction_store: list transactions: store not initialized")
	}

	var query string
	var args []interface{}

	if userID != "" {
		query = "SELECT transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint, approval_public_key FROM suspended_transactions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC"
		args = []interface{}{userID, sqliteutil.NowTimestamp()}
	} else {
		query = "SELECT transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint, approval_public_key FROM suspended_transactions WHERE expires_at > ? ORDER BY created_at DESC"
		args = []interface{}{sqliteutil.NowTimestamp()}
	}

	type suspendedTxRow struct {
		txHash                  sql.NullString
		envelopeStr             sql.NullString
		createdAtStr            sql.NullString
		expiresAtStr            sql.NullString
		toolName                sql.NullString
		toolArgsStr             sql.NullString
		userID                  sql.NullString
		operatorID              sql.NullString
		approved                int
		approvedAtStr           sql.NullString
		approvedBy              sql.NullString
		approvalSignature       sql.NullString
		expectedCertFingerprint sql.NullString
		approvalPublicKey       sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(sts.db, query, args, func(r *sql.Rows) (suspendedTxRow, error) {
		var row suspendedTxRow
		err := r.Scan(&row.txHash, &row.envelopeStr, &row.createdAtStr, &row.expiresAtStr, &row.toolName, &row.toolArgsStr, &row.userID, &row.operatorID, &row.approved, &row.approvedAtStr, &row.approvedBy, &row.approvalSignature, &row.expectedCertFingerprint, &row.approvalPublicKey)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("suspended_transaction_store: list transactions: %w", err)
	}

	var transactions []*models.SuspendedTransaction
	for _, row := range rows {
		createdAt, _ := sqliteutil.ParseTimestamp(row.createdAtStr.String)
		expiresAt, _ := sqliteutil.ParseTimestamp(row.expiresAtStr.String)

		var toolArgs []byte
		if row.toolArgsStr.Valid {
			toolArgs = []byte(row.toolArgsStr.String)
		}

		var approvedAt *time.Time
		if row.approvedAtStr.Valid {
			ts, _ := sqliteutil.ParseTimestamp(row.approvedAtStr.String)
			approvedAt = &ts
		}

		transactions = append(transactions, &models.SuspendedTransaction{
			TransactionHash:         row.txHash.String,
			Envelope:                []byte(row.envelopeStr.String),
			CreatedAt:               createdAt,
			ExpiresAt:               expiresAt,
			ToolName:                row.toolName.String,
			ToolArguments:           toolArgs,
			UserID:                  row.userID.String,
			OperatorID:              row.operatorID.String,
			Approved:                row.approved == 1,
			ApprovedAt:              approvedAt,
			ApprovedBy:              row.approvedBy.String,
			ApprovalSignature:       row.approvalSignature.String,
			ExpectedCertFingerprint: row.expectedCertFingerprint.String,
			ApprovalPublicKey:       row.approvalPublicKey.String,
		})
	}

	return transactions, nil
}

// ApproveSuspendedTransaction marks a suspended transaction as approved with cryptographic signature.
// This is called by the CLI approval command when a human approves a transaction.
func (sts *SuspendedTransactionService) ApproveSuspendedTransaction(ctx context.Context, txHash, approvedBy, approvalSignature, expectedCertFingerprint, approvalPublicKey string) error {
	if sts == nil || sts.db == nil {
		return fmt.Errorf("suspended_transaction_store: approve transaction: store not initialized")
	}
	sts.wg.Add(1)
	defer sts.wg.Done()

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	result, err := sts.db.ExecWithRetry(
		`UPDATE suspended_transactions 
		 SET approved = 1, approved_at = ?, approved_by = ?, approval_signature = ?, expected_cert_fingerprint = ?, approval_public_key = ?
		 WHERE transaction_hash = ? AND expires_at > ?`,
		nowStr, approvedBy, approvalSignature, expectedCertFingerprint, approvalPublicKey, txHash, sqliteutil.NowTimestamp(),
	)
	if err != nil {
		return fmt.Errorf("suspended_transaction_store: approve transaction: %w", err)
	}

	// Check if any row was actually updated
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("suspended_transaction_store: approve transaction: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("suspended_transaction_store: approve transaction: transaction not found or expired")
	}

	return nil
}

// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
func (sts *SuspendedTransactionService) DeleteSuspendedTransaction(ctx context.Context, txHash string) error {
	if sts == nil || sts.db == nil {
		return fmt.Errorf("suspended_transaction_store: delete transaction: store not initialized")
	}
	_, err := sts.db.ExecWithRetry("DELETE FROM suspended_transactions WHERE transaction_hash = ?", txHash)
	if err != nil {
		return fmt.Errorf("suspended_transaction_store: delete transaction: %w", err)
	}
	return nil
}

// CleanupExpiredSuspendedTransactions removes expired suspended transactions.
// Returns the count of deleted transactions.
func (sts *SuspendedTransactionService) CleanupExpiredSuspendedTransactions(ctx context.Context) (int64, error) {
	if sts == nil || sts.db == nil {
		return 0, fmt.Errorf("suspended_transaction_store: cleanup expired: store not initialized")
	}
	result, err := sts.db.ExecWithRetry("DELETE FROM suspended_transactions WHERE expires_at < ?", sqliteutil.NowTimestamp())
	if err != nil {
		return 0, fmt.Errorf("suspended_transaction_store: cleanup expired: %w", err)
	}
	return result.RowsAffected()
}

// GetExpiredSuspendedTransactions retrieves expired suspended transactions for audit.
// Returns the list of expired transactions with their full details.
func (sts *SuspendedTransactionService) GetExpiredSuspendedTransactions(ctx context.Context) ([]*models.SuspendedTransaction, error) {
	if sts == nil || sts.db == nil {
		return nil, fmt.Errorf("suspended_transaction_store: get expired transactions: store not initialized")
	}

	query := "SELECT transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint, approval_public_key FROM suspended_transactions WHERE expires_at < ? ORDER BY expires_at ASC"

	type suspendedTxRow struct {
		txHash                  sql.NullString
		envelopeStr             sql.NullString
		createdAtStr            sql.NullString
		expiresAtStr            sql.NullString
		toolName                sql.NullString
		toolArgsStr             sql.NullString
		userID                  sql.NullString
		operatorID              sql.NullString
		approved                int
		approvedAtStr           sql.NullString
		approvedBy              sql.NullString
		approvalSignature       sql.NullString
		expectedCertFingerprint sql.NullString
		approvalPublicKey       sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(sts.db, query, []interface{}{sqliteutil.NowTimestamp()}, func(r *sql.Rows) (suspendedTxRow, error) {
		var row suspendedTxRow
		err := r.Scan(&row.txHash, &row.envelopeStr, &row.createdAtStr, &row.expiresAtStr, &row.toolName, &row.toolArgsStr, &row.userID, &row.operatorID, &row.approved, &row.approvedAtStr, &row.approvedBy, &row.approvalSignature, &row.expectedCertFingerprint, &row.approvalPublicKey)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("suspended_transaction_store: get expired transactions: %w", err)
	}

	var transactions []*models.SuspendedTransaction
	for _, row := range rows {
		createdAt, _ := sqliteutil.ParseTimestamp(row.createdAtStr.String)
		expiresAt, _ := sqliteutil.ParseTimestamp(row.expiresAtStr.String)

		var toolArgs []byte
		if row.toolArgsStr.Valid {
			toolArgs = []byte(row.toolArgsStr.String)
		}

		var approvedAt *time.Time
		if row.approvedAtStr.Valid {
			ts, _ := sqliteutil.ParseTimestamp(row.approvedAtStr.String)
			approvedAt = &ts
		}

		transactions = append(transactions, &models.SuspendedTransaction{
			TransactionHash:         row.txHash.String,
			Envelope:                []byte(row.envelopeStr.String),
			CreatedAt:               createdAt,
			ExpiresAt:               expiresAt,
			ToolName:                row.toolName.String,
			ToolArguments:           toolArgs,
			UserID:                  row.userID.String,
			OperatorID:              row.operatorID.String,
			Approved:                row.approved == 1,
			ApprovedAt:              approvedAt,
			ApprovedBy:              row.approvedBy.String,
			ApprovalSignature:       row.approvalSignature.String,
			ExpectedCertFingerprint: row.expectedCertFingerprint.String,
			ApprovalPublicKey:       row.approvalPublicKey.String,
		})
	}

	return transactions, nil
}

// suspendedTransactionPrune returns a PruneFunc for retention and size-based pruning.
func suspendedTransactionPrune(config *SuspendedTransactionConfig) sqliteutil.PruneFunc {
	return func(ctx context.Context, db *sqliteutil.DB, logger *slog.Logger) error {
		result, err := db.ExecWithRetry("DELETE FROM suspended_transactions WHERE expires_at < ?", sqliteutil.NowTimestamp())
		if err != nil {
			logger.Error("Failed to prune expired suspended transactions", "error", err)
			return err
		}
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned expired suspended transactions", "rows_deleted", rowsDeleted)
		}

		dbSizeBytes, err := db.GetSizeBytes()
		if err != nil {
			logger.Warn("Failed to get database size", "error", err)
		}
		maxSizeBytes := config.MaxDBSizeMB * 1024 * 1024

		if err == nil && dbSizeBytes > maxSizeBytes {
			_, err := db.ExecWithRetry(`
				DELETE FROM suspended_transactions
				WHERE transaction_hash IN (
					SELECT transaction_hash FROM suspended_transactions
					ORDER BY created_at ASC
					LIMIT (SELECT COUNT(*) / 10 FROM suspended_transactions)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune suspended_transactions for size limit", "error", err)
			}

			logger.Info("Pruned for size limit", "db_size_mb", dbSizeBytes/(1024*1024))
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", "error", err)
		}
		return nil
	}
}

// Close shuts down the suspended transaction store service.
func (sts *SuspendedTransactionService) Close() error {
	if sts == nil {
		return nil
	}

	if sts.pruner != nil {
		sts.pruner.Stop()
	}

	if sts.db != nil {
		return sts.db.Close()
	}

	return nil
}

// Wait blocks until all background workers and writes have finished.
func (sts *SuspendedTransactionService) Wait() {
	sts.wg.Wait()
}

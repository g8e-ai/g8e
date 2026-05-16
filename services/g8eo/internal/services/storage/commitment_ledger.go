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
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sqliteutil"
)

// CommitmentLedger is the SQLite-backed storage for commitment attestations.
// It stores raw JSON attestations with atomic append operations to guarantee
// chain integrity under concurrent writes. This type is in the storage package
// to avoid import cycles with the governance package.
type CommitmentLedger struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewCommitmentLedger creates a new commitment ledger backed by the given SQLite database.
// The database must already have the commitment_ledger table created via migrations.
func NewCommitmentLedger(db *sqliteutil.DB, logger *slog.Logger) *CommitmentLedger {
	return &CommitmentLedger{
		db:     db,
		logger: logger,
	}
}

// GetLatestCommitmentJSON retrieves the most recent commitment as raw JSON.
// Returns (nil, nil) when the ledger is empty (genesis state).
func (cl *CommitmentLedger) GetLatestCommitmentJSON() ([]byte, error) {
	if cl == nil || cl.db == nil {
		return nil, fmt.Errorf("commitment ledger not initialized")
	}

	query := `
	SELECT attestation_json
	FROM commitment_ledger
	ORDER BY committed_at_unix_ms DESC
	LIMIT 1
	`

	var attestationJSON string
	err := cl.db.QueryRow(query).Scan(&attestationJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest commitment: %w", err)
	}

	return []byte(attestationJSON), nil
}

// AppendCommitmentJSON atomically appends a new commitment (as JSON) to the ledger.
// This operation is transactional to ensure two concurrent attestations cannot
// both chain to the same prior_hash. The JSON must contain a "prior_commitment_hash" field.
func (cl *CommitmentLedger) AppendCommitmentJSON(attestationJSON []byte, priorHash, hash string) error {
	if cl == nil || cl.db == nil {
		return fmt.Errorf("commitment ledger not initialized")
	}
	if len(attestationJSON) == 0 {
		return fmt.Errorf("attestation JSON is empty")
	}

	// Parse JSON to extract required fields for the structured columns
	var fields struct {
		TransactionID               string `json:"transaction_id"`
		TransactionHash             string `json:"transaction_hash"`
		StateRootAtCommit           string `json:"state_root_at_commit"`
		L2SignatureDigest           string `json:"l2_signature_digest"`
		WardenIntentSignatureDigest string `json:"warden_intent_signature_digest"`
		HumanSignatureDigest        string `json:"human_signature_digest"`
		ActionType                  string `json:"action_type"`
		TargetResource              string `json:"target_resource"`
		CommittedAtUnixMs           int64  `json:"committed_at_unix_ms"`
		AuditorKeyID                string `json:"auditor_key_id"`
		Signature                   string `json:"signature"`
	}

	if err := json.Unmarshal(attestationJSON, &fields); err != nil {
		return fmt.Errorf("failed to unmarshal attestation JSON: %w", err)
	}

	// Use a transaction to ensure atomicity
	tx, err := cl.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify the prior_hash matches the current latest commitment (if any)
	var currentPriorHash string
	checkQuery := `
	SELECT hash
	FROM commitment_ledger
	ORDER BY committed_at_unix_ms DESC
	LIMIT 1
	`
	err = tx.QueryRow(checkQuery).Scan(&currentPriorHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query current prior hash: %w", err)
	}

	// If ledger is not empty, verify chain integrity
	if err != sql.ErrNoRows && currentPriorHash != priorHash {
		return fmt.Errorf("prior_commitment_hash mismatch: expected %s, got %s", currentPriorHash, priorHash)
	}

	// Insert the new commitment
	insertQuery := `
	INSERT INTO commitment_ledger (
		transaction_id,
		transaction_hash,
		prior_commitment_hash,
		state_root_at_commit,
		l2_signature_digest,
		warden_intent_signature_digest,
		human_signature_digest,
		action_type,
		target_resource,
		committed_at_unix_ms,
		auditor_key_id,
		signature,
		hash,
		attestation_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = tx.Exec(insertQuery,
		fields.TransactionID,
		fields.TransactionHash,
		priorHash,
		fields.StateRootAtCommit,
		fields.L2SignatureDigest,
		fields.WardenIntentSignatureDigest,
		fields.HumanSignatureDigest,
		fields.ActionType,
		fields.TargetResource,
		fields.CommittedAtUnixMs,
		fields.AuditorKeyID,
		fields.Signature,
		hash,
		attestationJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert commitment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if cl.logger != nil {
		cl.logger.Info("Commitment appended to ledger",
			"transaction_id", fields.TransactionID,
			"commitment_hash", hash,
			"prior_commitment_hash", priorHash)
	}

	return nil
}

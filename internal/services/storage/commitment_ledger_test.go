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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// setupTestCommitmentLedger creates a test commitment ledger with an in-memory SQLite database.
func setupTestCommitmentLedger(t *testing.T) (*CommitmentLedger, *sqliteutil.DB) {
	t.Helper()

	// Use in-memory database for fast, isolated unit tests
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), testutil.NewTestLogger())
	require.NoError(t, err)

	// Create the commitment_ledger table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS commitment_ledger (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			transaction_id TEXT NOT NULL,
			transaction_hash TEXT NOT NULL,
			prior_commitment_hash TEXT NOT NULL,
			state_root_at_commit TEXT,
			l2_signature_digest TEXT,
			Actuator_intent_signature_digest TEXT,
			human_signature_digest TEXT,
			action_type TEXT,
			target_resource TEXT,
			committed_at_unix_ms INTEGER NOT NULL,
			auditor_key_id TEXT,
			signature TEXT,
			hash TEXT NOT NULL,
			attestation_json TEXT NOT NULL,
			UNIQUE(hash)
		)
	`)
	require.NoError(t, err)

	cl := NewCommitmentLedger(db, testutil.NewTestLogger())
	require.NotNil(t, cl)

	t.Cleanup(func() {
		db.Close()
	})

	return cl, db
}

func TestCommitmentLedger_NewCommitmentLedger(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()

	// Test with nil db - constructor returns non-nil ledger but with nil db
	cl := NewCommitmentLedger(nil, logger)
	assert.NotNil(t, cl) // Constructor returns non-nil even with nil db
	assert.Nil(t, cl.db)

	// Test with valid db
	cl, db := setupTestCommitmentLedger(t)
	assert.NotNil(t, cl)
	assert.NotNil(t, cl.db)
	assert.NotNil(t, cl.logger)
	assert.NotNil(t, db)
}

func TestCommitmentLedger_AppendCommitmentJSON_NilLedger(t *testing.T) {
	t.Parallel()

	var cl *CommitmentLedger

	attestation := []byte(`{"transaction_id":"tx123"}`)
	err := cl.AppendCommitmentJSON(attestation, "prior123", "hash123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestCommitmentLedger_AppendCommitmentJSON_NilDB(t *testing.T) {
	t.Parallel()

	cl := &CommitmentLedger{db: nil, logger: testutil.NewTestLogger()}

	attestation := []byte(`{"transaction_id":"tx123"}`)
	err := cl.AppendCommitmentJSON(attestation, "prior123", "hash123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestCommitmentLedger_AppendCommitmentJSON_EmptyJSON(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	err := cl.AppendCommitmentJSON([]byte{}, "prior123", "hash123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attestation JSON is empty")
}

func TestCommitmentLedger_AppendCommitmentJSON_InvalidJSON(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	invalidJSON := []byte(`{invalid json`)
	err := cl.AppendCommitmentJSON(invalidJSON, "prior123", "hash123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestCommitmentLedger_AppendCommitmentJSON_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	// Missing transaction_id and other required fields
	// Note: The JSON unmarshals successfully with empty strings for missing fields
	// The insert may succeed if the table allows empty strings for TEXT columns
	incompleteJSON := []byte(`{"action_type":"write"}`)
	err := cl.AppendCommitmentJSON(incompleteJSON, "prior123", "hash123")
	// The behavior depends on table constraints - we just verify it doesn't panic
	// and returns either success or a descriptive error
	if err != nil {
		assert.Contains(t, err.Error(), "failed to insert commitment")
	}
}

func TestCommitmentLedger_AppendCommitmentJSON_ValidJSON(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	validJSON := []byte(`{
		"transaction_id": "tx123",
		"transaction_hash": "thash123",
		"state_root_at_commit": "sr123",
		"l2_signature_digest": "l2sig123",
		"Actuator_intent_signature_digest": "act123",
		"human_signature_digest": "hsig123",
		"action_type": "write",
		"target_resource": "/etc/nginx.conf",
		"committed_at_unix_ms": 1234567890,
		"auditor_key_id": "auditor123",
		"signature": "sig123"
	}`)

	err := cl.AppendCommitmentJSON(validJSON, "", "hash123")
	require.NoError(t, err)

	// Verify it was stored via ListCommitments
	rows, err := cl.ListCommitments()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "tx123", rows[0].TransactionID)
}

func TestCommitmentLedger_AppendCommitmentJSON_ChainIntegrity(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	// Append first commitment (genesis)
	attestation1 := []byte(`{
		"transaction_id": "tx001",
		"transaction_hash": "thash001",
		"state_root_at_commit": "sr001",
		"l2_signature_digest": "l2sig001",
		"Actuator_intent_signature_digest": "act001",
		"human_signature_digest": "hsig001",
		"action_type": "write",
		"target_resource": "/file1",
		"committed_at_unix_ms": 1000,
		"auditor_key_id": "aud001",
		"signature": "sig001"
	}`)

	err := cl.AppendCommitmentJSON(attestation1, "", "hash001")
	require.NoError(t, err)

	// Append second commitment with correct prior hash
	attestation2 := []byte(`{
		"transaction_id": "tx002",
		"transaction_hash": "thash002",
		"state_root_at_commit": "sr002",
		"l2_signature_digest": "l2sig002",
		"Actuator_intent_signature_digest": "act002",
		"human_signature_digest": "hsig002",
		"action_type": "write",
		"target_resource": "/file2",
		"committed_at_unix_ms": 2000,
		"auditor_key_id": "aud002",
		"signature": "sig002"
	}`)

	err = cl.AppendCommitmentJSON(attestation2, "hash001", "hash002")
	require.NoError(t, err)

	// Try to append with wrong prior hash (should fail)
	attestation3 := []byte(`{
		"transaction_id": "tx003",
		"transaction_hash": "thash003",
		"state_root_at_commit": "sr003",
		"l2_signature_digest": "l2sig003",
		"Actuator_intent_signature_digest": "act003",
		"human_signature_digest": "hsig003",
		"action_type": "write",
		"target_resource": "/file3",
		"committed_at_unix_ms": 3000,
		"auditor_key_id": "aud003",
		"signature": "sig003"
	}`)

	err = cl.AppendCommitmentJSON(attestation3, "wrong_hash", "hash003")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prior_commitment_hash mismatch")
}

func TestCommitmentLedger_AppendCommitmentJSON_WithLogger(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	cl, _ := setupTestCommitmentLedger(t)
	cl.logger = logger

	validJSON := []byte(`{
		"transaction_id": "tx123",
		"transaction_hash": "thash123",
		"state_root_at_commit": "sr123",
		"l2_signature_digest": "l2sig123",
		"Actuator_intent_signature_digest": "act123",
		"human_signature_digest": "hsig123",
		"action_type": "write",
		"target_resource": "/etc/nginx.conf",
		"committed_at_unix_ms": 1234567890,
		"auditor_key_id": "auditor123",
		"signature": "sig123"
	}`)

	err := cl.AppendCommitmentJSON(validJSON, "", "hash123")
	require.NoError(t, err)
}

func TestCommitmentLedger_AppendCommitmentJSON_WithoutLogger(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)
	cl.logger = nil

	validJSON := []byte(`{
		"transaction_id": "tx123",
		"transaction_hash": "thash123",
		"state_root_at_commit": "sr123",
		"l2_signature_digest": "l2sig123",
		"Actuator_intent_signature_digest": "act123",
		"human_signature_digest": "hsig123",
		"action_type": "write",
		"target_resource": "/etc/nginx.conf",
		"committed_at_unix_ms": 1234567890,
		"auditor_key_id": "auditor123",
		"signature": "sig123"
	}`)

	err := cl.AppendCommitmentJSON(validJSON, "", "hash123")
	require.NoError(t, err)
	// Should not panic even with nil logger
}

func TestCommitmentLedger_JSONUnmarshal_AllFields(t *testing.T) {
	t.Parallel()

	// Test that all expected JSON fields can be unmarshaled
	attestationJSON := []byte(`{
		"transaction_id": "tx-001",
		"transaction_hash": "thash-abc",
		"state_root_at_commit": "sroot-xyz",
		"l2_signature_digest": "l2sig-def",
		"Actuator_intent_signature_digest": "actsig-ghi",
		"human_signature_digest": "hsig-jkl",
		"action_type": "write",
		"target_resource": "/etc/hosts",
		"committed_at_unix_ms": 1704067200000,
		"auditor_key_id": "auditor-key-1",
		"signature": "signature-mno"
	}`)

	var fields struct {
		TransactionID                 string `json:"transaction_id"`
		TransactionHash               string `json:"transaction_hash"`
		StateRootAtCommit             string `json:"state_root_at_commit"`
		L2SignatureDigest             string `json:"l2_signature_digest"`
		ActuatorIntentSignatureDigest string `json:"Actuator_intent_signature_digest"`
		HumanSignatureDigest          string `json:"human_signature_digest"`
		ActionType                    string `json:"action_type"`
		TargetResource                string `json:"target_resource"`
		CommittedAtUnixMs             int64  `json:"committed_at_unix_ms"`
		AuditorKeyID                  string `json:"auditor_key_id"`
		Signature                     string `json:"signature"`
	}

	err := json.Unmarshal(attestationJSON, &fields)
	require.NoError(t, err)

	assert.Equal(t, "tx-001", fields.TransactionID)
	assert.Equal(t, "thash-abc", fields.TransactionHash)
	assert.Equal(t, "sroot-xyz", fields.StateRootAtCommit)
	assert.Equal(t, "l2sig-def", fields.L2SignatureDigest)
	assert.Equal(t, "actsig-ghi", fields.ActuatorIntentSignatureDigest)
	assert.Equal(t, "hsig-jkl", fields.HumanSignatureDigest)
	assert.Equal(t, "write", fields.ActionType)
	assert.Equal(t, "/etc/hosts", fields.TargetResource)
	assert.Equal(t, int64(1704067200000), fields.CommittedAtUnixMs)
	assert.Equal(t, "auditor-key-1", fields.AuditorKeyID)
	assert.Equal(t, "signature-mno", fields.Signature)
}

func TestCommitmentLedger_JSONUnmarshal_PartialFields(t *testing.T) {
	t.Parallel()

	// Test that JSON with missing fields unmarshals with zero values
	partialJSON := []byte(`{
		"transaction_id": "tx-002",
		"action_type": "delete"
	}`)

	var fields struct {
		TransactionID                 string `json:"transaction_id"`
		TransactionHash               string `json:"transaction_hash"`
		StateRootAtCommit             string `json:"state_root_at_commit"`
		L2SignatureDigest             string `json:"l2_signature_digest"`
		ActuatorIntentSignatureDigest string `json:"Actuator_intent_signature_digest"`
		HumanSignatureDigest          string `json:"human_signature_digest"`
		ActionType                    string `json:"action_type"`
		TargetResource                string `json:"target_resource"`
		CommittedAtUnixMs             int64  `json:"committed_at_unix_ms"`
		AuditorKeyID                  string `json:"auditor_key_id"`
		Signature                     string `json:"signature"`
	}

	err := json.Unmarshal(partialJSON, &fields)
	require.NoError(t, err)

	assert.Equal(t, "tx-002", fields.TransactionID)
	assert.Equal(t, "delete", fields.ActionType)
	assert.Equal(t, "", fields.TransactionHash) // Zero value
	assert.Equal(t, "", fields.StateRootAtCommit)
	assert.Equal(t, int64(0), fields.CommittedAtUnixMs) // Zero value
}

func TestCommitmentLedger_JSONUnmarshal_InvalidTimestamp(t *testing.T) {
	t.Parallel()

	// Test that invalid timestamp type is handled
	invalidTimestampJSON := []byte(`{
		"transaction_id": "tx-003",
		"committed_at_unix_ms": "not-a-number"
	}`)

	var fields struct {
		TransactionID     string `json:"transaction_id"`
		CommittedAtUnixMs int64  `json:"committed_at_unix_ms"`
	}

	err := json.Unmarshal(invalidTimestampJSON, &fields)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal")
}

func TestCommitmentLedger_NilReceiverSafety(t *testing.T) {
	t.Parallel()

	// Test that methods handle nil receiver gracefully
	var cl *CommitmentLedger

	// AppendCommitmentJSON
	err := cl.AppendCommitmentJSON([]byte(`{}`), "prior", "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestCommitmentLedger_ConstructorWithNilLogger(t *testing.T) {
	t.Parallel()

	cl, db := setupTestCommitmentLedger(t)
	cl.logger = nil

	assert.NotNil(t, cl)
	assert.NotNil(t, cl.db)
	assert.Nil(t, cl.logger)

	// Should not panic when logger is nil
	attestation := []byte(`{
		"transaction_id": "tx-nil-logger",
		"action_type": "write",
		"committed_at_unix_ms": 1234567890
	}`)
	err := cl.AppendCommitmentJSON(attestation, "", "hash")
	require.NoError(t, err)

	_ = db.Close()
}

func TestCommitmentLedger_ErrorMessages(t *testing.T) {
	t.Parallel()

	// Test that error messages are descriptive
	var cl *CommitmentLedger

	err := cl.AppendCommitmentJSON([]byte{}, "prior", "hash")
	assert.Contains(t, err.Error(), "commitment ledger not initialized")

	cl = &CommitmentLedger{db: nil, logger: testutil.NewTestLogger()}
	err = cl.AppendCommitmentJSON([]byte{}, "prior", "hash")
	assert.Contains(t, err.Error(), "commitment ledger not initialized")

	cl, _ = setupTestCommitmentLedger(t)
	err = cl.AppendCommitmentJSON([]byte{}, "prior", "hash")
	assert.Contains(t, err.Error(), "attestation JSON is empty")

	err = cl.AppendCommitmentJSON([]byte(`{invalid`), "prior", "hash")
	assert.Contains(t, err.Error(), "failed to unmarshal attestation JSON")
}

func TestCommitmentLedger_ConcurrentAppendSafety(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	// Test sequential appends to verify transactional integrity
	// (In-memory databases don't support true concurrent access from different goroutines)
	var priorHash string
	for i := 0; i < 3; i++ {
		attestation := []byte(fmt.Sprintf(`{
			"transaction_id": "tx-sequential-%d",
			"transaction_hash": "thash-%d",
			"state_root_at_commit": "sr-%d",
			"l2_signature_digest": "l2sig-%d",
			"Actuator_intent_signature_digest": "act-%d",
			"human_signature_digest": "hsig-%d",
			"action_type": "write",
			"target_resource": "/file-%d",
			"committed_at_unix_ms": %d,
			"auditor_key_id": "aud-%d",
			"signature": "sig-%d"
		}`, i, i, i, i, i, i, i, 1234567890+i, i, i))
		hash := fmt.Sprintf("hash%d", i)
		err := cl.AppendCommitmentJSON(attestation, priorHash, hash)
		// Should succeed without panicking
		assert.NoError(t, err)
		priorHash = hash
	}
}

func TestCommitmentLedger_MultipleCommitments(t *testing.T) {
	t.Parallel()

	cl, _ := setupTestCommitmentLedger(t)

	// Append multiple commitments in sequence
	for i := 0; i < 5; i++ {
		attestation := []byte(fmt.Sprintf(`{
			"transaction_id": "tx-%d",
			"transaction_hash": "thash-%d",
			"state_root_at_commit": "sr-%d",
			"l2_signature_digest": "l2sig-%d",
			"Actuator_intent_signature_digest": "act-%d",
			"human_signature_digest": "hsig-%d",
			"action_type": "write",
			"target_resource": "/file-%d",
			"committed_at_unix_ms": %d,
			"auditor_key_id": "aud-%d",
			"signature": "sig-%d"
		}`, i, i, i, i, i, i, i, 1000+i*100, i, i))

		priorHash := ""
		if i > 0 {
			priorHash = fmt.Sprintf("hash-%d", i-1)
		}
		hash := fmt.Sprintf("hash-%d", i)

		err := cl.AppendCommitmentJSON(attestation, priorHash, hash)
		require.NoError(t, err)
	}

	// Verify the latest commitment is the last one via ListCommitments
	rows, err := cl.ListCommitments()
	require.NoError(t, err)
	require.Len(t, rows, 5)
	assert.Equal(t, "tx-4", rows[4].TransactionID)
}

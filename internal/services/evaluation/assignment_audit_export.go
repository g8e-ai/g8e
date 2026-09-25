// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	AssignmentAuditSliceKind    = "assignment_audit_slice"
	AssignmentAuditVaultKeyKind = "assignment_audit_vault_key"
	assignmentAuditSchemaRef    = "g8e.eval.v1.PublicAssignmentAuditSlice"
)

// AssignmentAuditSliceEvent is one disclosure-approved row written into an
// assignment audit export. Payload must already be valid public feed JSON.
type AssignmentAuditSliceEvent struct {
	Type      string
	Timestamp string
	Payload   []byte
}

// AssignmentAuditSliceArtifacts is the content-addressed export package for one
// assignment: a standalone SQLite file and a dedicated vault key.
type AssignmentAuditSliceArtifacts struct {
	Database       []byte
	DatabaseSHA256 string
	VaultKey       []byte
	VaultKeySHA256 string
}

var assignmentAuditProhibitedKeys = []string{
	"prompt", "output", "reasoning", "session_id", "transaction_id",
	"command_raw", "receipt_json", "private_key", "vault_key",
}

// BuildAssignmentAuditSlice builds a WAL-checkpointed SQLite audit slice and a
// fresh vault key that decrypts only that file. Payloads are validated against
// the public feed allowlist before encryption.
func BuildAssignmentAuditSlice(assignmentID string, events []AssignmentAuditSliceEvent) (AssignmentAuditSliceArtifacts, error) {
	if strings.TrimSpace(assignmentID) == "" {
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: %w", constants.ErrMissingRequiredField)
	}
	if len(events) == 0 {
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: no events: %w", constants.ErrMissingRequiredField)
	}
	normalized, err := normalizeAssignmentAuditEvents(events)
	if err != nil {
		return AssignmentAuditSliceArtifacts{}, err
	}

	privateKey := deriveAssignmentAuditVaultKey(assignmentID, normalized)
	header, dek, err := vault.NewVaultHeader(privateKey)
	if err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: %w", err)
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: marshal vault header: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "assignment-audit-*.db")
	if err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: create temp db: %w", err)
	}
	dbPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), slog.Default())
	if err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: open db: %w", err)
	}
	defer db.Close()

	if err := initAssignmentAuditSchema(db); err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, err
	}
	if _, err := db.Exec(`INSERT INTO vault_header (id, header_json) VALUES (1, ?)`, string(headerJSON)); err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: write vault header: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, session_type, title) VALUES (?, 'app', ?)`, assignmentID, assignmentID); err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: write session: %w", err)
	}

	priorHash := ""
	for _, event := range normalized {
		encrypted, err := encryptAssignmentAuditPayload(dek, event.Payload)
		if err != nil {
			vault.SecureZero(privateKey)
			vault.SecureZero(dek)
			return AssignmentAuditSliceArtifacts{}, err
		}
		result, err := db.Exec(`
			INSERT INTO events (operator_session_id, timestamp, type, content_text, encrypted)
			VALUES (?, ?, ?, ?, 1)
		`, assignmentID, event.Timestamp, event.Type, encrypted)
		if err != nil {
			vault.SecureZero(privateKey)
			vault.SecureZero(dek)
			return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: write event: %w", err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			vault.SecureZero(privateKey)
			vault.SecureZero(dek)
			return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: event id: %w", err)
		}
		priorHash, err = appendAssignmentAuditCommitment(db, priorHash, eventID, event)
		if err != nil {
			vault.SecureZero(privateKey)
			vault.SecureZero(dek)
			return AssignmentAuditSliceArtifacts{}, err
		}
	}

	if _, err := db.ExecContext(context.Background(), `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: checkpoint: %w", err)
	}
	if err := db.Close(); err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: close db: %w", err)
	}

	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		vault.SecureZero(privateKey)
		vault.SecureZero(dek)
		return AssignmentAuditSliceArtifacts{}, fmt.Errorf("evaluation: build assignment audit slice: read db: %w", err)
	}
	keyBytes := []byte(hex.EncodeToString(privateKey) + "\n")
	vault.SecureZero(privateKey)
	vault.SecureZero(dek)

	return AssignmentAuditSliceArtifacts{
		Database:       dbBytes,
		DatabaseSHA256: hashBytes(dbBytes),
		VaultKey:       keyBytes,
		VaultKeySHA256: hashBytes(keyBytes),
	}, nil
}

// VerifyAssignmentAuditSlice decrypts event payloads and checks the commitment
// chain without contacting the public mirror.
func VerifyAssignmentAuditSlice(dbBytes []byte, vaultKey []byte) error {
	if len(dbBytes) == 0 || len(vaultKey) == 0 {
		return fmt.Errorf("evaluation: verify assignment audit slice: %w", constants.ErrMissingRequiredField)
	}
	privateKey, err := decodeAssignmentAuditVaultKey(vaultKey)
	if err != nil {
		return err
	}
	defer vault.SecureZero(privateKey)

	tmpFile, err := os.CreateTemp("", "assignment-audit-verify-*.db")
	if err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: create temp db: %w", err)
	}
	dbPath := tmpFile.Name()
	if _, err := tmpFile.Write(dbBytes); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(dbPath)
		return fmt.Errorf("evaluation: verify assignment audit slice: write temp db: %w", err)
	}
	_ = tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), slog.Default())
	if err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: open db: %w", err)
	}
	defer db.Close()

	var headerJSON string
	if err := db.QueryRow(`SELECT header_json FROM vault_header WHERE id = 1`).Scan(&headerJSON); err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: read vault header: %w", err)
	}
	var header vault.VaultHeader
	if err := json.Unmarshal([]byte(headerJSON), &header); err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: parse vault header: %w", err)
	}
	dek, err := header.UnwrapDEK(privateKey)
	if err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: unwrap dek: %w", err)
	}
	defer vault.SecureZero(dek)

	rows, err := db.Query(`
		SELECT id, timestamp, type, content_text
		FROM events
		ORDER BY timestamp ASC, id ASC
	`)
	if err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: list events: %w", err)
	}
	defer rows.Close()

	type storedEvent struct {
		id        int64
		timestamp string
		eventType string
		payload   []byte
	}
	events := make([]storedEvent, 0)
	for rows.Next() {
		var event storedEvent
		var encrypted []byte
		if err := rows.Scan(&event.id, &event.timestamp, &event.eventType, &encrypted); err != nil {
			return fmt.Errorf("evaluation: verify assignment audit slice: scan event: %w", err)
		}
		plaintext, err := decryptAssignmentAuditPayload(dek, encrypted)
		if err != nil {
			return fmt.Errorf("evaluation: verify assignment audit slice: decrypt event %d: %w", event.id, err)
		}
		if err := validateAssignmentAuditPayload(plaintext); err != nil {
			return fmt.Errorf("evaluation: verify assignment audit slice: payload %d: %w", event.id, err)
		}
		event.payload = plaintext
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("evaluation: verify assignment audit slice: iterate events: %w", err)
	}

	commitments, err := listAssignmentAuditCommitments(db)
	if err != nil {
		return err
	}
	if len(commitments) != len(events) {
		return fmt.Errorf("evaluation: verify assignment audit slice: commitment count mismatch")
	}
	priorHash := ""
	for i, event := range events {
		digest := assignmentAuditEventDigest(event.eventType, event.timestamp, event.payload)
		commitment := commitments[i]
		if commitment.priorHash != priorHash || commitment.eventDigest != digest {
			return fmt.Errorf("evaluation: verify assignment audit slice: commitment mismatch at event %d", event.id)
		}
		if commitment.hash != assignmentAuditCommitmentHash(priorHash, digest) {
			return fmt.Errorf("evaluation: verify assignment audit slice: recomputed hash mismatch at event %d", event.id)
		}
		priorHash = commitment.hash
	}
	return nil
}

func normalizeAssignmentAuditEvents(events []AssignmentAuditSliceEvent) ([]AssignmentAuditSliceEvent, error) {
	normalized := make([]AssignmentAuditSliceEvent, 0, len(events))
	for _, event := range events {
		if event.Type == "" || event.Timestamp == "" || len(event.Payload) == 0 {
			return nil, fmt.Errorf("evaluation: build assignment audit slice: incomplete event: %w", constants.ErrMissingRequiredField)
		}
		if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
			if _, err := time.Parse(time.RFC3339, event.Timestamp); err != nil {
				return nil, fmt.Errorf("evaluation: build assignment audit slice: invalid timestamp: %w", constants.ErrEvidenceArtifactMalformed)
			}
		}
		payload := append([]byte(nil), event.Payload...)
		if err := validateAssignmentAuditPayload(payload); err != nil {
			return nil, err
		}
		normalized = append(normalized, AssignmentAuditSliceEvent{
			Type:      event.Type,
			Timestamp: event.Timestamp,
			Payload:   payload,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i].Timestamp
		right := normalized[j].Timestamp
		if left == right {
			return normalized[i].Type < normalized[j].Type
		}
		return left < right
	})
	return normalized, nil
}

func validateAssignmentAuditPayload(payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("evaluation: assignment audit payload: %w", constants.ErrMissingRequiredField)
	}
	if err := publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, payload); err != nil {
		return fmt.Errorf("evaluation: assignment audit payload: %w", err)
	}
	fields, err := decodeAssignmentAuditObject(payload)
	if err != nil {
		return fmt.Errorf("evaluation: assignment audit payload: %w", err)
	}
	for _, key := range assignmentAuditProhibitedKeys {
		if _, ok := fields[key]; ok {
			return fmt.Errorf("evaluation: assignment audit payload: prohibited field %q: %w", key, constants.ErrEvidenceArtifactMalformed)
		}
	}
	return nil
}

func decodeAssignmentAuditObject(payload []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func initAssignmentAuditSchema(db *sqliteutil.DB) error {
	_, err := db.Exec(assignmentAuditSchema)
	return err
}

const assignmentAuditSchema = `
CREATE TABLE IF NOT EXISTS vault_header (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	header_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	title TEXT,
	session_type TEXT NOT NULL DEFAULT 'app',
	created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	user_identity TEXT
);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	operator_session_id TEXT,
	timestamp TEXT NOT NULL,
	type TEXT NOT NULL,
	content_text BLOB,
	command_raw TEXT,
	command_exit_code INTEGER,
	command_stdout BLOB,
	command_stderr BLOB,
	execution_duration_ms INTEGER,
	stored_locally INTEGER DEFAULT 1,
	stdout_truncated INTEGER DEFAULT 0,
	stderr_truncated INTEGER DEFAULT 0,
	encrypted INTEGER DEFAULT 0,
	FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS commitment_ledger (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	transaction_id TEXT NOT NULL,
	transaction_hash TEXT NOT NULL,
	prior_commitment_hash TEXT NOT NULL,
	committed_at_unix_ms INTEGER NOT NULL,
	hash TEXT NOT NULL,
	attestation_json TEXT NOT NULL,
	event_digest TEXT NOT NULL,
	UNIQUE(hash)
);

CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_commitment_ledger_committed_at ON commitment_ledger(committed_at_unix_ms);
`

func encryptAssignmentAuditPayload(dek, payload []byte) ([]byte, error) {
	nonce, err := vault.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("evaluation: encrypt assignment audit payload: %w", err)
	}
	ciphertext, err := vault.EncryptAESGCM(dek, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("evaluation: encrypt assignment audit payload: %w", err)
	}
	result := make([]byte, vault.NonceSize+len(ciphertext))
	copy(result[:vault.NonceSize], nonce)
	copy(result[vault.NonceSize:], ciphertext)
	return result, nil
}

func decryptAssignmentAuditPayload(dek, encrypted []byte) ([]byte, error) {
	if len(encrypted) < vault.NonceSize {
		return nil, constants.ErrVaultCiphertextTooShort
	}
	nonce := encrypted[:vault.NonceSize]
	ciphertext := encrypted[vault.NonceSize:]
	plaintext, err := vault.DecryptAESGCM(dek, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

type assignmentAuditCommitment struct {
	priorHash   string
	eventDigest string
	hash        string
}

func appendAssignmentAuditCommitment(db *sqliteutil.DB, priorHash string, eventID int64, event AssignmentAuditSliceEvent) (string, error) {
	digest := assignmentAuditEventDigest(event.Type, event.Timestamp, event.Payload)
	hash := assignmentAuditCommitmentHash(priorHash, digest)
	attestation := map[string]string{
		"event_id":                fmt.Sprintf("%d", eventID),
		"event_digest":            digest,
		"prior_commitment_hash":   priorHash,
		"hash":                    hash,
		"type":                    event.Type,
		"timestamp":               event.Timestamp,
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		return "", fmt.Errorf("evaluation: build assignment audit slice: marshal attestation: %w", err)
	}
	committedAt, err := assignmentAuditCommittedAt(event.Timestamp)
	if err != nil {
		return "", err
	}
	transactionID := fmt.Sprintf("assignment-audit-event-%d", eventID)
	if _, err := db.Exec(`
		INSERT INTO commitment_ledger (
			transaction_id, transaction_hash, prior_commitment_hash,
			committed_at_unix_ms, hash, attestation_json, event_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, transactionID, digest, priorHash, committedAt, hash, string(attestationJSON), digest); err != nil {
		return "", fmt.Errorf("evaluation: build assignment audit slice: write commitment: %w", err)
	}
	return hash, nil
}

func listAssignmentAuditCommitments(db *sqliteutil.DB) ([]assignmentAuditCommitment, error) {
	rows, err := db.Query(`
		SELECT prior_commitment_hash, event_digest, hash
		FROM commitment_ledger
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("evaluation: verify assignment audit slice: list commitments: %w", err)
	}
	defer rows.Close()
	commitments := make([]assignmentAuditCommitment, 0)
	for rows.Next() {
		var commitment assignmentAuditCommitment
		if err := rows.Scan(&commitment.priorHash, &commitment.eventDigest, &commitment.hash); err != nil {
			return nil, fmt.Errorf("evaluation: verify assignment audit slice: scan commitment: %w", err)
		}
		commitments = append(commitments, commitment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evaluation: verify assignment audit slice: iterate commitments: %w", err)
	}
	return commitments, nil
}

func assignmentAuditEventDigest(eventType, timestamp string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(eventType))
	h.Write([]byte{0})
	h.Write([]byte(timestamp))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func assignmentAuditCommitmentHash(priorHash, eventDigest string) string {
	h := sha256.New()
	h.Write([]byte(priorHash))
	h.Write([]byte(eventDigest))
	return hex.EncodeToString(h.Sum(nil))
}

func assignmentAuditCommittedAt(timestamp string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return 0, fmt.Errorf("evaluation: build assignment audit slice: parse timestamp: %w", err)
		}
	}
	return parsed.UTC().UnixMilli(), nil
}

func decodeAssignmentAuditVaultKey(vaultKey []byte) ([]byte, error) {
	keyHex := strings.TrimSpace(string(vaultKey))
	privateKey, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("evaluation: verify assignment audit slice: %w", constants.ErrVaultKeyDecodeFailed)
	}
	if len(privateKey) != vault.KeySize {
		return nil, fmt.Errorf("evaluation: verify assignment audit slice: %w", constants.ErrVaultKeyInvalidSize)
	}
	return privateKey, nil
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// deriveAssignmentAuditVaultKey derives a stable vault key from assignment
// events so restore republish produces the same content-addressed artifacts.
func deriveAssignmentAuditVaultKey(assignmentID string, events []AssignmentAuditSliceEvent) []byte {
	h := sha256.New()
	h.Write([]byte(assignmentID))
	for _, event := range events {
		h.Write([]byte(event.Type))
		h.Write([]byte(event.Timestamp))
		h.Write(event.Payload)
	}
	digest := h.Sum(nil)
	key := make([]byte, vault.KeySize)
	copy(key, digest)
	for offset := len(digest); offset < vault.KeySize; offset += len(digest) {
		next := sha256.Sum256(digest)
		digest = next[:]
		copy(key[offset:], digest[:min(len(digest), vault.KeySize-offset)])
	}
	return key
}

// AssignmentAuditEvidenceBindings returns the public evidence bindings for a
// built audit slice package.
func AssignmentAuditEvidenceBindings(artifacts AssignmentAuditSliceArtifacts) []*evalv1.PublicEvidenceBinding {
	return AssignmentAuditEvidenceBindingsFromHashes(artifacts.DatabaseSHA256, artifacts.VaultKeySHA256)
}

// AssignmentAuditEvidenceBindingsFromHashes returns public evidence bindings
// from previously published content-addressed proof hashes.
func AssignmentAuditEvidenceBindingsFromHashes(databaseSHA256, vaultKeySHA256 string) []*evalv1.PublicEvidenceBinding {
	if databaseSHA256 == "" || vaultKeySHA256 == "" {
		return nil
	}
	return []*evalv1.PublicEvidenceBinding{
		{Sha256: databaseSHA256, SchemaRef: assignmentAuditSchemaRef, Kind: AssignmentAuditSliceKind},
		{Sha256: vaultKeySHA256, SchemaRef: assignmentAuditSchemaRef, Kind: AssignmentAuditVaultKeyKind},
	}
}

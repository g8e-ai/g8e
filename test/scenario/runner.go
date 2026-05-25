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

//go:build integration

package scenario

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// Test placeholder constants for detecting intentionally corrupted test fixtures
const (
	testPlaceholderBadID               = "wrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongid"
	testPlaceholderBadHash             = "wronghashwronghashwronghashwronghashwronghashwronghashwronghash"
	testPlaceholderBadSignature        = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	testPlaceholderInvalidL3ClientData = "invalidclientdata"
	testPlaceholderInvalidL3AuthData   = "invalidauthdata"
	testPlaceholderInvalidL3Signature  = "invalidsignature"
)

// InMemoryReplayStore is an in-memory replay store for testing.
type InMemoryReplayStore struct {
	nonces map[string]time.Time
	mu     sync.RWMutex
}

func NewInMemoryReplayStore() *InMemoryReplayStore {
	return &InMemoryReplayStore{
		nonces: make(map[string]time.Time),
	}
}

func (s *InMemoryReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.nonces[nonce]; exists {
		return true, nil
	}
	s.nonces[nonce] = expiresAt
	return false, nil
}

func (s *InMemoryReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	return s.CheckAndSetNonce(nonce, expiresAt)
}

func (s *InMemoryReplayStore) FinalizeNonce(nonce string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nonces, nonce)
	return nil
}

func (s *InMemoryReplayStore) ReleaseNonce(nonce string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nonces, nonce)
	return nil
}

// Clear removes all nonces from the store (useful for test isolation)
func (s *InMemoryReplayStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nonces = make(map[string]time.Time)
}

// Result represents the outcome of submitting a scenario through the admission path.
type Result struct {
	Receipt *operatorv1.ActionReceipt
	Error   error
}

// OperatorGate is the real admission path integration for a given governance mode.
type OperatorGate struct {
	verifier    *governance.L4Warden
	actuator    *governance.L5Actuator
	replayStore *InMemoryReplayStore
	clock       system.Clock
	mode        Mode
	logger      *slog.Logger
	db          *sqliteutil.DB
	auditVault  *storage.AuditVaultService
	tempDir     string // Temporary directory for audit vault git ledger
}

// NewOperatorGate creates an OperatorGate for the given mode with injectable dependencies.
// If t is non-nil, it creates a temporary directory for the audit vault git ledger.
func NewOperatorGate(mode Mode, clock system.Clock, stateRoot string, trustedSigners map[string]ed25519.PublicKey, db *sqliteutil.DB, t *testing.T) (*OperatorGate, error) {
	// Create a discard logger for testing to avoid nil pointer issues
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	replayStore := NewInMemoryReplayStore()
	stateRootProvider := &governance.SimpleStateRootProvider{Root: stateRoot}
	signerStore := &governance.SimpleSignerStore{Signers: trustedSigners}
	appPolicyStore := &governance.SimpleAppPolicyStore{Policies: make(map[string]*models.AppPolicy)}

	// Create a mock L3 notary that accepts all proofs for testing
	l3Notary := &mockL3Notary{}

	// Create a mock execution handler
	execHandler := &mockExecutionHandler{}

	// Generate signing key for the actuator
	actuatorPub, actuatorPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate actuator signing key: %w", err)
	}
	actuatorKeyID := hex.EncodeToString(actuatorPub)

	// Create transaction verifier
	knownActionTypes := constants.AllActionTypes()
	doctrine := governance.NewL1Doctrine()
	verifier := governance.NewL4Warden(
		logger,
		replayStore,
		stateRootProvider,
		signerStore,
		appPolicyStore,
		l3Notary,
		doctrine,
		knownActionTypes,
		string(mode),
		clock,
	)

	// Create real audit store from database (no mocks principle)
	var auditStore governance.TransactionAuditStore
	if db != nil {
		auditStore = &testAuditStore{db: db}
	}

	// Create audit vault with temporary directory for git ledger
	var auditVault *storage.AuditVaultService
	var tempDir string
	if t != nil {
		tempDir = t.TempDir()
		auditVaultConfig := storage.DefaultAuditVaultConfig()
		auditVaultConfig.DataDir = tempDir
		auditVaultConfig.DBPath = "audit_vault.db"
		auditVaultConfig.LedgerDir = "ledger"
		auditVaultConfig.GitPath = "embedded" // Use native go-git
		auditVaultConfig.Enabled = true

		var err error
		auditVault, err = storage.NewAuditVaultService(auditVaultConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit vault: %w", err)
		}
	}

	// Create actuator with audit vault
	actuator := &governance.L5Actuator{
		Logger:            logger,
		SignerStore:       signerStore,
		ExecutionHandler:  execHandler,
		L3Notary:          l3Notary,
		StateRootProvider: stateRootProvider,
		Posture:           verifier.Posture(),
		SigningKey:        actuatorPriv,
		KeyID:             actuatorKeyID,
		AuditStore:        auditStore,
		AuditVault:        auditVault,
	}

	return &OperatorGate{
		verifier:    verifier,
		actuator:    actuator,
		replayStore: replayStore,
		clock:       clock,
		mode:        mode,
		logger:      logger,
		db:          db,
		auditVault:  auditVault,
		tempDir:     tempDir,
	}, nil
}

// Close cleans up resources including the audit vault.
func (g *OperatorGate) Close() error {
	if g.auditVault != nil {
		// Clean up temporary directory
		if g.tempDir != "" {
			os.RemoveAll(g.tempDir)
		}
	}
	return nil
}

// normalizeEnvelope dynamically computes hashes and signatures.
// For "bad" tests (wrong id/hash), it computes the correct value then corrupts it.
// For "good" tests, it computes and sets the correct values.
// Mode-aware: only adds L2 for consensus/notary modes, not doctrine.
func (g *OperatorGate) normalizeEnvelope(envelope *commonv1.GovernanceEnvelope) error {
	// Use the test private key for signing
	privKeyHex := "c847d8625a1d1be737b8c86012ef1ceb7cfe1c2f5bed5115b90b490c55600502797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd"
	privKeyBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode test private key: %w", err)
	}
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)
	keyID := hex.EncodeToString(pubKey)

	// Compute the correct hash from the envelope content
	correctHash, err := uap.GenerateMessageID(envelope)
	if err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}

	// Set id: if empty use correct hash, if it's a "bad" placeholder keep it
	if envelope.Id == "" {
		envelope.Id = correctHash
	} else if envelope.Id == testPlaceholderBadID {
		// Keep the bad id for bad_integrity test
	}

	// Set transaction_hash: if empty use correct hash, if it's a "bad" placeholder keep it
	if envelope.TransactionHash == "" {
		envelope.TransactionHash = correctHash
	} else if envelope.TransactionHash == testPlaceholderBadHash {
		// Keep the bad hash for hash_mismatch test
	}

	// Initialize governance if nil
	if envelope.Governance == nil {
		envelope.Governance = &commonv1.GovernanceMetadata{}
	}

	// Handle L2 signature - compute if L2 metadata is present
	// Normalization is mode-agnostic: if L2 exists, compute the signature.
	// Mode-specific enforcement (whether L2 is required) is handled by the verifier.
	if envelope.Governance.L2 != nil {
		// Compute correct signature
		correctSig := hex.EncodeToString(ed25519.Sign(privKey, []byte(correctHash+"|true")))

		// If signature is empty, compute it
		if envelope.Governance.L2.TribunalSignature == "" {
			envelope.Governance.L2.TribunalSignature = correctSig
		} else if envelope.Governance.L2.TribunalSignature == testPlaceholderBadSignature {
			// Keep the forged signature for l2_invalid test
			// Recompute hash to match the forged signature content
			envelope.Id = correctHash
			envelope.TransactionHash = correctHash
		} else if envelope.Governance.L2.KeyId == keyID {
			// Signature is set with the test key_id - recompute to ensure correctness
			envelope.Governance.L2.TribunalSignature = correctSig
		}
		// else: signature is set with a different key_id (e.g., forge_signature fixture)
		// - preserve it to test unknown signer rejection

		// Set key ID if empty
		if envelope.Governance.L2.KeyId == "" {
			envelope.Governance.L2.KeyId = keyID
		}
	}

	return nil
}

// Submit sends a raw intent through the real admission path and returns the result.
func (g *OperatorGate) Submit(ctx context.Context, intent json.RawMessage) Result {
	// Decode the UAP envelope using protojson
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(intent, &envelope); err != nil {
		return Result{Error: fmt.Errorf("failed to unmarshal UAP envelope: %w", err)}
	}

	// Normalize the envelope (compute hash, sign L2 if present)
	if err := g.normalizeEnvelope(&envelope); err != nil {
		return Result{Error: fmt.Errorf("failed to normalize envelope: %w", err)}
	}

	// Verify the envelope through the real transaction verifier
	verified, err := g.verifier.VerifyEnvelope(ctx, &envelope)
	if err != nil {
		return Result{Error: err}
	}

	// Execute through the real actuator
	receipt, err := g.actuator.Execute(ctx, verified, nil)
	if err != nil {
		return Result{Error: err}
	}

	return Result{Receipt: receipt}
}

// mockL3Notary is a mock L3 notary that rejects proofs with invalid data for testing.
type mockL3Notary struct{}

func (m *mockL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	// Reject proofs with invalid placeholder data
	if proof != nil {
		if proof.ClientDataJson == testPlaceholderInvalidL3ClientData ||
			proof.AuthenticatorData == testPlaceholderInvalidL3AuthData ||
			proof.Signature == testPlaceholderInvalidL3Signature {
			return false, nil
		}
	}
	return true, nil
}

// mockExecutionHandler is a mock execution handler that returns success.
type mockExecutionHandler struct{}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
	return "mock execution succeeded", nil
}

// testAuditStore implements TransactionAuditStore using a real database.
// This follows the "no mocks" principle for integration tests.
type testAuditStore struct {
	db *sqliteutil.DB
}

func (s *testAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)
	_, err := s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(collection, id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		collection, id, string(data), nowStr, nowStr,
	)
	return err
}

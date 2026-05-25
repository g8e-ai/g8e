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

package scenario

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/system"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
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
	verifier    *governance.TransactionVerifier
	actuator    *governance.Actuator
	replayStore *InMemoryReplayStore
	clock       system.Clock
	mode        Mode
	logger      *slog.Logger
}

// NewOperatorGate creates an OperatorGate for the given mode with injectable dependencies.
func NewOperatorGate(mode Mode, clock system.Clock, stateRoot string, trustedSigners map[string]ed25519.PublicKey) (*OperatorGate, error) {
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
	verifier := governance.NewTransactionVerifier(
		logger,
		replayStore,
		stateRootProvider,
		signerStore,
		appPolicyStore,
		l3Notary,
		knownActionTypes,
		string(mode),
		clock,
	)

	// Create actuator
	actuator := &governance.Actuator{
		Logger:            logger,
		SignerStore:       signerStore,
		ExecutionHandler:  execHandler,
		L3Notary:          l3Notary,
		StateRootProvider: stateRootProvider,
		Posture:           verifier.Posture(),
		SigningKey:        actuatorPriv,
		KeyID:             actuatorKeyID,
	}

	return &OperatorGate{
		verifier:    verifier,
		actuator:    actuator,
		replayStore: replayStore,
		clock:       clock,
		mode:        mode,
		logger:      logger,
	}, nil
}

// Submit sends a raw intent through the real admission path and returns the result.
func (g *OperatorGate) Submit(ctx context.Context, intent json.RawMessage) Result {
	// Decode the UAP envelope using protojson
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(intent, &envelope); err != nil {
		return Result{Error: fmt.Errorf("failed to unmarshal UAP envelope: %w", err)}
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

// mockL3Notary is a mock L3 notary that accepts all proofs for testing.
type mockL3Notary struct{}

func (m *mockL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return true, nil
}

// mockExecutionHandler is a mock execution handler that returns success.
type mockExecutionHandler struct{}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
	return "mock execution succeeded", nil
}

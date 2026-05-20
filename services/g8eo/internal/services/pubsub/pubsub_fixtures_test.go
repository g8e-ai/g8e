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

package pubsub

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"crypto/ed25519"
	"crypto/rand"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
)

// Mock governance dependencies for testing
type mockReplayStore struct{}

func (m *mockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	return false, nil // Never replay in tests
}

type mockStateRootProvider struct{}

func (m *mockStateRootProvider) GetCurrentStateRoot() (string, error) {
	return "test-state-root", nil
}

type mockTransactionAudit struct{}

func (m *mockTransactionAudit) DocSet(collection, id string, data json.RawMessage) error {
	return nil
}

type mockL3Verifier struct{}

func (m *mockL3Verifier) VerifyL3Proof(userID, transactionHash string, proof *commonv1.L3Proof) (bool, error) {
	return true, nil // Always verify in tests
}

type pubsubFixture struct {
	Cfg        *config.Config
	Logger     *slog.Logger
	DB         *MockOperatorPubSubClient
	Svc        *PubSubCommandService
	SignerPriv ed25519.PrivateKey
	SignerPub  ed25519.PublicKey
}

func newPubsubFixture(t *testing.T) *pubsubFixture {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockOperatorPubSubClient()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signerStore := &governance.SimpleSignerStore{
		Signers: map[string]ed25519.PublicKey{
			"test-key": pub,
		},
	}

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:            cfg,
		Logger:            logger,
		PubSubClient:      db,
		ReplayStore:       &mockReplayStore{},
		StateRootProvider: &mockStateRootProvider{},
		TransactionAudit:  &mockTransactionAudit{},
		L3Verifier:        &mockL3Verifier{},
		SignerStore:       signerStore,
		WardenSigningKey:  priv,
		WardenKeyID:       "warden-key",
	})
	if err != nil {
		t.Fatalf("failed to create PubSubCommandService: %v", err)
	}

	return &pubsubFixture{
		Cfg:        cfg,
		Logger:     logger,
		DB:         db,
		Svc:        svc,
		SignerPriv: priv,
		SignerPub:  pub,
	}
}

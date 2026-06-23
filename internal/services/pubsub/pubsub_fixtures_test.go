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
	"context"
	"log/slog"
	"testing"

	"crypto/ed25519"
	"crypto/rand"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// simpleTribunalStore is an in-memory governance.TribunalStore for unit tests.
type simpleTribunalStore struct {
	tribunals map[string]*models.TribunalPolicy
}

func (s *simpleTribunalStore) GetTribunal(id string) (*models.TribunalPolicy, error) {
	if s.tribunals == nil {
		return nil, nil
	}
	return s.tribunals[id], nil
}

// testTribunalStore returns a TribunalStore with a single 1-of-1 "test-tribunal"
// policy whose sole member is "test-key". This mirrors the single trusted signer
// registered by the pubsub test fixtures so L4 quorum verification can pass.
func testTribunalStore() governance.TribunalStore {
	return &simpleTribunalStore{
		tribunals: map[string]*models.TribunalPolicy{
			"test-tribunal": {
				ID:              "test-tribunal",
				MemberAppIDs:    []string{"test-key"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
		},
	}
}

type pubsubFixture struct {
	Cfg        *config.Config
	Logger     *slog.Logger
	DB         *MockOperatorPubSubClient
	Svc        *OperatorPubSubService
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

	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       db,
		ReplayStore:        &testutil.MockReplayStore{},
		StateRootProvider:  testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:   &testutil.MockTransactionAudit{},
		L3Notary:           &testutil.MockL3Notary{},
		SignerStore:        signerStore,
		TribunalStore:      testTribunalStore(),
		ActuatorSigningKey: priv,
		ActuatorKeyID:      "Actuator-key",
	})
	if err != nil {
		t.Fatalf("failed to create OperatorPubSubService: %v", err)
	}

	// Set up a mock actuator for tests that require execution
	mockHandler := &mockExecutionHandler{
		ExecuteVerifiedTransactionFunc: func(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
			return "test-receipt-id", nil
		},
	}
	mockActuator := &governance.L5Actuator{
		Logger:           logger,
		ExecutionHandler: mockHandler,
		SigningKey:       priv,
		KeyID:            "Actuator-key",
	}
	svc.SetActuator(mockActuator)

	return &pubsubFixture{
		Cfg:        cfg,
		Logger:     logger,
		DB:         db,
		Svc:        svc,
		SignerPriv: priv,
		SignerPub:  pub,
	}
}

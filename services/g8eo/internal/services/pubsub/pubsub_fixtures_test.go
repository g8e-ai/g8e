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
	"log/slog"
	"testing"

	"crypto/ed25519"
	"crypto/rand"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
)

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
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       db,
		ReplayStore:        &testutil.MockReplayStore{},
		StateRootProvider:  testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:   &testutil.MockTransactionAudit{},
		L3Notary:           &testutil.MockL3Notary{},
		SignerStore:        signerStore,
		ActuatorSigningKey: priv,
		ActuatorKeyID:      "Actuator-key",
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

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
	"encoding/hex"
	"fmt"
	"log/slog"
	"testing"

	"crypto/ed25519"
	"crypto/rand"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/governance/governancetest"
	pubsubtest "github.com/g8e-ai/g8e/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// signL2Vote creates an L2Vote with a signature derived from the Decision field,
// preventing decision/signature mismatch bugs where the signed string and the
// Decision field diverge.
func signL2Vote(privKey ed25519.PrivateKey, keyID, hash string, decision bool) *commonv1.L2Vote {
	sig := ed25519.Sign(privKey, []byte(fmt.Sprintf("%s|%v", hash, decision)))
	return &commonv1.L2Vote{
		SignerKeyId:        keyID,
		ConsensusSignature: hex.EncodeToString(sig),
		Decision:           decision,
	}
}

// pubsubTestConsensusStoreAdapter wraps a governancetest.SimpleConsensusStore and
// adapts it to satisfy governance.L2ConsensusPolicyStore for pubsub test code.
// This is the test-only replacement for the removed production ConsensusStoreAdapter.
type pubsubTestConsensusStoreAdapter struct {
	Inner *governancetest.SimpleConsensusStore
}

func (a *pubsubTestConsensusStoreAdapter) GetConsensusPolicy(id string) (*governance.L2ConsensusPolicy, error) {
	policy, err := a.Inner.GetConsensus(id)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil
	}
	return &governance.L2ConsensusPolicy{
		MemberKeyIDs:    policy.MemberAppIDs,
		Quorum:          policy.Quorum,
		RequireDistinct: policy.RequireDistinct,
		Enabled:         policy.Enabled,
	}, nil
}

// testConsensusStore returns an L2ConsensusPolicyStore with a single 1-of-1
// "test-consensus" policy whose sole member is "test-key". This mirrors the
// single trusted signer registered by the pubsub test fixtures so L4 quorum
// verification can pass.
func testConsensusStore() governance.L2ConsensusPolicyStore {
	return &pubsubTestConsensusStoreAdapter{Inner: &governancetest.SimpleConsensusStore{
		Consensus: map[string]*models.ConsensusPolicy{
			"test-consensus": {
				ID:              "test-consensus",
				MemberAppIDs:    []string{"test-key"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
		},
	}}
}

type pubsubFixture struct {
	Cfg        *config.Config
	Logger     *slog.Logger
	DB         *pubsubtest.MockOperatorPubSubClient
	Svc        *OperatorPubSubService
	SignerPriv ed25519.PrivateKey
	SignerPub  ed25519.PublicKey
}

func newPubsubFixture(t *testing.T) *pubsubFixture {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := pubsubtest.NewMockOperatorPubSubClient()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signerStore := &governance.FailClosedSignerStore{
		Signers: map[string]ed25519.PublicKey{
			"test-key": pub,
		},
	}

	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       db,
		ActuatorSigningKey: priv,
		ActuatorKeyID:      "Actuator-key",
	}, GovernanceDeps{
		ReplayStore:          &testutil.MockReplayStore{},
		StateRootProvider:    testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:     &testutil.MockTransactionAudit{},
		L3Notary:             &testutil.MockL3Notary{},
		SignerStore:          signerStore,
		ConsensusPolicyStore: testConsensusStore(),
	})
	if err != nil {
		t.Fatalf("failed to create OperatorPubSubService: %v", err)
	}

	// Set up a mock actuator for tests that require execution
	mockHandler := &mockExecutionHandler{
		ExecuteVerifiedTransactionFunc: func(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
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

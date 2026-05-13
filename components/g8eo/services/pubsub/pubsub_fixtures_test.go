package pubsub

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
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

func (m *mockL3Verifier) VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error) {
	return true, nil // Always verify in tests
}

type pubsubFixture struct {
	Cfg    *config.Config
	Logger *slog.Logger
	DB     *MockOperatorPubSubClient
	Svc    *PubSubCommandService
}

func newPubsubFixture(t *testing.T) *pubsubFixture {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockOperatorPubSubClient()

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:            cfg,
		Logger:            logger,
		PubSubClient:      db,
		ReplayStore:       &mockReplayStore{},
		StateRootProvider: &mockStateRootProvider{},
		TransactionAudit:  &mockTransactionAudit{},
		L3Verifier:        &mockL3Verifier{},
	})
	if err != nil {
		t.Fatalf("failed to create PubSubCommandService: %v", err)
	}

	return &pubsubFixture{
		Cfg:    cfg,
		Logger: logger,
		DB:     db,
		Svc:    svc,
	}
}

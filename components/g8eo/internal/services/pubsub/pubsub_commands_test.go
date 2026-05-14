package pubsub

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewPubSubCommandService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:            cfg,
			Logger:            testutil.NewTestLogger(),
			PubSubClient:      NewMockOperatorPubSubClient(),
			ReplayStore:       &mockReplayStore{},
			StateRootProvider: &mockStateRootProvider{},
			TransactionAudit:  &mockTransactionAudit{},
			L3Verifier:        &mockL3Verifier{},
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestNewPubSubCommandService_StartsWithoutTrustedSignersButRejectsL2(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.PKIDir = filepath.Join(t.TempDir(), "pki")
	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:            cfg,
		Logger:            testutil.NewTestLogger(),
		PubSubClient:      NewMockOperatorPubSubClient(),
		ReplayStore:       &mockReplayStore{},
		StateRootProvider: &mockStateRootProvider{},
		TransactionAudit:  &mockTransactionAudit{},
		L3Verifier:        &mockL3Verifier{},
	})
	require.NoError(t, err)
	require.Empty(t, svc.trustedSigners)
	require.NotNil(t, svc.transactionVerifier)

	_, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	env := unsignedSignerEnvelope(t, signerPriv)

	_, err = svc.transactionVerifier.VerifyEnvelope(env)
	require.Error(t, err)
	assert.True(t, errors.Is(err, governance.ErrL2KeyNotConfigured), "expected missing L2 key error, got %v", err)
}

func TestNewPubSubCommandService_RejectsMalformedTrustedSignerFile(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.PKIDir = filepath.Join(t.TempDir(), "pki")
	signersDir := filepath.Join(cfg.PKIDir, "trusted_signers")
	require.NoError(t, os.MkdirAll(signersDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(signersDir, "bad.pub"), []byte("not-hex"), 0600))

	_, err := NewPubSubCommandService(CommandServiceConfig{
		Config:            cfg,
		Logger:            testutil.NewTestLogger(),
		PubSubClient:      NewMockOperatorPubSubClient(),
		ReplayStore:       &mockReplayStore{},
		StateRootProvider: &mockStateRootProvider{},
		TransactionAudit:  &mockTransactionAudit{},
		L3Verifier:        &mockL3Verifier{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode trusted L2 signer")
}

func unsignedSignerEnvelope(t *testing.T, signerPriv ed25519.PrivateKey) *uap.UAPEnvelope {
	t.Helper()
	req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
	payload, err := proto.Marshal(req)
	require.NoError(t, err)
	env := &uap.UAPEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        "FS_LIST",
		TargetResource:    "localhost",
		Payload:           payload,
		StateMerkleRoot:   "test-state-root",
		Nonce:             "nonce-missing-signer",
	}
	hash, err := uap.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             "missing-key",
			TribunalSignature: hex.EncodeToString(ed25519.Sign(signerPriv, []byte(hash+"|true"))),
		},
	}
	return env
}

func TestPubSubCommandService_HandleShutdownRequest_UAP(t *testing.T) {
	f := newPubsubFixture(t)
	t.Run("successful UAP shutdown", func(t *testing.T) {
		reason := "remote control"
		req := &operatorv1.ShutdownRequested{Reason: reason}
		payload, _ := proto.Marshal(req)

		msg := PubSubCommandMessage{
			ID:        "shutdown-1",
			EventType: constants.Event.Operator.ShutdownRequested,
			Payload:   payload,
		}
		f.Svc.handleShutdownRequest(msg)
		select {
		case r := <-f.Svc.ShutdownChan:
			assert.Equal(t, reason, r)
		case <-time.After(1 * time.Second):
			t.Fatal("shutdown not received")
		}
	})
}

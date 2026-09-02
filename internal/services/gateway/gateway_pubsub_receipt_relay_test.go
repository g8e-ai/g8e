// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	pubsubv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/pubsub/v1"
)

// --- stubs for receipt relay unit tests ---

// stubSignerStore implements governance.SignerStore for receipt relay tests.
type stubSignerStore struct {
	keys map[string]ed25519.PublicKey
	err  error
}

func (s *stubSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.keys == nil {
		return nil, nil
	}
	pub, ok := s.keys[keyID]
	if !ok {
		return nil, nil
	}
	return pub, nil
}

// stubReceiptRecorder implements ReceiptRecorder for receipt relay tests.
type stubReceiptRecorder struct {
	mu      sync.Mutex
	records []*models.ActionReceiptRecord
	err     error
}

func (r *stubReceiptRecorder) RecordActionReceipt(record *models.ActionReceiptRecord) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

func (r *stubReceiptRecorder) recorded() []*models.ActionReceiptRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.ActionReceiptRecord, len(r.records))
	copy(out, r.records)
	return out
}

// --- helpers ---

// newReceiptRelayTestBroker creates a GatewayWebSocketHandler wired with
// stub signer store and receipt recorder for receipt relay unit tests.
// signerPub is the operator's actuator public key registered under keyID.
func newReceiptRelayTestBroker(t *testing.T, keyID string, signerPub ed25519.PublicKey) (*GatewayWebSocketHandler, *stubSignerStore, *stubReceiptRecorder) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	signerStore := &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: signerPub}}
	recorder := &stubReceiptRecorder{}
	broker.SetReceiptRelayDeps(signerStore, recorder)
	return broker, signerStore, recorder
}

// buildSignedReceiptEnvelope builds a GovernanceEnvelope wrapping a signed
// ActionReceipt as its payload, mirroring what the operator's
// PubSubResultsService.PublishActionReceipt publishes to the receipts:
// channel. signerPriv signs the receipt via governance.CanonicalizeActionReceipt.
func buildSignedReceiptEnvelope(t *testing.T, signerPriv ed25519.PrivateKey, keyID, operatorID, operatorSessionID, actionType, targetResource, requestorUserID, actingAppID, transactionID string) ([]byte, *operatorv1.ActionReceipt) {
	t.Helper()
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    transactionID,
		TransactionHash:  "hash-" + transactionID,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "completed",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1700000000000,
		SignerKeyId:      keyID,
	}
	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	sig := ed25519.Sign(signerPriv, canonical)
	receipt.Signature = hex.EncodeToString(sig)
	var digestPayload bytes.Buffer
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], 1)
	digestPayload.Write(count[:])
	binary.BigEndian.PutUint32(count[:], uint32(len(receipt.Signature)))
	digestPayload.Write(count[:])
	digestPayload.WriteString(receipt.Signature)
	digest := sha256.Sum256(digestPayload.Bytes())
	receipt.FinalPersistenceAttestation = &operatorv1.ReceiptPersistenceAttestation{
		TransactionId:          transactionID,
		ReceiptSignatureDigest: hex.EncodeToString(digest[:]),
		PersistedAtUnixMs:      1700000000001,
		AuditRecordId:          transactionID,
		SignerKeyId:            keyID,
	}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(receipt.FinalPersistenceAttestation)
	require.NoError(t, err)
	receipt.FinalPersistenceAttestation.Signature = hex.EncodeToString(ed25519.Sign(signerPriv, attestationPayload))

	receiptBytes, err := proto.Marshal(receipt)
	require.NoError(t, err)

	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
		ActionType:        actionType,
		TargetResource:    targetResource,
		EventType:         string(constants.Event.Operator.Receipt.Recorded),
		Payload:           receiptBytes,
		RequestorUserId:   requestorUserID,
		ActingAppId:       actingAppID,
	}
	wire, err := protojson.Marshal(env)
	require.NoError(t, err)
	return wire, receipt
}

// --- relayActionReceipt table-driven tests ---

func TestRelayActionReceipt_ValidReceiptRecordedAndFannedOut(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, signerStore, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	// Register a handler to capture fan-out.
	var capturedData []byte
	unregister := broker.RegisterHandler(channel, func(_ string, data []byte) {
		capturedData = data
	})
	defer unregister()

	wire, receipt := buildSignedReceiptEnvelope(t, priv, keyID, operatorID, sessionID,
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "spiffe://g8e.local/app/g8ee", "tx-001")

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	// Verify the receipt was recorded in the audit store.
	records := recorder.recorded()
	require.Len(t, records, 1, "valid receipt must be recorded in the audit store")
	rec := records[0]
	assert.Equal(t, receipt.TransactionId, rec.TransactionID)
	assert.Equal(t, operatorID, rec.OperatorID)
	assert.Equal(t, sessionID, rec.OperatorSessionID)
	assert.Equal(t, "user-001", rec.RequestorUserID)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", rec.ActingAppID)
	assert.Equal(t, constants.ActionTypeFileEdit, rec.ActionType)
	assert.Equal(t, "/tmp/test.txt", rec.TargetResource)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, rec.Status)
	assert.Equal(t, receipt.Signature, rec.Signature)
	assert.Equal(t, keyID, rec.SignerKeyID)

	// Verify the original envelope was fanned out verbatim.
	require.NotNil(t, capturedData, "valid receipt must be fanned out to subscribers")
	assert.Equal(t, wire, capturedData, "fan-out must be the original published data, not a re-marshaled copy")

	// Signer store must have been consulted.
	_ = signerStore // used via broker wiring
}

func TestRelayActionReceipt_InvalidSignatureRejected(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, _, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	wire, _ := buildSignedReceiptEnvelope(t, priv, keyID, operatorID, sessionID,
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-002")

	// Tamper with the wire data: re-sign with a different key so the
	// signature no longer matches the registered public key.
	_, otherPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	env := &commonv1.GovernanceEnvelope{}
	require.NoError(t, protojson.Unmarshal(wire, env))
	rcpt := &operatorv1.ActionReceipt{}
	require.NoError(t, proto.Unmarshal(env.Payload, rcpt))
	canonical, err := governance.CanonicalizeActionReceipt(rcpt)
	require.NoError(t, err)
	badSig := ed25519.Sign(otherPriv, canonical)
	rcpt.Signature = hex.EncodeToString(badSig)
	badPayload, err := proto.Marshal(rcpt)
	require.NoError(t, err)
	env.Payload = badPayload
	badWire, err := protojson.Marshal(env)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    badWire,
	})

	assert.Empty(t, recorder.recorded(), "invalid signature must not be recorded")
	assert.False(t, called, "invalid signature must not be fanned out")
}

func TestRelayActionReceipt_MissingOrInvalidPersistenceAttestationRejected(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	const keyID = "actuator-key-persistence"
	const operatorID = "op-persistence"
	const sessionID = "session-persistence"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	tests := []struct {
		name   string
		mutate func(*operatorv1.ActionReceipt)
	}{
		{name: "missing attestation", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation = nil
		}},
		{name: "invalid attestation signature", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.Signature = "invalid"
		}},
		{name: "mismatched receipt digest", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.ReceiptSignatureDigest = "different-digest"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, _, recorder := newReceiptRelayTestBroker(t, keyID, publicKey)
			handler := newOperatorSessionHandler(broker, operatorID)
			var called bool
			unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
			t.Cleanup(unregister)
			wire, _ := buildSignedReceiptEnvelope(t, privateKey, keyID, operatorID, sessionID,
				string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-persistence", "app-persistence", "tx-persistence")
			envelope := &commonv1.GovernanceEnvelope{}
			require.NoError(t, protojson.Unmarshal(wire, envelope))
			receipt := &operatorv1.ActionReceipt{}
			require.NoError(t, proto.Unmarshal(envelope.Payload, receipt))
			tt.mutate(receipt)
			envelope.Payload, err = proto.Marshal(receipt)
			require.NoError(t, err)
			wire, err = protojson.Marshal(envelope)
			require.NoError(t, err)

			handler.handleAction(&pubsubv1.PubSubMessage{Action: constants.PubSubActionPublish, Channel: channel, Data: wire})

			assert.Empty(t, recorder.recorded())
			assert.False(t, called)
		})
	}
}

func TestRelayActionReceipt_UnknownSignerKeyRejected(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	// Register a different key than the one that signs the receipt.
	broker, _, recorder := newReceiptRelayTestBroker(t, "registered-key", pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	// Sign with "unknown-key" which is not in the signer store.
	wire, _ := buildSignedReceiptEnvelope(t, priv, "unknown-key", operatorID, sessionID,
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-003")

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	assert.Empty(t, recorder.recorded(), "unknown signer key must not be recorded")
	assert.False(t, called, "unknown signer key must not be fanned out")
}

func TestRelayActionReceipt_ChannelEnvelopeIdentityMismatchRejected(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, _, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	// Publish to op-001's channel but the envelope claims op-002.
	channel := pubsub.ReceiptsChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	wire, _ := buildSignedReceiptEnvelope(t, priv, keyID, "op-002", "sess-002",
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-004")

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	assert.Empty(t, recorder.recorded(), "channel/envelope identity mismatch must not be recorded")
	assert.False(t, called, "channel/envelope identity mismatch must not be fanned out")
}

func TestRelayActionReceipt_MalformedProtoJSONRejected(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, _, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	channel := pubsub.ReceiptsChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    []byte("{invalid json"),
	})

	assert.Empty(t, recorder.recorded(), "malformed protojson must not be recorded")
	assert.False(t, called, "malformed protojson must not be fanned out")
}

func TestRelayActionReceipt_MalformedPayloadRejected(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, _, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	// Build an envelope with a valid identity but a malformed payload
	// (not a valid ActionReceipt proto).
	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		OperatorId:        operatorID,
		OperatorSessionId: sessionID,
		ActionType:        string(constants.ActionTypeFileEdit),
		EventType:         string(constants.Event.Operator.Receipt.Recorded),
		Payload:           []byte("not-a-valid-protobuf"),
	}
	wire, err := protojson.Marshal(env)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	assert.Empty(t, recorder.recorded(), "malformed payload must not be recorded")
	assert.False(t, called, "malformed payload must not be fanned out")
}

func TestRelayActionReceipt_MissingPayloadRejected(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	broker, _, recorder := newReceiptRelayTestBroker(t, keyID, pub)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	// Envelope with no payload bytes.
	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		OperatorId:        operatorID,
		OperatorSessionId: sessionID,
		ActionType:        string(constants.ActionTypeFileEdit),
		EventType:         string(constants.Event.Operator.Receipt.Recorded),
	}
	wire, err := protojson.Marshal(env)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	assert.Empty(t, recorder.recorded(), "missing payload must not be recorded")
	assert.False(t, called, "missing payload must not be fanned out")
}

func TestRelayActionReceipt_RelayDisabledWhenDepsNotConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	// Do NOT call SetReceiptRelayDeps — relay is disabled.
	handler := newOperatorSessionHandler(broker, "op-001")

	channel := pubsub.ReceiptsChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	wire, _ := buildSignedReceiptEnvelope(t, priv, "actuator-key-1", "op-001", "sess-001",
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-005")
	_ = pub

	assert.NotPanics(t, func() {
		handler.handleAction(&pubsubv1.PubSubMessage{
			Action:  constants.PubSubActionPublish,
			Channel: channel,
			Data:    wire,
		})
	})
	assert.False(t, called, "receipts: relay must be dropped when deps are not configured")
}

func TestRelayActionReceipt_SignerStoreErrorRejected(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	signerStore := &stubSignerStore{err: errors.New("signer store unavailable")}
	recorder := &stubReceiptRecorder{}
	broker.SetReceiptRelayDeps(signerStore, recorder)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var called bool
	unregister := broker.RegisterHandler(channel, func(_ string, _ []byte) { called = true })
	defer unregister()

	wire, _ := buildSignedReceiptEnvelope(t, priv, keyID, operatorID, sessionID,
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-006")

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    wire,
	})

	assert.Empty(t, recorder.recorded(), "signer store error must not record the receipt")
	assert.False(t, called, "signer store error must not fan out")
}

func TestRelayActionReceipt_RecorderErrorStillFansOut(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	signerStore := &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}}
	recorder := &stubReceiptRecorder{err: errors.New("audit store write failed")}
	broker.SetReceiptRelayDeps(signerStore, recorder)
	handler := newOperatorSessionHandler(broker, "op-001")

	operatorID := "op-001"
	sessionID := "sess-001"
	channel := pubsub.ReceiptsChannel(operatorID, sessionID)

	var capturedData []byte
	unregister := broker.RegisterHandler(channel, func(_ string, data []byte) {
		capturedData = data
	})
	defer unregister()

	wire, _ := buildSignedReceiptEnvelope(t, priv, keyID, operatorID, sessionID,
		string(constants.ActionTypeFileEdit), "/tmp/test.txt", "user-001", "app-1", "tx-007")

	assert.NotPanics(t, func() {
		handler.handleAction(&pubsubv1.PubSubMessage{
			Action:  constants.PubSubActionPublish,
			Channel: channel,
			Data:    wire,
		})
	})
	// The recorder returned an error, but the receipt should still be
	// fanned out to subscribers (best-effort recording, not a gate).
	require.NotNil(t, capturedData, "recorder error must not block fan-out")
	assert.Equal(t, wire, capturedData)
}

func TestSetReceiptRelayDeps(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	broker.receiptRelayMu.RLock()
	assert.Nil(t, broker.receiptSignerStore, "signer store must be nil before SetReceiptRelayDeps")
	assert.Nil(t, broker.receiptAuditRecorder, "recorder must be nil before SetReceiptRelayDeps")
	broker.receiptRelayMu.RUnlock()

	signerStore := &stubSignerStore{keys: map[string]ed25519.PublicKey{"k1": {1, 2, 3}}}
	recorder := &stubReceiptRecorder{}
	broker.SetReceiptRelayDeps(signerStore, recorder)

	broker.receiptRelayMu.RLock()
	assert.NotNil(t, broker.receiptSignerStore)
	assert.NotNil(t, broker.receiptAuditRecorder)
	broker.receiptRelayMu.RUnlock()
}

// Ensure the context import is used (some stubs may reference it in future).
var _ = context.Background

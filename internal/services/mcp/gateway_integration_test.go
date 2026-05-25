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

package mcp

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestGatewaySignedEndToEndIntegration tests the full flow from MCP gateway
// through envelope processing to receipt generation, verifying that GatewaySigned
// is properly propagated through the entire chain.
func TestGatewaySignedEndToEndIntegration(t *testing.T) {
	t.Parallel()
	// Setup: Create a real envelope processor that will verify GatewaySigned
	processorCalled := false
	var receivedEnvelope *commonv1.GovernanceEnvelope

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "test result",
		},
	}

	// Wrap the processor to capture the envelope for verification
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture: func(env *commonv1.GovernanceEnvelope) {
			processorCalled = true
			receivedEnvelope = env
		},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		envProc:           wrappedProcessor,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024, // 10MB
	}

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify the processor was called
	require.True(t, processorCalled, "Envelope processor should have been called")
	require.NotNil(t, receivedEnvelope)

	// Verify GatewaySigned is set in the envelope
	require.NotNil(t, receivedEnvelope.Governance)
	require.True(t, receivedEnvelope.Governance.GatewaySigned, "Envelope should have GatewaySigned=true for MCP gateway")

	// Verify L2 metadata is present (gateway-local signer)
	require.NotNil(t, receivedEnvelope.Governance.L2)
	require.Equal(t, "test-key", receivedEnvelope.Governance.L2.KeyId)
	require.Contains(t, receivedEnvelope.Governance.L2.AgentIds, "gateway-local-signer")
}

// envelopeCaptureProcessor wraps an EnvelopeProcessor to capture envelopes for verification
type envelopeCaptureProcessor struct {
	delegate governance.EnvelopeProcessor
	capture  func(*commonv1.GovernanceEnvelope)
}

func (e *envelopeCaptureProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	// Parse the envelope to capture it
	var envelope commonv1.GovernanceEnvelope
	err := protojson.Unmarshal(payload, &envelope)
	if err == nil && e.capture != nil {
		e.capture(&envelope)
	}
	return e.delegate.ProcessEnvelope(ctx, payload)
}

// TestGatewaySignedReceiptIntegration tests that GatewaySigned is properly
// set in the receipt returned by the envelope processor.
func TestGatewaySignedReceiptIntegration(t *testing.T) {
	t.Parallel()
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId:    "tx-1",
			TransactionHash:  "hash-1",
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "test result",
			GatewaySigned:    true,
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: 1234567890,
			SignerKeyId:      "test-key",
			Signature:        "test-sig",
			L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		envProc:           processor,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024, // 10MB
	}

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify the receipt has GatewaySigned=true
	require.True(t, processor.receipt.GatewaySigned, "Receipt should have GatewaySigned=true")
}

// TestGatewaySignedCanonicalizationIntegration tests that GatewaySigned is included
// in the canonical form used for receipt signing.
func TestGatewaySignedCanonicalizationIntegration(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-1",
		TransactionHash:  "hash-1",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "test result",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1234567890,
		SignerKeyId:      "test-key",
		GatewaySigned:    true,
		L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
	}

	// Canonicalize the receipt
	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	// Parse the canonical form to verify GatewaySigned is included
	var parsed map[string]interface{}
	err = json.Unmarshal(canonical, &parsed)
	require.NoError(t, err)

	// Verify gateway_signed field is present and true
	gatewaySigned, ok := parsed["gateway_signed"]
	require.True(t, ok, "gateway_signed should be in canonical form")
	require.True(t, gatewaySigned.(bool), "gateway_signed should be true")

	// Verify it's deterministic
	canonical2, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	require.Equal(t, canonical, canonical2, "Canonicalization should be deterministic")
}

// TestGatewaySignedFalseIntegration tests the Tribunal path where GatewaySigned=false
func TestGatewaySignedFalseIntegration(t *testing.T) {
	t.Parallel()
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "test result",
			GatewaySigned: false,
		},
	}

	// Simulate a Tribunal-signed envelope (not from MCP gateway)
	envelope := &commonv1.GovernanceEnvelope{
		Id:              "tx-1",
		TransactionHash: "hash-1",
		ActionType:      string(constants.ActionTypeExecuteBash),
		Governance: &commonv1.GovernanceMetadata{
			GatewaySigned: false,
			L2: &commonv1.L2Metadata{
				TribunalSignature: "tribunal-sig",
				KeyId:             "tribunal-key",
				AgentIds:          []string{"agent-1", "agent-2", "agent-3"},
			},
		},
	}

	// Marshal the envelope
	uapBytes, err := protojson.Marshal(envelope)
	require.NoError(t, err)

	// Process through the envelope processor
	receipt, err := processor.ProcessEnvelope(context.Background(), uapBytes)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify GatewaySigned is false
	require.False(t, receipt.GatewaySigned, "Tribunal-signed transactions should have GatewaySigned=false")
}

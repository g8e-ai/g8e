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
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
)

type fakeEnvelopeProcessor struct {
	receipt    *operatorv1.ActionReceipt
	err        error
	gotPayload []byte
}

func (f *fakeEnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	f.gotPayload = append([]byte(nil), payload...)
	return f.receipt, f.err
}

type fakeStateRootProvider struct {
	root string
}

func (f *fakeStateRootProvider) GetCurrentStateRoot() (string, error) {
	return f.root, nil
}

type fakeSuspendedStore struct {
	txs map[string]*models.SuspendedTransaction
}

func (f *fakeSuspendedStore) StoreSuspendedTransaction(tx *models.SuspendedTransaction) error {
	if f.txs == nil {
		f.txs = make(map[string]*models.SuspendedTransaction)
	}
	f.txs[tx.TransactionHash] = tx
	return nil
}

func (f *fakeSuspendedStore) GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool) {
	tx, ok := f.txs[txHash]
	return tx, ok
}

func (f *fakeSuspendedStore) DeleteSuspendedTransaction(txHash string) error {
	delete(f.txs, txHash)
	return nil
}

func TestGatewayService_HandleToolsCall_ErrorMapping(t *testing.T) {
	cases := []struct {
		name         string
		GatewayErr   error
		expectedCode int
	}{
		{
			name:         "invalid envelope",
			GatewayErr:   governance.ErrInvalidEnvelope,
			expectedCode: ErrCodeInvalidEnvelope,
		},
		{
			name:         "hash mismatch",
			GatewayErr:   governance.ErrTransactionHashMismatch,
			expectedCode: ErrCodeHashMismatch,
		},
		{
			name:         "expired",
			GatewayErr:   governance.ErrTransactionExpired,
			expectedCode: ErrCodeExpired,
		},
		{
			name:         "replay",
			GatewayErr:   governance.ErrTransactionReplay,
			expectedCode: ErrCodeReplay,
		},
		{
			name:         "state root mismatch",
			GatewayErr:   governance.ErrStateRootMismatch,
			expectedCode: ErrCodeStateMismatch,
		},
		{
			name:         "L1 validation failed",
			GatewayErr:   governance.ErrL1ValidationFailed,
			expectedCode: ErrCodeL1ValidationFailed,
		},
		{
			name:         "L2 signature invalid",
			GatewayErr:   governance.ErrL2SignatureInvalid,
			expectedCode: ErrCodeL2SignatureInvalid,
		},
		{
			name:         "L3 proof invalid",
			GatewayErr:   governance.ErrL3ProofInvalid,
			expectedCode: ErrCodeL3ProofInvalid,
		},
		{
			name:         "payload decode failed",
			GatewayErr:   governance.ErrPayloadDecodeFailed,
			expectedCode: ErrCodePayloadDecodeFailed,
		},
		{
			name:         "internal error fallback",
			GatewayErr:   errors.New("some internal error"),
			expectedCode: -32603,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := &fakeEnvelopeProcessor{err: tc.GatewayErr}
			g := &GatewayService{
				envProc: proc,
			}

			// Valid MCP tools/call request
			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
			w := httptest.NewRecorder()

			g.HandleToolsCall(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp JSONRPCResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			require.NotNil(t, resp.Error)
			require.Equal(t, tc.expectedCode, resp.Error.Code)
			require.Equal(t, tc.GatewayErr.Error(), resp.Error.Message)
		})
	}
}

func TestGatewayService_HandleToolsCall_Suspension(t *testing.T) {
	proc := &fakeEnvelopeProcessor{err: governance.ErrL3ProofMissing}
	store := &fakeSuspendedStore{}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = pubKey

	g := &GatewayService{
		envProc:        proc,
		suspendedStore: store,
		signingKey:     privKey,
		keyID:          "test-key",
		publicBaseURL:  "https://localhost:8442",
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Nil(t, resp.Error)
	var result CallToolResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Contains(t, result.Content[0].Text, "Execution paused")
	require.Contains(t, result.Content[0].Text, "https://localhost:8442/approve/")

	// Verify it was stored
	require.Len(t, store.txs, 1)
	for _, tx := range store.txs {
		require.Equal(t, "test-tool", tx.ToolName)
		require.Equal(t, `{"foo":"bar"}`, string(tx.ToolArguments))
	}
}

func TestGatewayService_ResumeWithL3Proof(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "tool result",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	store := &fakeSuspendedStore{}

	txHash := "hash-1"
	envelope := `{"id":"tx-1","transaction_hash":"hash-1","action_type":"MCP_CALL","payload":"e30="}`
	store.StoreSuspendedTransaction(&models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        json.RawMessage(envelope),
	})

	g := &GatewayService{
		envProc:        proc,
		suspendedStore: store,
	}

	proof := &commonv1.L3Proof{
		CredentialId: "cred-1",
	}

	gotReceipt, err := g.ResumeWithL3Proof(context.Background(), txHash, "user-1", proof)
	require.NoError(t, err)
	require.Equal(t, receipt, gotReceipt)

	// Verify it was deleted from store
	require.Empty(t, store.txs)

	// Verify L3 proof was attached in the call to ProcessEnvelope
	require.Contains(t, string(proc.gotPayload), `"l3"`)
	require.Contains(t, string(proc.gotPayload), `"credentialId"`)
}

func TestGatewayService_HandleResourcesRead(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "resource content",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = pubKey

	g := &GatewayService{
		envProc:           proc,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///test.txt"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/resources/read", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleResourcesRead(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Nil(t, resp.Error)
	var result ReadResourceResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Contents, 1)
	require.Equal(t, "resource content", result.Contents[0].Text)
}

func TestGatewayService_HandlePromptsGet(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "prompt template",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = pubKey

	g := &GatewayService{
		envProc:           proc,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test-prompt"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/get", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandlePromptsGet(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Nil(t, resp.Error)
	var result GetPromptResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Equal(t, "prompt template", result.Description)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "user", result.Messages[0].Role)
}

func TestGatewayService_HandleToolsCallSSE(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "streamed result",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = pubKey

	g := &GatewayService{
		envProc:           proc,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCallSSE(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

	body := w.Body.String()
	require.Contains(t, body, "data:")
	require.Contains(t, body, "streamed result")
}

func TestGatewayService_EnvelopeGatewaySigned(t *testing.T) {
	// Test that MCP gateway sets GatewaySigned=true in envelope
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	_ = pubKey

	g := &GatewayService{
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
	}

	opts := processGatewayOptions{
		actionType:     constants.ActionTypeMcpCall,
		targetResource: "test-tool",
		payloadBytes:   []byte("{}"),
	}

	hash, uapBytes, err := g.processGatewayTransaction(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.NotEmpty(t, uapBytes)

	// Parse the envelope to verify GatewaySigned is set
	var envelope commonv1.GovernanceEnvelope
	err = protojson.Unmarshal(uapBytes, &envelope)
	require.NoError(t, err)

	// Verify GatewaySigned is set to true
	require.NotNil(t, envelope.Governance)
	require.True(t, envelope.Governance.GatewaySigned, "MCP gateway should set GatewaySigned=true for single-agent clients")
}

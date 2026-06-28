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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/storage/storagetest"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (f *fakeSuspendedStore) StoreSuspendedTransaction(_ context.Context, tx *models.SuspendedTransaction) error {
	if f.txs == nil {
		f.txs = make(map[string]*models.SuspendedTransaction)
	}
	f.txs[tx.TransactionHash] = tx
	return nil
}

func (f *fakeSuspendedStore) GetSuspendedTransaction(_ context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	tx, ok := f.txs[txHash]
	return tx, ok, nil
}

func (f *fakeSuspendedStore) ListSuspendedTransactions(_ context.Context, userID string) ([]*models.SuspendedTransaction, error) {
	var result []*models.SuspendedTransaction
	for _, tx := range f.txs {
		if userID == "" || tx.UserID == userID {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (f *fakeSuspendedStore) ApproveSuspendedTransaction(_ context.Context, txHash string, proof models.ApprovalProof) error {
	if tx, ok := f.txs[txHash]; ok {
		tx.ApprovedBy = proof.ApprovedBy
		tx.ApprovalSignature = proof.CliSignature
		tx.ExpectedCertFingerprint = proof.CertFingerprint
		tx.ApprovalPublicKey = proof.ApprovalPublicKey
		tx.PasskeyCredentialID = proof.CredentialID
		tx.PasskeyClientDataJSON = proof.ClientDataJSON
		tx.PasskeyAuthenticatorData = proof.AuthenticatorData
		tx.PasskeySignature = proof.Signature
	}
	return nil
}

func (f *fakeSuspendedStore) DeleteSuspendedTransaction(_ context.Context, txHash string) error {
	delete(f.txs, txHash)
	return nil
}

func (f *fakeSuspendedStore) CleanupExpiredSuspendedTransactions(_ context.Context) (int64, error) {
	var count int64
	for hash, tx := range f.txs {
		if tx.ExpiresAt.Before(time.Now()) {
			delete(f.txs, hash)
			count++
		}
	}
	return count, nil
}

func (f *fakeSuspendedStore) GetExpiredSuspendedTransactions(_ context.Context) ([]*models.SuspendedTransaction, error) {
	var expired []*models.SuspendedTransaction
	for _, tx := range f.txs {
		if tx.ExpiresAt.Before(time.Now()) {
			expired = append(expired, tx)
		}
	}
	return expired, nil
}

func TestGatewayService_HandleToolsCall_ErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		GatewayErr   error
		expectedCode int
	}{
		{
			name:         "invalid envelope",
			GatewayErr:   governance.ErrInvalidEnvelope,
			expectedCode: constants.ErrCodeInvalidEnvelope,
		},
		{
			name:         "hash mismatch",
			GatewayErr:   governance.ErrTransactionHashMismatch,
			expectedCode: constants.ErrCodeHashMismatch,
		},
		{
			name:         "expired",
			GatewayErr:   governance.ErrTransactionExpired,
			expectedCode: constants.ErrCodeExpired,
		},
		{
			name:         "replay",
			GatewayErr:   governance.ErrTransactionReplay,
			expectedCode: constants.ErrCodeReplay,
		},
		{
			name:         "state root mismatch",
			GatewayErr:   governance.ErrStateRootMismatch,
			expectedCode: constants.ErrCodeStateMismatch,
		},
		{
			name:         "L1 validation failed",
			GatewayErr:   governance.ErrL1ValidationFailed,
			expectedCode: constants.ErrCodeL1ValidationFailed,
		},
		{
			name:         "L2 signature invalid",
			GatewayErr:   governance.ErrL2SignatureInvalid,
			expectedCode: constants.ErrCodeL2SignatureInvalid,
		},
		{
			name:         "L3 proof invalid",
			GatewayErr:   governance.ErrL3ProofInvalid,
			expectedCode: constants.ErrCodeL3ProofInvalid,
		},
		{
			name:         "payload decode failed",
			GatewayErr:   governance.ErrPayloadDecodeFailed,
			expectedCode: constants.ErrCodePayloadDecodeFailed,
		},
		{
			name:         "internal error fallback",
			GatewayErr:   errors.New("some internal error"),
			expectedCode: -32603,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proc := &fakeEnvelopeProcessor{err: tc.GatewayErr}
			g := newTestGatewayService(t, withEnvProc(proc))

			// Valid MCP tools/call request
			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
			w := httptest.NewRecorder()

			g.HandleToolsCall(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			expectedJSON := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":%d,"message":"%s"}}`, tc.expectedCode, tc.GatewayErr.Error())
			require.JSONEq(t, expectedJSON, w.Body.String())
		})
	}
}

func TestGatewayService_HandleToolsCall_Suspension(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{err: governance.ErrL3ProofMissing}
	store := &fakeSuspendedStore{}

	g := newTestGatewayService(t,
		withEnvProc(proc),
		withSuspendedStore(store),
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	require.Len(t, store.txs, 1)
	var txHash string
	for k, tx := range store.txs {
		txHash = k
		require.Equal(t, "test-tool", tx.ToolName)
		require.JSONEq(t, `{"foo":"bar"}`, string(tx.ToolArguments))
	}
	approvalURL := fmt.Sprintf("https://localhost:%d/approve/%s", constants.Ports.OperatorHttps, txHash)
	textJSON, err := json.Marshal(approvalPausedMessage(approvalURL))
	require.NoError(t, err)
	expectedJSON := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, textJSON)
	require.JSONEq(t, expectedJSON, w.Body.String())
}

func TestGatewayService_ResumeWithL3Proof(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "tool result",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	store := &fakeSuspendedStore{}

	txHash := "hash-1"
	envelope := `{"id":"tx-1","transaction_hash":"hash-1","action_type":"MCP_CALL","payload":"e30="}`

	g := newTestGatewayService(t,
		withEnvProc(proc),
		withSuspendedStore(store),
	)

	g.StoreSuspendedTransaction(context.Background(), txHash, []byte(envelope), "test-tool", json.RawMessage(`{}`), "user-1", "op-1", "")

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

func TestGatewayService_ResumeWithL3Proof_ExpiredTransaction(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{}
	store := &fakeSuspendedStore{}

	g := newTestGatewayService(t,
		withEnvProc(proc),
		withSuspendedStore(store),
	)

	proof := &commonv1.L3Proof{
		CredentialId: "cred-1",
	}

	// Attempt to resume a non-existent (expired) transaction
	_, err := g.ResumeWithL3Proof(context.Background(), "expired-hash", "user-1", proof)
	require.Error(t, err)
	require.ErrorIs(t, err, constants.ErrTransactionExpired)
}

func TestGatewayService_RunMaintenance_AuditsExpiredTransactions(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := storagetest.CreateTestVault(t, vaultDir, privKey)

	// Create audit store
	auditConfig := &storage.AuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    filepath.Join(tempDir, "audit.db"),
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}
	auditStore, err := storage.NewSQLAuditStore(auditConfig, slog.Default())
	require.NoError(t, err)
	defer auditStore.Close()

	// Create session for the operator
	operatorSessionID := "session-maintenance-test"
	err = auditStore.CreateSession(operatorSessionID, "operator", "Maintenance Test", "user@test.com")
	require.NoError(t, err)

	// Create a fake store with an expired transaction
	store := &fakeSuspendedStore{}
	expiredTx := &models.SuspendedTransaction{
		TransactionHash: "expired-hash-123",
		Envelope:        []byte(`{"id":"123"}`),
		CreatedAt:       time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:       time.Now().UTC().Add(-5 * time.Minute),
		ToolName:        "test-tool",
		ToolArguments:   json.RawMessage(`{}`),
		UserID:          "user-1",
		OperatorID:      operatorSessionID,
	}
	store.StoreSuspendedTransaction(context.Background(), expiredTx)

	g := newTestGatewayService(t,
		withSuspendedStore(store),
		withAuditStore(auditStore),
	)

	// Run a single maintenance sweep directly
	err = g.runMaintenanceSweep(context.Background())
	require.NoError(t, err)

	// Verify expiry event was recorded in audit store
	events, err := auditStore.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Verify the event details
	require.Equal(t, constants.Event.Operator.Notary.TransactionExpired, events[0].Type)
	require.Contains(t, events[0].ContentText, "expired-hash-123")
	require.Contains(t, events[0].ContentText, "test-tool")
	require.Contains(t, events[0].ContentText, "expired")

	// Verify the expired transaction was deleted
	_, found, err := store.GetSuspendedTransaction(context.Background(), "expired-hash-123")
	require.NoError(t, err)
	require.False(t, found)
}

func TestGatewayService_HandleResourcesRead(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "resource content",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}

	g := newTestGatewayService(t, withEnvProc(proc))

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///test.txt"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/resources/read", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleResourcesRead(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	expectedJSON := `{"jsonrpc":"2.0","id":1,"result":{"contents":[{"type":"text","text":"resource content"}]}}`
	require.JSONEq(t, expectedJSON, w.Body.String())
}

func TestGatewayService_HandlePromptsGet(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary: "prompt template",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}

	g := newTestGatewayService(t, withEnvProc(proc))

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test-prompt"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/get", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandlePromptsGet(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	expectedJSON := `{"jsonrpc":"2.0","id":1,"result":{"description":"prompt template","messages":[{"role":"user","content":{"type":"text","text":"prompt template"}}]}}`
	require.JSONEq(t, expectedJSON, w.Body.String())
}

func TestGatewayService_HandleToolsCallSSE(t *testing.T) {
	t.Parallel()

	t.Run("successful SSE stream", func(t *testing.T) {
		t.Parallel()
		receipt := &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "streamed result",
		}
		proc := &fakeEnvelopeProcessor{receipt: receipt}

		g := newTestGatewayService(t, withEnvProc(proc))

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
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCallSSE(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		require.Contains(t, body, "circuit open")
	})

	t.Run("invalid JSON-RPC", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		reqBody := `invalid json`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCallSSE(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		require.Contains(t, body, "error")
	})

	t.Run("payload too large", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withMaxPayloadBytes(100))

		largePayload := strings.Repeat("a", 1000)
		reqBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"data":"%s"}}}`, largePayload)
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCallSSE(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		require.Contains(t, body, "error")
	})

	t.Run("missing tool name", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCallSSE(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		require.Contains(t, body, "error")
	})

	t.Run("governance error - L1 validation failed", func(t *testing.T) {
		t.Parallel()
		proc := &fakeEnvelopeProcessor{err: governance.ErrL1ValidationFailed}

		g := newTestGatewayService(t, withEnvProc(proc))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call/sse", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCallSSE(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		require.Contains(t, body, "error")
	})

}

func TestGatewayService_CircuitBreaker(t *testing.T) {
	t.Parallel()

	t.Run("initially closed", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withCircuitBreaker(5, 1*time.Minute))
		require.False(t, g.isCircuitOpen())
	})

	t.Run("opens after max failures", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withCircuitBreaker(3, 1*time.Minute))

		for i := 0; i < 3; i++ {
			g.recordFailure()
		}
		require.True(t, g.isCircuitOpen())
	})

	t.Run("closes after success", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}
		require.True(t, g.isCircuitOpen())

		// Close it with success
		g.recordSuccess()
		require.False(t, g.isCircuitOpen())
		require.Equal(t, 0, g.failureCount)
	})

	t.Run("half-open after cooldown", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withCircuitBreaker(3, 100*time.Millisecond))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}
		require.True(t, g.isCircuitOpen())

		// Wait for cooldown
		time.Sleep(150 * time.Millisecond)
		require.False(t, g.isCircuitOpen(), "circuit should be half-open after cooldown")
	})
}

func TestGatewayService_HandleToolsList(t *testing.T) {
	t.Parallel()

	t.Run("native tools when no downstream", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.NotEmpty(t, result.Tools)
	})

	t.Run("successful POST proxy to downstream", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"test-tool"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("successful GET proxy to downstream", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 5; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		// Should return native tools as fallback
		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.NotEmpty(t, result.Tools)
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		// Should return native tools as fallback when downstream returns 500
		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.NotEmpty(t, result.Tools)
	})

	t.Run("downstream connection error", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		// Should return native tools as fallback when downstream is unreachable
		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.NotEmpty(t, result.Tools)
	})

	t.Run("empty POST body uses default", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "tools/list")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(""))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("merges native tools with downstream tools", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return a downstream tool that doesn't conflict with native tools
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"downstream_tool","description":"A downstream tool","inputSchema":{"type":"object"}}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)

		// Should have both downstream tool and native tools
		toolNames := make(map[string]bool)
		for _, tool := range result.Tools {
			toolNames[tool.Name] = true
		}

		require.Contains(t, toolNames, "downstream_tool", "should include downstream tool")
		require.Contains(t, toolNames, "db_discover_topology", "should include native tools")
	})

	t.Run("method not allowed", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodDelete, "/mcp/tools/list", nil)
		w := httptest.NewRecorder()

		g.HandleToolsList(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGatewayService_HandleResourcesList(t *testing.T) {
	t.Parallel()

	t.Run("empty list when no downstream", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp ResourcesListResult
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Empty(t, resp.Resources)
	})

	t.Run("successful POST proxy to downstream", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[{"uri":"file:///test.txt","name":"test"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("successful GET proxy to downstream", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodGet, "/mcp/resources/list", nil)
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 5; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("downstream connection error", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "failed to query downstream")
	})

	t.Run("method not allowed", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPut, "/mcp/resources/list", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("empty POST body uses default", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "resources/list")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/resources/list", strings.NewReader(""))
		w := httptest.NewRecorder()

		g.HandleResourcesList(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGatewayService_HandlePromptsList(t *testing.T) {
	t.Parallel()

	t.Run("empty list when no downstream", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandlePromptsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp PromptsListResult
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Empty(t, resp.Prompts)
	})

	t.Run("proxies to downstream MCP server", func(t *testing.T) {
		t.Parallel()
		// Mock downstream MCP server
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"prompts":[{"name":"test-prompt","description":"A test prompt"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandlePromptsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandlePromptsList(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp/prompts/list", strings.NewReader("{}"))
		w := httptest.NewRecorder()

		g.HandlePromptsList(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodDelete, "/mcp/prompts/list", nil)
		w := httptest.NewRecorder()

		g.HandlePromptsList(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGatewayService_IsNativeTool(t *testing.T) {
	t.Parallel()
	g := newTestGatewayService(t)

	t.Run("native tool recognized", func(t *testing.T) {
		t.Parallel()
		require.True(t, g.isNativeTool("db_discover_topology"))
		require.True(t, g.isNativeTool("db_query_validate"))
		require.True(t, g.isNativeTool("log_stream_filter"))
	})

	t.Run("non-native tool rejected", func(t *testing.T) {
		t.Parallel()
		require.False(t, g.isNativeTool("external_tool"))
		require.False(t, g.isNativeTool(""))
	})
}

func TestGatewayService_ScanForForbiddenPatterns(t *testing.T) {
	t.Parallel()
	g := newTestGatewayService(t)

	strVal := func(s string) FieldValue { return FieldValue{Str: &s} }

	t.Run("detects sudo with context", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("sudo rm -rf /"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "L1 hard gate")
		require.Contains(t, err.Error(), "sudo")
		require.Contains(t, err.Error(), "privilege escalation")
	})

	t.Run("detects password with context", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("password=secret123"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "L1 hard gate")
		require.Contains(t, err.Error(), "password")
		require.Contains(t, err.Error(), "credential leak")
	})

	t.Run("detects api_key with context", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("api_key=sk-12345"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "L1 hard gate")
		require.Contains(t, err.Error(), "api_key")
		require.Contains(t, err.Error(), "credential leak")
	})

	t.Run("detects destructive file operation", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("rm -rf /"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "destructive file operation")
	})

	t.Run("detects external URL pattern", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("visit https://example.com"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "external URL")
	})

	t.Run("allows safe values", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(strVal("safe value 123"))
		require.NoError(t, err)
	})

	t.Run("null value allowed", func(t *testing.T) {
		t.Parallel()
		err := g.scanForForbiddenPatterns(FieldValue{Null: true})
		require.NoError(t, err)
	})
}

func TestGatewayService_MapGatewayError(t *testing.T) {
	t.Parallel()
	g := newTestGatewayService(t)

	t.Run("maps governance errors", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			err      error
			wantCode int
		}{
			{governance.ErrInvalidEnvelope, constants.ErrCodeInvalidEnvelope},
			{governance.ErrTransactionHashMismatch, constants.ErrCodeHashMismatch},
			{governance.ErrTransactionExpired, constants.ErrCodeExpired},
			{governance.ErrTransactionReplay, constants.ErrCodeReplay},
			{governance.ErrStateRootMismatch, constants.ErrCodeStateMismatch},
			{governance.ErrL1ValidationFailed, constants.ErrCodeL1ValidationFailed},
			{governance.ErrL2SignatureInvalid, constants.ErrCodeL2SignatureInvalid},
			{governance.ErrL3ProofInvalid, constants.ErrCodeL3ProofInvalid},
			{governance.ErrPayloadDecodeFailed, constants.ErrCodePayloadDecodeFailed},
		}

		for _, tt := range tests {
			code, msg := g.mapGatewayError(tt.err)
			require.Equal(t, tt.wantCode, code)
			require.NotEmpty(t, msg)
		}
	})

	t.Run("unknown error maps to internal error", func(t *testing.T) {
		t.Parallel()
		code, msg := g.mapGatewayError(errors.New("unknown error"))
		require.Equal(t, -32603, code) // Internal error code
		require.Contains(t, msg, "unknown error")
	})
}

func TestGatewayService_StoreSuspendedTransaction(t *testing.T) {
	t.Parallel()

	t.Run("stores transaction", func(t *testing.T) {
		t.Parallel()
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withSuspendedStore(store))

		g.StoreSuspendedTransaction(context.Background(), "hash-123", []byte(`{"id":"123"}`), "test-tool", json.RawMessage(`{"arg":"val"}`), "user-1", "op-1", "cert-fp-abc123")

		tx, found, err := store.GetSuspendedTransaction(context.Background(), "hash-123")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "hash-123", tx.TransactionHash)
		require.Equal(t, "test-tool", tx.ToolName)
		require.Equal(t, "user-1", tx.UserID)
		require.Equal(t, "op-1", tx.OperatorID)
		require.Equal(t, "cert-fp-abc123", tx.ExpectedCertFingerprint)
	})

	t.Run("nil store does not panic", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withSuspendedStore(nil))

		// Should not panic
		g.StoreSuspendedTransaction(context.Background(), "test-hash", []byte(`{}`), "test-tool", json.RawMessage(`{}`), "user-1", "op-1", "")
	})

	t.Run("records approval requested event", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		vaultDir := filepath.Join(tempDir, "vault")

		// Create test vault
		_, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		testVault := storagetest.CreateTestVault(t, vaultDir, privKey)

		// Create audit store
		auditConfig := &storage.AuditStoreConfig{
			DataDir:                   tempDir,
			DBPath:                    filepath.Join(tempDir, "audit.db"),
			MaxDBSizeMB:               100,
			RetentionDays:             7,
			PruneIntervalMinutes:      60,
			OutputTruncationThreshold: 102400,
			HeadTailSize:              51200,
			EncryptionVault:           testVault,
		}
		auditStore, err := storage.NewSQLAuditStore(auditConfig, slog.Default())
		require.NoError(t, err)
		defer auditStore.Close()

		// Create session for the operator
		operatorSessionID := "session-approval-requested-test"
		err = auditStore.CreateSession(operatorSessionID, "operator", "Approval Requested Test", "user@test.com")
		require.NoError(t, err)

		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t,
			withSuspendedStore(store),
			withAuditStore(auditStore),
		)

		txHash := "hash-approval-123"
		g.StoreSuspendedTransaction(context.Background(), txHash, []byte(`{"id":"123"}`), "test-tool", json.RawMessage(`{"arg":"val"}`), "user-1", operatorSessionID, "cert-fp-abc123")

		// Verify approval requested event was recorded in audit store
		events, err := auditStore.GetEvents(operatorSessionID, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)

		// Verify the event details
		require.Equal(t, constants.Event.Operator.Notary.ApprovalRequested, events[0].Type)
		require.Contains(t, events[0].ContentText, txHash)
		require.Contains(t, events[0].ContentText, "test-tool")
		require.Contains(t, events[0].ContentText, "approval requested")
	})
}

func TestGatewayService_GetSuspendedTransaction(t *testing.T) {
	t.Parallel()

	t.Run("successful retrieval", func(t *testing.T) {
		t.Parallel()
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withSuspendedStore(store))

		g.StoreSuspendedTransaction(context.Background(), "test-hash", []byte(`{"test":"envelope"}`), "test-tool", json.RawMessage(`{"arg":"val"}`), "user-1", "op-1", "")

		retrieved, ok, err := g.GetSuspendedTransaction(context.Background(), "test-hash")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "test-hash", retrieved.TransactionHash)
		require.Equal(t, "test-tool", retrieved.ToolName)
	})

	t.Run("transaction not found", func(t *testing.T) {
		t.Parallel()
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withSuspendedStore(store))

		retrieved, ok, err := g.GetSuspendedTransaction(context.Background(), "nonexistent")
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, retrieved)
	})

	t.Run("nil store returns false", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withSuspendedStore(nil))

		retrieved, ok, err := g.GetSuspendedTransaction(context.Background(), "test-hash")
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, retrieved)
	})
}

func TestGatewayService_DeleteSuspendedTransaction(t *testing.T) {
	t.Parallel()

	t.Run("successful deletion", func(t *testing.T) {
		t.Parallel()
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withSuspendedStore(store))

		g.StoreSuspendedTransaction(context.Background(), "test-hash", []byte(`{}`), "test-tool", json.RawMessage(`{}`), "user-1", "op-1", "")

		g.DeleteSuspendedTransaction(context.Background(), "test-hash")

		_, ok, err := store.GetSuspendedTransaction(context.Background(), "test-hash")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("delete nonexistent transaction", func(t *testing.T) {
		t.Parallel()
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withSuspendedStore(store))

		// Should not panic
		g.DeleteSuspendedTransaction(context.Background(), "nonexistent")
	})

	t.Run("nil store does not panic", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withSuspendedStore(nil))

		// Should not panic
		g.DeleteSuspendedTransaction(context.Background(), "test-hash")
	})
}

func TestGatewayService_NewGatewayService(t *testing.T) {
	t.Parallel()

	t.Run("creates service with dependencies", func(t *testing.T) {
		t.Parallel()
		deps := Dependencies{
			Logger:          slog.Default(),
			Responder:       response.NewWriter(slog.Default()),
			SuspendedStore:  &fakeSuspendedStore{},
			MaxPayloadBytes: 10 * 1024 * 1024,
		}

		g, err := NewGatewayService(deps)
		require.NoError(t, err)

		require.NotNil(t, g)
		require.Equal(t, deps.Logger, g.logger)
		require.Equal(t, deps.Responder, g.responder)
		require.Equal(t, deps.SuspendedStore, g.suspendedStore)
		require.Equal(t, deps.MaxPayloadBytes, g.maxPayloadBytes)
		require.Equal(t, 5, g.maxFailures)
		require.Equal(t, 1*time.Minute, g.cooldownDuration)
		require.NotNil(t, g.nativeToolHandler)
	})

	t.Run("initializes field path registry", func(t *testing.T) {
		t.Parallel()
		deps := Dependencies{
			Logger:          slog.Default(),
			Responder:       response.NewWriter(slog.Default()),
			MaxPayloadBytes: 10 * 1024 * 1024,
		}

		g, err := NewGatewayService(deps)
		require.NoError(t, err)

		require.NotNil(t, g.fieldPathRegistry)
	})

	t.Run("handles field path registry initialization error gracefully", func(t *testing.T) {
		t.Parallel()
		deps := Dependencies{
			Logger:          slog.Default(),
			Responder:       response.NewWriter(slog.Default()),
			MaxPayloadBytes: 10 * 1024 * 1024,
		}

		g, err := NewGatewayService(deps)
		require.NoError(t, err)

		// Should not panic even if registry init fails
		require.NotNil(t, g)
	})
}

func TestGatewayService_HandleReadField(t *testing.T) {
	t.Parallel()

	t.Run("field path registry not initialized", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withFieldPathRegistry(nil))

		_, err := g.handleReadField(context.Background(), json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "field path registry not initialized")
	})

	t.Run("database service not configured", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(nil))

		_, err := g.handleReadField(context.Background(), json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "database service not configured")
	})

	t.Run("invalid JSON arguments", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(&fakeDBService{}))

		_, err := g.handleReadField(context.Background(), json.RawMessage(`invalid json`))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("missing required fields", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(&fakeDBService{}))

		testCases := []struct {
			name  string
			args  string
			error string
		}{
			{"missing collection", `{"document_id":"doc1","field_path":"field1","operator_session_id":"sess1"}`, "collection required"},
			{"missing document_id", `{"collection":"coll1","field_path":"field1","operator_session_id":"sess1"}`, "document_id required"},
			{"missing field_path", `{"collection":"coll1","document_id":"doc1","operator_session_id":"sess1"}`, "field_path required"},
			{"missing operator_session_id", `{"collection":"coll1","document_id":"doc1","field_path":"field1"}`, "operator_session_id required"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := g.handleReadField(context.Background(), json.RawMessage(tc.args))
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.error)
			})
		}
	})

	t.Run("field path validation failed", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(&fakeDBService{}))

		args := `{"collection":"investigations","document_id":"doc1","field_path":"credentials.api_key","operator_session_id":"sess1"}`
		_, err := g.handleReadField(context.Background(), json.RawMessage(args))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("session validation failed", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		validator := &fakeSessionValidator{valid: false}
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(&fakeDBService{}), withSessionValidator(validator))

		args := `{"collection":"investigations","document_id":"doc1","field_path":"status","operator_session_id":"sess1"}`
		_, err := g.handleReadField(context.Background(), json.RawMessage(args))
		require.Error(t, err)
		require.Contains(t, err.Error(), "operator session is invalid or expired")
	})

	t.Run("successful field read", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		db := &fakeDBService{}
		validator := &fakeSessionValidator{valid: true}
		audit := &fakeAuditLogger{}
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(db), withSessionValidator(validator), withAuditLogger(audit))

		args := `{"collection":"investigations","document_id":"doc1","field_path":"status","operator_session_id":"sess1"}`
		result, err := g.handleReadField(context.Background(), json.RawMessage(args))
		require.NoError(t, err)
		require.NotNil(t, result)

		toolResult, ok := result.(CallToolResult)
		require.True(t, ok)
		require.Len(t, toolResult.Content, 1)
		require.Equal(t, "text", toolResult.Content[0].Type)
		require.Contains(t, toolResult.Content[0].Text, "test-value")
	})

	t.Run("forbidden pattern in field value", func(t *testing.T) {
		t.Parallel()
		registry, _ := NewFieldPathRegistry(slog.Default())
		s := "password=secret123"
		db := &fakeDBService{fieldValue: &FieldValue{Str: &s}}
		validator := &fakeSessionValidator{valid: true}
		g := newTestGatewayService(t, withFieldPathRegistry(registry), withDBService(db), withSessionValidator(validator))

		args := `{"collection":"investigations","document_id":"doc1","field_path":"status","operator_session_id":"sess1"}`
		_, err := g.handleReadField(context.Background(), json.RawMessage(args))
		require.Error(t, err)
		require.Error(t, err)
	})
}

func TestGatewayService_HandleMCPRequest(t *testing.T) {
	t.Parallel()

	t.Run("method not allowed", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodGet, "/mcp/test", nil)
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 5; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"test/method"}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("payload too large", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withMaxPayloadBytes(1024))

		largeBody := strings.Repeat("a", 2048)
		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"test/method","params":"%s"}`, largeBody)))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "payload too large")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`invalid json`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "parse error")
	})

	t.Run("invalid JSON-RPC version", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"1.0","id":1,"method":"test/method"}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "jsonrpc version must be 2.0")
	})

	t.Run("missing method", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"2.0","id":1}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "method required")
	})

	t.Run("method mismatch", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"wrong/method"}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "method not found")
	})

	t.Run("handler error", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"test/method"}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return nil, errors.New("handler error")
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "handler error")
	})

	t.Run("successful request", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"test/method"}`))
		w := httptest.NewRecorder()

		g.handleMCPRequest(w, req, "test/method", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
			return map[string]string{"result": "success"}, nil
		})

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
		require.NotNil(t, resp.Result)
	})
}

func TestGatewayService_DependencySetters(t *testing.T) {
	t.Parallel()
	g := newTestGatewayService(t)

	t.Run("SetDependencies", func(t *testing.T) {
		t.Parallel()
		_, privKey, _ := ed25519.GenerateKey(rand.Reader)

		g.SetDependencies(&fakeEnvelopeProcessor{}, &fakeStateRootProvider{root: "test"}, privKey, "key-1", "http://downstream")
		require.NotNil(t, g.envProc)
		require.NotNil(t, g.stateRootProvider)
		require.NotNil(t, g.signingKey)
		require.Equal(t, "key-1", g.keyID)
		require.Equal(t, "http://downstream", g.downstreamURL)
	})

	t.Run("SetA2ADependencies", func(t *testing.T) {
		t.Parallel()
		g.SetA2ADependencies("http://a2a-downstream")
		require.Equal(t, "http://a2a-downstream", g.a2aDownstreamURL)
	})

	t.Run("SetPublicBaseURL", func(t *testing.T) {
		t.Parallel()
		g.SetPublicBaseURL("https://public.example.com")
		require.Equal(t, "https://public.example.com", g.publicBaseURL)
	})

	t.Run("SetDBService", func(t *testing.T) {
		t.Parallel()
		fakeDB := &fakeDBService{}
		g.SetDBService(fakeDB)
		require.Equal(t, fakeDB, g.dbService)
	})

	t.Run("SetSessionValidator", func(t *testing.T) {
		t.Parallel()
		fakeValidator := &fakeSessionValidator{}
		g.SetSessionValidator(fakeValidator)
		require.Equal(t, fakeValidator, g.sessionValidator)
	})

	t.Run("SetAuditLogger", func(t *testing.T) {
		t.Parallel()
		fakeLogger := &fakeAuditLogger{}
		g.SetAuditLogger(fakeLogger)
		require.Equal(t, fakeLogger, g.auditLogger)
	})
}

func TestGatewayService_RunMaintenance(t *testing.T) {
	t.Parallel()
	g := newTestGatewayService(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run maintenance for a short time - should not panic
	g.RunMaintenance(ctx)
}

// Helper fakes for dependency tests

type fakeDBService struct {
	fieldValue *FieldValue
}

func (f *fakeDBService) GetField(collection, id, fieldPath string) (FieldValue, error) {
	if f.fieldValue != nil {
		return *f.fieldValue, nil
	}
	s := "test-value"
	return FieldValue{Str: &s}, nil
}

type fakeSessionValidator struct {
	valid bool
}

func (f *fakeSessionValidator) ValidateSession(operatorSessionID string) (bool, error) {
	return f.valid, nil
}

type fakeAuditLogger struct{}

func (f *fakeAuditLogger) LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value FieldValue) error {
	return nil
}

// testGatewayOption is a functional option for configuring test GatewayService instances
type testGatewayOption func(*GatewayService)

// withEnvProc sets a custom envelope processor for the test GatewayService
func withEnvProc(proc governance.EnvelopeProcessor) testGatewayOption {
	return func(g *GatewayService) {
		g.envProc = proc
	}
}

// withSuspendedStore sets a custom suspended store for the test GatewayService
func withSuspendedStore(store storage.SuspendedTransactionStore) testGatewayOption {
	return func(g *GatewayService) {
		g.suspendedStore = store
	}
}

// withAuditStore sets a custom audit store for the test GatewayService
func withAuditStore(auditStore *storage.SQLAuditStore) testGatewayOption {
	return func(g *GatewayService) {
		g.auditStore = auditStore
	}
}

// withDownstreamURL sets a custom downstream URL for the test GatewayService
func withDownstreamURL(url string) testGatewayOption {
	return func(g *GatewayService) {
		g.downstreamURL = url
	}
}

// withA2ADownstreamURL sets a custom A2A downstream URL for the test GatewayService
func withA2ADownstreamURL(url string) testGatewayOption {
	return func(g *GatewayService) {
		g.a2aDownstreamURL = url
	}
}

// withCircuitBreaker sets custom circuit breaker settings for the test GatewayService
func withCircuitBreaker(maxFailures int, cooldown time.Duration) testGatewayOption {
	return func(g *GatewayService) {
		g.maxFailures = maxFailures
		g.cooldownDuration = cooldown
	}
}

// withMaxPayloadBytes sets a custom max payload size for the test GatewayService
func withMaxPayloadBytes(bytes int64) testGatewayOption {
	return func(g *GatewayService) {
		g.maxPayloadBytes = bytes
	}
}

// withFieldPathRegistry sets a custom field path registry for the test GatewayService
func withFieldPathRegistry(registry *FieldPathRegistry) testGatewayOption {
	return func(g *GatewayService) {
		g.fieldPathRegistry = registry
	}
}

// withDBService sets a custom database service for the test GatewayService
func withDBService(db FieldReader) testGatewayOption {
	return func(g *GatewayService) {
		g.dbService = db
	}
}

// withSessionValidator sets a custom session validator for the test GatewayService
func withSessionValidator(v SessionValidator) testGatewayOption {
	return func(g *GatewayService) {
		g.sessionValidator = v
	}
}

// withAuditLogger sets a custom audit logger for the test GatewayService
func withAuditLogger(audit AuditLogger) testGatewayOption {
	return func(g *GatewayService) {
		g.auditLogger = audit
	}
}

// newTestGatewayService creates a GatewayService with sensible defaults for testing.
// Options can be provided to override specific fields.
func newTestGatewayService(t *testing.T, opts ...testGatewayOption) *GatewayService {
	// Generate default signing key
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)

	g := &GatewayService{
		logger:            slog.Default(),
		responder:         response.NewWriter(slog.Default()),
		envProc:           &fakeEnvelopeProcessor{},
		suspendedStore:    &fakeSuspendedStore{},
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		publicBaseURL:     netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps),
		maxFailures:       5,
		cooldownDuration:  1 * time.Minute,
		maxPayloadBytes:   10 * 1024 * 1024,
	}
	nativeToolHandler, err := NewNativeToolHandler(nil)
	if err != nil {
		t.Fatalf("failed to create native tool handler: %v", err)
	}
	g.nativeToolHandler = nativeToolHandler

	// Apply options
	for _, opt := range opts {
		opt(g)
	}

	return g
}

func TestGatewayService_HandleA2aCall(t *testing.T) {
	t.Parallel()

	t.Run("successful call", func(t *testing.T) {
		t.Parallel()
		receipt := &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "a2a result",
		}
		proc := &fakeEnvelopeProcessor{receipt: receipt}
		g := newTestGatewayService(t, withEnvProc(proc))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test-skill","payload":{"arg":"val"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/a2a/call", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleA2aCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("missing skill_name", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t)

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"payload":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/a2a/call", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleA2aCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "skill_name required")
	})

	t.Run("L3 proof missing - suspension", func(t *testing.T) {
		t.Parallel()
		proc := &fakeEnvelopeProcessor{err: governance.ErrL3ProofMissing}
		store := &fakeSuspendedStore{}
		g := newTestGatewayService(t, withEnvProc(proc), withSuspendedStore(store))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test-skill","payload":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/a2a/call", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleA2aCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		require.Len(t, store.txs, 1)
	})
}

func TestGatewayService_DispatchToDownstream(t *testing.T) {
	t.Parallel()

	t.Run("successful dispatch", func(t *testing.T) {
		t.Parallel()
		// Mock downstream MCP server
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"tool output"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		result, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{"arg":"val"}`), "test-session-id")
		require.NoError(t, err)
		require.Contains(t, result, "tool output")
	})

	t.Run("no downstream configured", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL(""))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no downstream MCP server configured")
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"), withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "circuit open")
	})

	t.Run("HTTP connection failure", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("non-200 status code", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("MCP error response", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "MCP error")
	})

}

func TestGatewayService_DispatchToA2ADownstream(t *testing.T) {
	t.Parallel()

	t.Run("successful dispatch with result", func(t *testing.T) {
		t.Parallel()
		// Mock downstream A2A server
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":"skill output"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{"arg":"val"}`))
		require.NoError(t, err)
		require.Equal(t, "skill output", result)
	})

	t.Run("successful dispatch with summary", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"summary":"skill summary"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Equal(t, "skill summary", result)
	})

	t.Run("successful dispatch with empty response", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Equal(t, "completed", result)
	})

	t.Run("no downstream configured", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withA2ADownstreamURL(""))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "no downstream A2A server configured")
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withA2ADownstreamURL("http://localhost:9999"), withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "circuit open")
	})

	t.Run("HTTP connection failure", func(t *testing.T) {
		t.Parallel()
		g := newTestGatewayService(t, withA2ADownstreamURL("http://localhost:9999"), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("non-200 status code", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("A2A error response", func(t *testing.T) {
		t.Parallel()
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"error":"skill execution failed"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "A2A error")
	})

}

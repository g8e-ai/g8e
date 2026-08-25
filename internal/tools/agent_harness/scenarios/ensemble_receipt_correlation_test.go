// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// fakeMCPCallServer returns an httptest server that handles audit_receipt_list MCP calls
// returning the specified receipts list.
func fakeAuditReceiptListServer(t *testing.T, receipts []map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPaths.MCPEndpoint {
			http.NotFound(w, r)
			return
		}

		receiptsRaw := make([]json.RawMessage, 0, len(receipts))
		for _, rec := range receipts {
			b, err := json.Marshal(rec)
			if err != nil {
				t.Fatalf("marshal receipt: %v", err)
			}
			receiptsRaw = append(receiptsRaw, b)
		}

		listResult := map[string]any{
			"receipts": receiptsRaw,
			"count":    len(receiptsRaw),
		}
		listResultBytes, _ := json.Marshal(listResult)

		callResult := map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": string(listResultBytes),
				},
			},
			"isError": false,
		}
		callResultBytes, _ := json.Marshal(callResult)

		mcpResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  json.RawMessage(callResultBytes),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcpResp)
	}))
}

func TestReceiptCorrelation_RejectsStaleReceiptBeforeNotBefore(t *testing.T) {
	now := time.Now()
	staleTime := now.Add(-10 * time.Minute)

	receipts := []map[string]any{
		{
			"transaction_id":      "stale-tx-1",
			"action_type":         string(constants.ActionTypeFileEdit),
			"target_resource":     "/tmp/file.txt",
			"operator_session_id": "sess-1",
			"signature":           "deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678",
			"executed_at":         staleTime.Format(time.RFC3339Nano),
			"status":              int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}

	server := fakeAuditReceiptListServer(t, receipts)
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := &Result{}
	corr := ReceiptCorrelation{
		NotBefore:         now,
		OperatorSessionID: "sess-1",
		ActionType:        string(constants.ActionTypeFileEdit),
		TargetResource:    "/tmp/file.txt",
		Persona:           clientpkg.Persona{ID: "test"},
	}

	// We set a very short poll interval or timeout by context
	_, err = pollForReceiptWithCorrelation(ctx, client, res, corr)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, constants.ErrHarnessEnsembleReceiptTimeout))
}

func TestReceiptCorrelation_RejectsUnrelatedActionType(t *testing.T) {
	now := time.Now()

	receipts := []map[string]any{
		{
			"transaction_id":      "diff-action-tx",
			"action_type":         string(constants.ActionTypeFsRead),
			"target_resource":     "/tmp/file.txt",
			"operator_session_id": "sess-1",
			"signature":           "deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678",
			"executed_at":         now.Add(1 * time.Second).Format(time.RFC3339Nano),
			"status":              int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}

	server := fakeAuditReceiptListServer(t, receipts)
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := &Result{}
	corr := ReceiptCorrelation{
		NotBefore:         now,
		OperatorSessionID: "sess-1",
		ActionType:        string(constants.ActionTypeFileEdit),
		TargetResource:    "/tmp/file.txt",
		Persona:           clientpkg.Persona{ID: "test"},
	}

	_, err = pollForReceiptWithCorrelation(ctx, client, res, corr)
	assert.Error(t, err)
}

func TestReceiptCorrelation_RejectsMismatchedOperatorSessionID(t *testing.T) {
	now := time.Now()

	receipts := []map[string]any{
		{
			"transaction_id":      "diff-session-tx",
			"action_type":         string(constants.ActionTypeFileEdit),
			"target_resource":     "/tmp/file.txt",
			"operator_session_id": "other-session",
			"signature":           "deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678",
			"executed_at":         now.Add(1 * time.Second).Format(time.RFC3339Nano),
			"status":              int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}

	server := fakeAuditReceiptListServer(t, receipts)
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := &Result{}
	corr := ReceiptCorrelation{
		NotBefore:         now,
		OperatorSessionID: "expected-session",
		ActionType:        string(constants.ActionTypeFileEdit),
		TargetResource:    "/tmp/file.txt",
		Persona:           clientpkg.Persona{ID: "test"},
	}

	_, err = pollForReceiptWithCorrelation(ctx, client, res, corr)
	assert.Error(t, err)
}

func TestReceiptCorrelation_RejectsUnsignedReceipt(t *testing.T) {
	now := time.Now()

	receipts := []map[string]any{
		{
			"transaction_id":      "unsigned-tx",
			"action_type":         string(constants.ActionTypeFileEdit),
			"target_resource":     "/tmp/file.txt",
			"operator_session_id": "sess-1",
			"signature":           "", // empty signature
			"executed_at":         now.Add(1 * time.Second).Format(time.RFC3339Nano),
			"status":              int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}

	server := fakeAuditReceiptListServer(t, receipts)
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := &Result{}
	corr := ReceiptCorrelation{
		NotBefore:         now,
		OperatorSessionID: "sess-1",
		ActionType:        string(constants.ActionTypeFileEdit),
		TargetResource:    "/tmp/file.txt",
		Persona:           clientpkg.Persona{ID: "test"},
	}

	_, err = pollForReceiptWithCorrelation(ctx, client, res, corr)
	assert.Error(t, err)
}

func TestReceiptCorrelation_FailsImmediatelyOnFailedReceipt(t *testing.T) {
	now := time.Now()

	receipts := []map[string]any{
		{
			"transaction_id":      "failed-tx",
			"action_type":         string(constants.ActionTypeFileEdit),
			"target_resource":     "/tmp/file.txt",
			"operator_session_id": "sess-1",
			"signature":           "deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678deadbeef12345678",
			"executed_at":         now.Add(1 * time.Second).Format(time.RFC3339Nano),
			"status":              int(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED),
			"result_summary":      "permission denied",
		},
	}

	server := fakeAuditReceiptListServer(t, receipts)
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := &Result{}
	corr := ReceiptCorrelation{
		NotBefore:         now,
		OperatorSessionID: "sess-1",
		ActionType:        string(constants.ActionTypeFileEdit),
		TargetResource:    "/tmp/file.txt",
		Persona:           clientpkg.Persona{ID: "test"},
	}

	_, err = pollForReceiptWithCorrelation(ctx, client, res, corr)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrHarnessEnsembleReceiptFailed))
	assert.Contains(t, err.Error(), "permission denied")
}

func TestVerifyReceiptIdentity_RejectsEmptyOrMismatchedRequestorUserID(t *testing.T) {
	oldKit := kit
	defer func() { kit = oldKit }()

	kit = &GovKit{UserID: "expected-user-123"}
	res := &Result{}

	t.Run("empty requestor_user_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt requestor_user_id is empty")
	})

	t.Run("mismatched requestor_user_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "wrong-user",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt requestor_user_id=\"wrong-user\" != expected=\"expected-user-123\"")
	})

	t.Run("empty acting_app_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "expected-user-123",
			ActingAppID:     "",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt acting_app_id is empty")
	})

	t.Run("valid matching identity passes", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "expected-user-123",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.NoError(t, err)
	})
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAuditReceiptQuery is a Tier 1 stub implementation of AuditReceiptQuery
// for unit testing the audit receipt native tools. It returns canned records
// and records the arguments it was called with so tests can assert dispatch.
type stubAuditReceiptQuery struct {
	listBySession []*models.ActionReceiptRecord
	listSince     []*models.ActionReceiptRecord
	getByID       map[string]*models.ActionReceiptRecord
	listErr       error
	listSinceErr  error
	getErr        error

	lastListSession string
	lastListLimit   int
	lastListOffset  int
	lastSinceTime   time.Time
	lastSinceLimit  int
	lastGetID       string
}

func (s *stubAuditReceiptQuery) ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error) {
	s.lastListSession = operatorSessionID
	s.lastListLimit = limit
	s.lastListOffset = offset
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listBySession, nil
}

func (s *stubAuditReceiptQuery) ListActionReceiptsSince(since time.Time, limit int) ([]*models.ActionReceiptRecord, error) {
	s.lastSinceTime = since
	s.lastSinceLimit = limit
	if s.listSinceErr != nil {
		return nil, s.listSinceErr
	}
	return s.listSince, nil
}

func (s *stubAuditReceiptQuery) GetActionReceipt(transactionID string) (*models.ActionReceiptRecord, error) {
	s.lastGetID = transactionID
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getByID[transactionID], nil
}

func sampleReceiptRecord(txID, sessionID, actionType string, executedAt time.Time, status operatorv1.ExecutionStatus) *models.ActionReceiptRecord {
	return &models.ActionReceiptRecord{
		TransactionID:     txID,
		TransactionHash:   "hash-" + txID,
		OperatorID:        "op-1",
		OperatorSessionID: sessionID,
		RequestorUserID:   "user-1",
		ActingAppID:       "app-1",
		ActionType:        constants.ActionType(actionType),
		TargetResource:    "/tmp/file.txt",
		Status:            status,
		ResultSummary:     "ok",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after",
		ExecutedAt:        executedAt,
		SignerKeyID:       "key-1",
		Signature:         "sig-" + txID,
		Timestamp:         executedAt,
	}
}

func TestAuditReceiptListTool_Name_Description_Schema(t *testing.T) {
	tool := &AuditReceiptListTool{}
	assert.Equal(t, "audit_receipt_list", tool.Name())
	assert.NotEmpty(t, tool.Description())
	schema := tool.InputSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "operator_session_id")
	assert.Contains(t, schema.Required, "operator_session_id")
}

func TestAuditReceiptListTool_Execute_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	stub := &stubAuditReceiptQuery{
		listBySession: []*models.ActionReceiptRecord{
			sampleReceiptRecord("tx-1", "sess-1", "FILE_EDIT", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
			sampleReceiptRecord("tx-2", "sess-1", "DOCUMENT_UPDATE", now.Add(-time.Minute), operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1"})

	tool := &AuditReceiptListTool{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.False(t, result.IsError)

	var lr AuditReceiptListResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &lr))
	assert.Len(t, lr.Receipts, 2)
	assert.Equal(t, 2, lr.Count)
	assert.Equal(t, "sess-1", stub.lastListSession)
	assert.Equal(t, 50, stub.lastListLimit)
	assert.Equal(t, 0, stub.lastListOffset)
}

func TestAuditReceiptListTool_Execute_ActionTypeFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	stub := &stubAuditReceiptQuery{
		listBySession: []*models.ActionReceiptRecord{
			sampleReceiptRecord("tx-1", "sess-1", "FILE_EDIT", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
			sampleReceiptRecord("tx-2", "sess-1", "DOCUMENT_UPDATE", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1", ActionType: "FILE_EDIT"})

	tool := &AuditReceiptListTool{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var lr AuditReceiptListResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &lr))
	require.Len(t, lr.Receipts, 1)
	assert.Equal(t, "tx-1", lr.Receipts[0].TransactionID)
	assert.Equal(t, "FILE_EDIT", string(lr.Receipts[0].ActionType))
}

func TestAuditReceiptListTool_Execute_NotBeforeFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	stub := &stubAuditReceiptQuery{
		listBySession: []*models.ActionReceiptRecord{
			sampleReceiptRecord("tx-old", "sess-1", "FILE_EDIT", now.Add(-2*time.Hour), operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
			sampleReceiptRecord("tx-new", "sess-1", "FILE_EDIT", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptListRequest{
		OperatorSessionID: "sess-1",
		ActionType:        "FILE_EDIT",
		NotBefore:         now.Add(-time.Hour).Format(time.RFC3339),
	})

	tool := &AuditReceiptListTool{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var lr AuditReceiptListResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &lr))
	require.Len(t, lr.Receipts, 1)
	assert.Equal(t, "tx-new", lr.Receipts[0].TransactionID)
}

func TestAuditReceiptListTool_Execute_Pagination(t *testing.T) {
	stub := &stubAuditReceiptQuery{listBySession: nil}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1", Limit: 10, Offset: 20})

	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.Equal(t, 10, stub.lastListLimit)
	assert.Equal(t, 20, stub.lastListOffset)
}

func TestAuditReceiptListTool_Execute_MissingOperatorSessionID(t *testing.T) {
	ctx := withAuditReceiptQuery(context.Background(), &stubAuditReceiptQuery{})
	args, _ := json.Marshal(AuditReceiptListRequest{})

	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptListOperatorSessionReq))
}

func TestAuditReceiptListTool_Execute_QueryNotConfigured(t *testing.T) {
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1"})

	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptQueryNotConfigured))
}

func TestAuditReceiptListTool_Execute_InvalidNotBefore(t *testing.T) {
	ctx := withAuditReceiptQuery(context.Background(), &stubAuditReceiptQuery{})
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1", NotBefore: "not-a-timestamp"})

	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptParseNotBefore))
}

func TestAuditReceiptListTool_Execute_QueryError(t *testing.T) {
	queryErr := errors.New("db offline")
	stub := &stubAuditReceiptQuery{listErr: queryErr}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1"})

	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	assert.ErrorIs(t, err, queryErr)
}

func TestAuditReceiptListTool_Execute_InvalidJSON(t *testing.T) {
	ctx := withAuditReceiptQuery(context.Background(), &stubAuditReceiptQuery{})
	tool := &AuditReceiptListTool{}
	_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
	require.Error(t, err)
}

func TestAuditReceiptGetTool_Name_Description_Schema(t *testing.T) {
	tool := &AuditReceiptGetTool{}
	assert.Equal(t, "audit_receipt_get", tool.Name())
	assert.NotEmpty(t, tool.Description())
	schema := tool.InputSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "transaction_id")
	assert.Contains(t, schema.Required, "transaction_id")
}

func TestAuditReceiptGetTool_Execute_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	rec := sampleReceiptRecord("tx-1", "sess-1", "FILE_EDIT", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED)
	stub := &stubAuditReceiptQuery{getByID: map[string]*models.ActionReceiptRecord{"tx-1": rec}}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptGetRequest{TransactionID: "tx-1"})

	tool := &AuditReceiptGetTool{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.False(t, result.IsError)

	var gr AuditReceiptGetResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &gr))
	require.NotNil(t, gr.Receipt)
	assert.Equal(t, "tx-1", gr.Receipt.TransactionID)
	assert.Equal(t, "sess-1", gr.Receipt.OperatorSessionID)
	assert.Equal(t, "tx-1", stub.lastGetID)
}

func TestAuditReceiptGetTool_Execute_NotFound(t *testing.T) {
	stub := &stubAuditReceiptQuery{getByID: map[string]*models.ActionReceiptRecord{}}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptGetRequest{TransactionID: "missing"})

	tool := &AuditReceiptGetTool{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var gr AuditReceiptGetResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &gr))
	assert.Nil(t, gr.Receipt)
}

func TestAuditReceiptGetTool_Execute_MissingTransactionID(t *testing.T) {
	ctx := withAuditReceiptQuery(context.Background(), &stubAuditReceiptQuery{})
	args, _ := json.Marshal(AuditReceiptGetRequest{})

	tool := &AuditReceiptGetTool{}
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptGetTransactionIDReq))
}

func TestAuditReceiptGetTool_Execute_QueryNotConfigured(t *testing.T) {
	args, _ := json.Marshal(AuditReceiptGetRequest{TransactionID: "tx-1"})

	tool := &AuditReceiptGetTool{}
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptQueryNotConfigured))
}

func TestAuditReceiptGetTool_Execute_QueryError(t *testing.T) {
	queryErr := errors.New("db offline")
	stub := &stubAuditReceiptQuery{getErr: queryErr}
	ctx := withAuditReceiptQuery(context.Background(), stub)
	args, _ := json.Marshal(AuditReceiptGetRequest{TransactionID: "tx-1"})

	tool := &AuditReceiptGetTool{}
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	assert.ErrorIs(t, err, queryErr)
}

func TestAuditReceiptGetTool_Execute_InvalidJSON(t *testing.T) {
	ctx := withAuditReceiptQuery(context.Background(), &stubAuditReceiptQuery{})
	tool := &AuditReceiptGetTool{}
	_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
	require.Error(t, err)
}

// TestNativeToolHandler_SetAuditReceiptQuery verifies the handler injects the
// configured AuditReceiptQuery into the execution context so the audit
// receipt tools can access the operator audit vault.
func TestNativeToolHandler_SetAuditReceiptQuery(t *testing.T) {
	handler, err := NewNativeToolHandler(nil)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Millisecond)
	stub := &stubAuditReceiptQuery{
		listBySession: []*models.ActionReceiptRecord{
			sampleReceiptRecord("tx-1", "sess-1", "FILE_EDIT", now, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED),
		},
	}
	handler.SetAuditReceiptQuery(stub)

	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1"})
	result, err := handler.HandleTool(context.Background(), "audit_receipt_list", args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var lr AuditReceiptListResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &lr))
	assert.Len(t, lr.Receipts, 1)
}

// TestNativeToolHandler_AuditReceiptQueryNotConfigured verifies the handler
// fails closed when no AuditReceiptQuery is wired, proving the audit receipt
// tools cannot execute without a configured audit vault.
func TestNativeToolHandler_AuditReceiptQueryNotConfigured(t *testing.T) {
	handler, err := NewNativeToolHandler(nil)
	require.NoError(t, err)

	args, _ := json.Marshal(AuditReceiptListRequest{OperatorSessionID: "sess-1"})
	_, err = handler.HandleTool(context.Background(), "audit_receipt_list", args)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPAuditReceiptQueryNotConfigured))
}

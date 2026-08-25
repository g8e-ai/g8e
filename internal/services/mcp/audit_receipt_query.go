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
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// AuditReceiptQuery is the read interface into the operator audit vault for
// the governed native audit receipt tools. It is implemented by
// *storage.SQLAuditStore in production and by stubs in unit tests. The
// interface is intentionally narrow: only the receipt retrieval methods the
// native tools require are exposed, so the tools cannot mutate audit state.
type AuditReceiptQuery interface {
	ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error)
	ListActionReceiptsSince(since time.Time, limit int) ([]*models.ActionReceiptRecord, error)
	GetActionReceipt(transactionID string) (*models.ActionReceiptRecord, error)
}

// auditReceiptQueryCtxKey is the context key under which the
// NativeToolHandler injects the configured AuditReceiptQuery so the audit
// receipt tools can retrieve it during execution. It is unexported to keep
// the injection mechanism internal to the mcp package.
type auditReceiptQueryCtxKey struct{}

// withAuditReceiptQuery returns a context carrying the provided
// AuditReceiptQuery so audit receipt tools can access the operator audit
// vault during native tool execution.
func withAuditReceiptQuery(ctx context.Context, q AuditReceiptQuery) context.Context {
	return context.WithValue(ctx, auditReceiptQueryCtxKey{}, q)
}

// auditReceiptQueryFromContext returns the AuditReceiptQuery injected by the
// NativeToolHandler, or nil if none is configured. Tools fail closed when
// this returns nil — a missing audit store is a wiring bug, not a no-op
// condition.
func auditReceiptQueryFromContext(ctx context.Context) AuditReceiptQuery {
	q, _ := ctx.Value(auditReceiptQueryCtxKey{}).(AuditReceiptQuery)
	return q
}

// AuditReceiptListTool queries signed ActionReceipt records from the operator
// audit vault under governed MCP tool dispatch. It is the governed
// replacement for unauthenticated HTTP audit polling: every invocation
// traverses the L1–L5 gauntlet and produces its own audit record.
type AuditReceiptListTool struct{}

// Name returns the tool identifier.
func (t *AuditReceiptListTool) Name() string {
	return "audit_receipt_list"
}

// Description returns a human-readable description.
func (t *AuditReceiptListTool) Description() string {
	return "Lists signed ActionReceipt records from the operator audit vault, scoped to an operator session. Supports optional action_type and not_before (RFC3339) filtering with limit/offset pagination."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *AuditReceiptListTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"operator_session_id": {
				Type:        "string",
				Description: "Operator session ID to scope the receipt query (required).",
			},
			"action_type": {
				Type:        "string",
				Description: "Optional action type filter (e.g. FILE_EDIT, DOCUMENT_UPDATE).",
			},
			"not_before": {
				Type:        "string",
				Description: "Optional RFC3339 timestamp; only receipts executed at or after this time are returned.",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of receipts to return (default 50).",
			},
			"offset": {
				Type:        "integer",
				Description: "Pagination offset (default 0).",
			},
		},
		Required: []string{"operator_session_id"},
	}
}

// AuditReceiptListRequest is the params for the "audit_receipt_list" tool.
type AuditReceiptListRequest struct {
	OperatorSessionID string `json:"operator_session_id"`
	ActionType        string `json:"action_type,omitempty"`
	NotBefore         string `json:"not_before,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Offset            int    `json:"offset,omitempty"`
}

// AuditReceiptListResult is the result for the "audit_receipt_list" tool.
type AuditReceiptListResult struct {
	Receipts []*models.ActionReceiptRecord `json:"receipts"`
	Count    int                           `json:"count"`
	Error    string                        `json:"error,omitempty"`
}

// Execute implements the tool logic.
func (t *AuditReceiptListTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req AuditReceiptListRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_list: unmarshal arguments: %w", err)
	}

	if req.OperatorSessionID == "" {
		return CallToolResult{}, fmt.Errorf("audit_receipt_list: %w", constants.ErrMCPAuditReceiptListOperatorSessionReq)
	}

	q := auditReceiptQueryFromContext(ctx)
	if q == nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_list: %w", constants.ErrMCPAuditReceiptQueryNotConfigured)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	receipts, err := q.ListActionReceipts(req.OperatorSessionID, limit, req.Offset)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_list: query: %w", err)
	}

	var notBefore time.Time
	if req.NotBefore != "" {
		parsed, perr := time.Parse(time.RFC3339, req.NotBefore)
		if perr != nil {
			return CallToolResult{}, fmt.Errorf("audit_receipt_list: %w: %w", constants.ErrMCPAuditReceiptParseNotBefore, perr)
		}
		notBefore = parsed
	}

	filtered := make([]*models.ActionReceiptRecord, 0, len(receipts))
	for _, rec := range receipts {
		if req.ActionType != "" && string(rec.ActionType) != req.ActionType {
			continue
		}
		if !notBefore.IsZero() && !rec.ExecutedAt.IsZero() && rec.ExecutedAt.Before(notBefore) {
			continue
		}
		filtered = append(filtered, rec)
	}

	result := AuditReceiptListResult{
		Receipts: filtered,
		Count:    len(filtered),
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_list: %w: %w", constants.ErrMCPAuditReceiptMarshalResult, err)
	}

	return CallToolResult{
		Content: []TextContent{
			{Type: "text", Text: string(resultJSON)},
		},
	}, nil
}

// AuditReceiptGetTool retrieves a single signed ActionReceipt by transaction
// ID from the operator audit vault under governed MCP tool dispatch.
type AuditReceiptGetTool struct{}

// Name returns the tool identifier.
func (t *AuditReceiptGetTool) Name() string {
	return "audit_receipt_get"
}

// Description returns a human-readable description.
func (t *AuditReceiptGetTool) Description() string {
	return "Retrieves a single signed ActionReceipt by transaction_id from the operator audit vault."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *AuditReceiptGetTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"transaction_id": {
				Type:        "string",
				Description: "Transaction ID of the receipt to retrieve (required).",
			},
		},
		Required: []string{"transaction_id"},
	}
}

// AuditReceiptGetRequest is the params for the "audit_receipt_get" tool.
type AuditReceiptGetRequest struct {
	TransactionID string `json:"transaction_id"`
}

// AuditReceiptGetResult is the result for the "audit_receipt_get" tool.
type AuditReceiptGetResult struct {
	Receipt *models.ActionReceiptRecord `json:"receipt,omitempty"`
	Error   string                      `json:"error,omitempty"`
}

// Execute implements the tool logic.
func (t *AuditReceiptGetTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req AuditReceiptGetRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_get: unmarshal arguments: %w", err)
	}

	if req.TransactionID == "" {
		return CallToolResult{}, fmt.Errorf("audit_receipt_get: %w", constants.ErrMCPAuditReceiptGetTransactionIDReq)
	}

	q := auditReceiptQueryFromContext(ctx)
	if q == nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_get: %w", constants.ErrMCPAuditReceiptQueryNotConfigured)
	}

	receipt, err := q.GetActionReceipt(req.TransactionID)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_get: query: %w", err)
	}

	result := AuditReceiptGetResult{Receipt: receipt}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("audit_receipt_get: %w: %w", constants.ErrMCPAuditReceiptMarshalResult, err)
	}

	return CallToolResult{
		Content: []TextContent{
			{Type: "text", Text: string(resultJSON)},
		},
	}, nil
}

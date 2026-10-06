// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// gateway issues the TUI's Gateway requests over the CLI session. Each
// request is the one the matching CLI command makes, so the TUI and the CLI
// see the same data through the same endpoints.
type gateway struct {
	session Session
	userID  string
}

// fetchPendingApprovals lists the session user's pending L3 transactions,
// as 'g8e auth approve' expects them.
func (g *gateway) fetchPendingApprovals(ctx context.Context) PendingApprovalsMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.ApprovalsCLIList, nil)
	if err != nil {
		return PendingApprovalsMsg{Err: err}
	}
	var resp models.SuspendedTransactionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return PendingApprovalsMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return PendingApprovalsMsg{Transactions: resp.Transactions}
}

// fetchOperators lists the session user's Operators, as 'g8e gw status' does.
func (g *gateway) fetchOperators(ctx context.Context) OperatorsMsg {
	path := constants.APIPaths.Operators
	if g.userID != "" {
		path += "?user_id=" + url.QueryEscape(g.userID)
	}
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return OperatorsMsg{Err: err}
	}
	var resp models.OperatorSlotResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return OperatorsMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return OperatorsMsg{Operators: resp.Operators}
}

// fetchHealth reads the Gateway's health report, which carries its
// governance posture.
func (g *gateway) fetchHealth(ctx context.Context) HealthMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.Health, nil)
	if err != nil {
		return HealthMsg{Err: err}
	}
	var health models.HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return HealthMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return HealthMsg{Health: health}
}

// verifyApproval confirms a transaction's approval after approval.completed,
// exactly as 'g8e auth approve' does.
func (g *gateway) verifyApproval(ctx context.Context, txHash string) ApprovalVerifiedMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, auth.ApprovalStatusPath(txHash), nil)
	if err != nil {
		return ApprovalVerifiedMsg{TxHash: txHash, Err: fmt.Errorf("verify status: %w", err)}
	}
	status, err := auth.VerifyApprovalStatus(txHash, body)
	return ApprovalVerifiedMsg{TxHash: txHash, Status: status, Err: err}
}

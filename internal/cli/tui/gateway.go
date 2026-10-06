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
	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
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

// fetchEnrollments lists the pending platform enrollment requests and the
// completed enrollments, as 'g8e auth enroll pending' and 'g8e auth enroll
// list' do.
func (g *gateway) fetchEnrollments(ctx context.Context) EnrollmentsMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	if err != nil {
		return EnrollmentsMsg{Err: fmt.Errorf("fetch pending list: %w", err)}
	}
	var pending models.PlatformEnrollmentPendingResponse
	if err := json.Unmarshal(body, &pending); err != nil {
		return EnrollmentsMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	body, err = g.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentEnrolled, nil)
	if err != nil {
		return EnrollmentsMsg{Err: fmt.Errorf("fetch enrolled list: %w", err)}
	}
	var enrolled models.PlatformEnrollmentEnrolledResponse
	if err := json.Unmarshal(body, &enrolled); err != nil {
		return EnrollmentsMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return EnrollmentsMsg{Pending: pending.Requests, Enrolled: enrolled.Enrollments}
}

// decideEnrollment posts an owner decision on a pending platform enrollment
// request, as 'g8e auth enroll approve|deny' does.
func (g *gateway) decideEnrollment(ctx context.Context, req models.PlatformEnrollmentDecisionRequest) EnrollmentDecidedMsg {
	msg := EnrollmentDecidedMsg{RequestID: req.RequestID, Decision: req.Decision}
	if err := req.Validate(); err != nil {
		msg.Err = fmt.Errorf("validate decision: %w", err)
		return msg
	}
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, req)
	if err != nil {
		msg.Err = fmt.Errorf("post decision: %w", err)
		return msg
	}
	msg.Response, msg.Err = auth.DecodePlatformEnrollmentDecision(body)
	return msg
}

// revokeEnrollment revokes a completed platform enrollment, as 'g8e auth
// enroll revoke' does.
func (g *gateway) revokeEnrollment(ctx context.Context, req models.PlatformEnrollmentRevokeRequest) EnrollmentRevokedMsg {
	msg := EnrollmentRevokedMsg{RequestID: req.RequestID}
	if err := req.Validate(); err != nil {
		msg.Err = err
		return msg
	}
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRevoke, req)
	if err != nil {
		msg.Err = fmt.Errorf("post revocation: %w", err)
		return msg
	}
	msg.Response, msg.Err = auth.DecodePlatformEnrollmentRevoke(body)
	return msg
}

// stopOperator asks the Gateway for a governed shutdown of a remote Operator,
// as 'g8e operator stop <session_id>' does.
func (g *gateway) stopOperator(ctx context.Context, operatorSessionID, reason string) OperatorStopMsg {
	msg := OperatorStopMsg{OperatorSessionID: operatorSessionID}
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.OperatorsStop, clioperator.NewStopRequest(operatorSessionID, reason))
	if err != nil {
		msg.Err = fmt.Errorf("operator stop: request shutdown: %w", err)
		return msg
	}
	msg.Response, msg.Err = clioperator.DecodeStopResponse(body)
	return msg
}

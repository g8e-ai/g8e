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

// bindOperator binds the CLI session to one Operator session, matching the
// request and response validation used by 'g8e operator bind'.
func (g *gateway) bindOperator(ctx context.Context, operatorSessionID string) OperatorBindMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.AuthCLIBind, models.CLIBindRequest{OperatorSessionIDs: []string{operatorSessionID}})
	if err != nil {
		return OperatorBindMsg{Err: fmt.Errorf("operator bind: %w", err)}
	}
	response, err := auth.DecodeCLIBindResponse(body, []string{operatorSessionID})
	return OperatorBindMsg{Response: response, Err: err}
}

// unbindOperator clears the CLI session's Operator binding, matching
// 'g8e operator bind unbind'.
func (g *gateway) unbindOperator(ctx context.Context) OperatorUnbindMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.AuthCLIUnbind, models.CLIUnbindRequest{})
	if err != nil {
		return OperatorUnbindMsg{Err: fmt.Errorf("operator unbind: %w", err)}
	}
	response, err := auth.DecodeCLIUnbindResponse(body)
	return OperatorUnbindMsg{Response: response, Err: err}
}

// approveRecovery posts the CLI recovery decision, matching
// 'g8e auth approve-recovery <token> [--deny]'.
func (g *gateway) approveRecovery(ctx context.Context, token string, approve bool) RecoveryApprovedMsg {
	msg := RecoveryApprovedMsg{Approve: approve}
	body, err := g.session.DoRequestContext(ctx, http.MethodPost, constants.APIPaths.AuthCLIRecoveryApproveCLI, models.CLIRecoveryApproveRequest{Token: token, Approve: approve})
	if err != nil {
		msg.Err = fmt.Errorf("approve-recovery: post approve: %w", err)
		return msg
	}
	if err := json.Unmarshal(body, &msg.Response); err != nil {
		msg.Err = fmt.Errorf("approve-recovery: parse response: %w", err)
		return msg
	}
	if msg.Response.State != models.CLIRecoveryStateApproved && msg.Response.State != models.CLIRecoveryStateDenied {
		msg.Err = fmt.Errorf("approve-recovery: %w: state %q", constants.ErrCLIRecoveryRequestFailed, msg.Response.State)
	}
	return msg
}

const auditPageSize = 10

// fetchAuditEvents lists one page of the Gateway audit event store, matching
// 'g8e audit events' while using the TUI's enrolled CLI session.
func (g *gateway) fetchAuditEvents(ctx context.Context, offset int) AuditEventsMsg {
	query := url.Values{}
	query.Set("limit", fmt.Sprint(auditPageSize))
	query.Set("offset", fmt.Sprint(max(offset, 0)))
	path := constants.APIPaths.AuditEvents + "?" + query.Encode()
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return AuditEventsMsg{Offset: offset, Err: fmt.Errorf("fetch audit events: %w", err)}
	}
	var resp models.AuditEventsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return AuditEventsMsg{Offset: offset, Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return AuditEventsMsg{Events: resp.Events, Count: resp.Count, Offset: offset}
}

// fetchAuditSummary reads the aggregate Gateway audit summary, matching
// 'g8e audit summary'.
func (g *gateway) fetchAuditSummary(ctx context.Context) AuditSummaryMsg {
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.AuditSummary, nil)
	if err != nil {
		return AuditSummaryMsg{Err: fmt.Errorf("fetch audit summary: %w", err)}
	}
	var summary models.AuditSummaryResponse
	if err := json.Unmarshal(body, &summary); err != nil {
		return AuditSummaryMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return AuditSummaryMsg{Summary: summary}
}

// verifyAudit verifies the Gateway audit hash chain, matching
// 'g8e audit verify'.
func (g *gateway) verifyAudit(ctx context.Context, fromSeq int64) AuditVerifyMsg {
	path := constants.APIPaths.AuditVerify
	if fromSeq > 0 {
		path += "?from_seq=" + fmt.Sprint(fromSeq)
	}
	body, err := g.session.DoRequestContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return AuditVerifyMsg{Err: fmt.Errorf("verify audit chain: %w", err)}
	}
	var verify models.AuditVerifyResponse
	if err := json.Unmarshal(body, &verify); err != nil {
		return AuditVerifyMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return AuditVerifyMsg{Verify: verify}
}

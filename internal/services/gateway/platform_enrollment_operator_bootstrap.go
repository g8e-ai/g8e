// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Runtime grant delivered with every Operator bundle.
const (
	operatorBootstrapMaxConcurrentTasks = 25
	operatorBootstrapMaxMemoryMB        = 2048
)

// operatorBootstrapCloseWait bounds how long the Gateway waits to hand the
// close frame to a peer after the exchange ends.
const operatorBootstrapCloseWait = 5 * time.Second

// handleOperatorBootstrap runs one Operator's enrollment over a websocket and
// delivers its PEM bundle exactly once. The frame sequence is documented on
// models.OperatorBootstrapFrameType. The request is the platform enrollment
// request, so creation, owner decision, proof-of-possession completion, and
// the issuance saga are the same governed path as HTTP enrollment; the socket
// only replaces the HTTP round trips. A peer that drops is redialed with the
// same token and CSRs: creation recovers the live request, a pending decision
// is waited on again, and completion returns the stored issuance.
//
// GET /ws/operator/bootstrap  (RouteAuthNone; plain HTTP and HTTPS)
func (c *PlatformEnrollmentController) handleOperatorBootstrap(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Warn("operator bootstrap: websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(c.cfg.Gateway.MaxPayloadBytes)

	// A hijacked connection outlives the request context's disconnect
	// signal, so the read loop ends the exchange when the peer goes away.
	ctx, cancel := context.WithCancelCause(r.Context())
	defer cancel(nil)
	frames := make(chan models.OperatorBootstrapFrame, 1)
	go func() {
		defer close(frames)
		for {
			var frame models.OperatorBootstrapFrame
			if err := conn.ReadJSON(&frame); err != nil {
				cancel(fmt.Errorf("operator bootstrap: peer closed: %w", err))
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	exchange := operatorBootstrapExchange{c: c, conn: conn, frames: frames}
	if err := exchange.run(ctx); err != nil {
		if ctx.Err() != nil {
			c.logger.Info("operator bootstrap: exchange ended", "cause", context.Cause(ctx))
			return
		}
		c.logger.Warn("operator bootstrap: exchange failed", "error", err)
		exchange.writeError(err)
	}
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(operatorBootstrapCloseWait))
}

type operatorBootstrapExchange struct {
	c      *PlatformEnrollmentController
	conn   *websocket.Conn
	frames <-chan models.OperatorBootstrapFrame
}

func (e operatorBootstrapExchange) run(ctx context.Context) error {
	frame, err := e.next(ctx, models.OperatorBootstrapFrameRequest)
	if err != nil {
		return err
	}
	if frame.Request == nil {
		return constants.ErrPlatformEnrollmentInvalidPayload
	}
	token := frame.Request.Token
	enrollment := frame.Request.Enrollment
	if token == "" {
		return constants.ErrPlatformEnrollmentTokenRequired
	}
	if !constantTimeEqual(models.PlatformEnrollmentTokenHash(token), enrollment.TokenHash) {
		return constants.ErrPlatformEnrollmentInvalidToken
	}
	if enrollment.ComponentKind != models.PlatformComponentOperator {
		return constants.ErrPlatformEnrollmentInvalidComponent
	}
	if err := enrollment.ValidateShape(); err != nil {
		return err
	}
	runtimeConfig, err := operatorBootstrapRuntimeConfig(frame.Request)
	if err != nil {
		return err
	}

	// Request creation is rejected before bootstrap; an Operator started
	// ahead of the owner holds here until the first user exists.
	if err := e.c.userSvc.WaitForAnyUser(ctx); err != nil {
		return err
	}
	created, err := e.c.enrollSvc.CreateRequest(ctx, enrollment, e.c.approvalURLBase())
	if errors.Is(err, constants.ErrPlatformEnrollmentTokenConflict) {
		// A redial after the request left the live states (approval,
		// issuance, completion) resumes the request this token owns.
		created, err = e.c.enrollSvc.ResumeRequest(ctx, token, e.c.approvalURLBase())
	}
	if err != nil {
		return err
	}
	if err := e.write(models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameCreated, Created: created}); err != nil {
		return err
	}

	decision, err := e.c.enrollSvc.WaitForDecision(ctx, enrollment.TokenHash)
	if err != nil {
		return err
	}
	if err := e.write(models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameDecision, Decision: decision}); err != nil {
		return err
	}
	switch decision.State {
	case models.PlatformEnrollmentStateApproved, models.PlatformEnrollmentStateIssuing, models.PlatformEnrollmentStateCompleted:
	default:
		return nil
	}

	frame, err = e.next(ctx, models.OperatorBootstrapFrameComplete)
	if err != nil {
		return err
	}
	if frame.Complete == nil {
		return constants.ErrPlatformEnrollmentInvalidPayload
	}
	completed, err := e.c.enrollSvc.CompleteHeld(ctx, token, *frame.Complete)
	if err != nil {
		return err
	}
	if completed.Operator == nil {
		return constants.ErrPlatformEnrollmentStoredRequestInvalid
	}
	creds := *completed.Operator
	if err := e.c.reg.UpdateOperatorRuntimeConfig(ctx, creds.OperatorID, runtimeConfig); err != nil {
		return fmt.Errorf("operator bootstrap: persist runtime config: %w", err)
	}

	e.c.logger.Info("operator bootstrap: bundle delivered",
		"request_id", completed.RequestID,
		"operator_id", creds.OperatorID,
		"operator_session_id", creds.OperatorSessionID)
	return e.write(models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameBundle, Bundle: &models.OperatorBootstrapBundle{
		Credentials:        creds,
		MaxConcurrentTasks: operatorBootstrapMaxConcurrentTasks,
		MaxMemoryMB:        operatorBootstrapMaxMemoryMB,
	}})
}

// operatorBootstrapRuntimeConfig decodes the declared runtime config and
// rejects a heartbeat interval the Gateway's staleness window cannot honor.
func operatorBootstrapRuntimeConfig(req *models.OperatorBootstrapRequest) (*operatorv1.OperatorRuntimeConfig, error) {
	runtimeConfig, err := models.UnmarshalOperatorRuntimeConfig(req.RuntimeConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime_config: %w", constants.ErrPlatformEnrollmentInvalidPayload, err)
	}
	declared := time.Duration(runtimeConfig.GetHeartbeatIntervalMs()) * time.Millisecond
	if declared > constants.OperatorHeartbeatMaxInterval {
		return nil, fmt.Errorf("%w: %s exceeds %s",
			constants.ErrOperatorHeartbeatIntervalInvalid, declared, constants.OperatorHeartbeatMaxInterval)
	}
	return runtimeConfig, nil
}

// next receives the Operator's next frame and requires it to be want.
func (e operatorBootstrapExchange) next(ctx context.Context, want models.OperatorBootstrapFrameType) (models.OperatorBootstrapFrame, error) {
	select {
	case <-ctx.Done():
		return models.OperatorBootstrapFrame{}, ctx.Err()
	case frame, ok := <-e.frames:
		if !ok {
			return models.OperatorBootstrapFrame{}, context.Canceled
		}
		if frame.Type != want {
			return models.OperatorBootstrapFrame{}, fmt.Errorf("%w: expected %s frame, got %q",
				constants.ErrPlatformEnrollmentInvalidPayload, want, frame.Type)
		}
		return frame, nil
	}
}

func (e operatorBootstrapExchange) write(frame models.OperatorBootstrapFrame) error {
	if err := e.conn.WriteJSON(frame); err != nil {
		return fmt.Errorf("operator bootstrap: write %s frame: %w", frame.Type, err)
	}
	return nil
}

// writeError ends the exchange with a typed error frame. Retryable errors
// are transient: the Operator redials with the same in-memory request.
func (e operatorBootstrapExchange) writeError(err error) {
	frame := models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameError, Error: &models.OperatorBootstrapError{
		Retryable: operatorBootstrapRetryable(err),
		Message:   err.Error(),
	}}
	if writeErr := e.conn.WriteJSON(frame); writeErr != nil {
		e.c.logger.Info("operator bootstrap: error frame not delivered", "error", writeErr)
	}
}

func operatorBootstrapRetryable(err error) bool {
	return sqliteutil.IsBusyError(err) ||
		errors.Is(err, constants.ErrPlatformEnrollmentIssuanceInProgress) ||
		errors.Is(err, constants.ErrPlatformEnrollmentIssuanceFailed)
}

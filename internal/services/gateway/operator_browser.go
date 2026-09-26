// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"net/http"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func browserSessionIDs(r *http.Request) (userID, webSessionID string, ok bool) {
	userID, _ = r.Context().Value(constants.ContextKeyUserID).(string)
	webSessionID, _ = r.Context().Value(constants.ContextKeyWebSessionID).(string)
	userID = strings.TrimSpace(userID)
	webSessionID = strings.TrimSpace(webSessionID)
	return userID, webSessionID, userID != "" && webSessionID != ""
}

// handleOperatorsByID routes operator sub-paths under /api/v1/operators/{id}/.
// Browser: GET detail, POST .../stop. mTLS: POST terminate on /api/v1/operators/{id}.
func (c *OperatorController) handleOperatorsByID(w http.ResponseWriter, r *http.Request) {
	c.handleOperatorBrowserSubpath(w, r)
}

// handleOperatorBrowserSubpath serves browser-facing operator detail and stop routes:
//
//	GET  /api/v1/operators/{operator_id}
//	POST /api/v1/operators/{operator_id}/stop
func (c *OperatorController) handleOperatorBrowserSubpath(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, constants.APIPaths.OperatorsByID)
	remainder = strings.Trim(remainder, "/")
	if remainder == "" {
		c.responder.Error(w, http.StatusNotFound, "not found")
		return
	}
	parts := strings.Split(remainder, "/")
	operatorID := parts[0]
	if operatorID == "" {
		c.responder.Error(w, http.StatusNotFound, "not found")
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		c.handleBrowserGetOperator(w, r, operatorID)
	case len(parts) == 2 && parts[1] == "stop" && r.Method == http.MethodPost:
		c.handleBrowserStopOperator(w, r, operatorID)
	case len(parts) == 1 && r.Method == http.MethodPost:
		if _, _, ok := browserSessionIDs(r); ok && (r.TLS == nil || len(r.TLS.PeerCertificates) == 0) {
			c.responder.Error(w, http.StatusForbidden, constants.ErrForbidden.Error())
			return
		}
		c.handleTerminateOperator(w, r)
	default:
		c.responder.Error(w, http.StatusNotFound, "not found")
	}
}

func (c *OperatorController) handleBrowserGetOperator(w http.ResponseWriter, r *http.Request, operatorID string) {
	userID, _, ok := browserSessionIDs(r)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrProtocolAuthRequired.Error())
		return
	}
	op, err := c.reg.GetOperator(operatorID)
	if err != nil || op == nil {
		c.responder.Error(w, http.StatusNotFound, "operator not found")
		return
	}
	if op.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, constants.ErrRegistrationOperatorNotBelongToUser.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, op)
}

func (c *OperatorController) handleBrowserStopOperator(w http.ResponseWriter, r *http.Request, operatorID string) {
	userID, _, ok := browserSessionIDs(r)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrProtocolAuthRequired.Error())
		return
	}
	op, err := c.reg.GetOperator(operatorID)
	if err != nil || op == nil {
		c.responder.Error(w, http.StatusNotFound, "operator not found")
		return
	}
	if op.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, constants.ErrRegistrationOperatorNotBelongToUser.Error())
		return
	}
	if strings.TrimSpace(op.OperatorSessionID) == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrRegistrationOperatorNoActiveSession.Error())
		return
	}
	if op.Status != constants.OperatorStatusActive {
		c.responder.Error(w, http.StatusConflict, constants.ErrRegistrationOperatorNoActiveSession.Error())
		return
	}
	if op.OperatorType == constants.OperatorTypeEmbedded {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrOperatorStopEmbedded.Error())
		return
	}
	if op.OperatorType != constants.OperatorTypeRemote {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrOperatorStopNotRemote.Error())
		return
	}
	payload, err := proto.Marshal(&operatorv1.ShutdownRequested{})
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrRequestMarshalFailed.Error())
		return
	}
	result, err := c.dispatch.Dispatch(r.Context(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionID,
		EventType:               string(constants.Event.Operator.ShutdownRequested),
		Payload:                 payload,
		TargetResource:          op.ID,
		RequestorUserID:         userID,
	})
	if err != nil {
		c.logger.Warn("gateway: browser stop operator dispatch failed", "operator_id", op.ID, "error", err)
		c.responder.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := c.reg.MarkOperatorStopped(op.ID, userID, ""); err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, browserOperatorStopResponse{
		Message:           "Stop command relayed to orchestrator",
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		Success:           true,
		TransactionID:     result.TransactionID,
	})
}

// browserOperatorStopResponse is the browser stop-operator acknowledgement.
// Field order matches encoding/json map key order.
type browserOperatorStopResponse struct {
	Message           string `json:"message"`
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id"`
	Success           bool   `json:"success"`
	TransactionID     string `json:"transaction_id"`
}

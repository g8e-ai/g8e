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

package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// OperatorController handles Operator lifecycle endpoints.
type OperatorController struct {
	cfg       *config.Config
	logger    *slog.Logger
	reg       *RegistrationService
	auth      *AuthService
	responder *response.Writer
}

func newOperatorController(cfg *config.Config, logger *slog.Logger, reg *RegistrationService, auth *AuthService, responder *response.Writer) *OperatorController {
	return &OperatorController{
		cfg:       cfg,
		logger:    logger,
		reg:       reg,
		auth:      auth,
		responder: responder,
	}
}

func (c *OperatorController) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

// GET /api/v1/operators
func (c *OperatorController) handleListOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}
	slots, err := c.reg.ListOperatorSlots(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.OperatorSlotResponse{Success: true, Operators: slots})
}

// POST /api/v1/operators/{id}/terminate
func (c *OperatorController) handleTerminateOperator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.TerminateOperatorRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.OperatorID == "" {
		c.responder.Error(w, http.StatusBadRequest, "operator_id required")
		return
	}
	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}
	if err := c.reg.TerminateOperator(req.OperatorID, req.UserID, req.Reason); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.TerminateOperatorResponse{Success: true, Message: "Operator terminated"})
}

// POST /api/v1/operators/bind
func (c *OperatorController) handleBindOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.BindOperatorsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := c.reg.BindOperators(req)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, resp)
}

// POST /api/v1/operators/unbind
func (c *OperatorController) handleUnbindOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.UnbindOperatorsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := c.reg.UnbindOperators(req)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, resp)
}

// POST /api/v1/operators/{id}/target
func (c *OperatorController) handleSetTargetContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.SetTargetContextRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := c.reg.SetTargetContext(req)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, resp)
}

// POST /api/v1/operators/reauth
func (c *OperatorController) handleReauth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	operatorSessionID, _ := r.Context().Value(constants.ContextKeyOperatorSessionID).(string)
	if operatorSessionID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "missing operator session id")
		return
	}
	op, err := c.auth.ValidateOperatorSession(operatorSessionID)
	if err != nil {
		c.responder.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Read request body to get RuntimeConfig (operator sends its config)
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req struct {
		RuntimeConfig *models.RuntimeConfig `json:"runtime_config"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	// Build BootstrapConfig with default values
	bootstrapConfig := map[string]interface{}{
		"max_concurrent_tasks":       25,
		"max_memory_mb":              2048,
		"heartbeat_interval_seconds": 30,
		"operator_session_id":        op.OperatorSessionID,
		"operator_id":                op.ID,
		"user_id":                    op.UserID,
	}

	// Return response in format expected by Operator (AuthServicesResponse)
	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		"success":             true,
		"operator_session_id": op.OperatorSessionID,
		"operator_id":         op.ID,
		"user_id":             op.UserID,
		"config":              bootstrapConfig,
	})
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// ObserveProducerControllerDeps groups all dependencies for
// ObserveProducerController.
type ObserveProducerControllerDeps struct {
	Cfg          *config.Config
	Logger       *slog.Logger
	ProducerSvc  *ObserveProducerService
	Responder    *response.Writer
	MaxBodyBytes int64
}

// ObserveProducerController handles the mTLS producer endpoints that the
// g8ee ensemble calls to report agent and run state changes. The gateway
// derives user_id from the mTLS peer certificate (stamped into context by
// handleAppAuth from the delegated user SAN), never from the request body.
// Only app workload identities may call these endpoints; the controller
// rejects operator and CLI certs with 403.
type ObserveProducerController struct {
	cfg          *config.Config
	logger       *slog.Logger
	producerSvc  *ObserveProducerService
	responder    *response.Writer
	maxBodyBytes int64
}

func newObserveProducerController(deps ObserveProducerControllerDeps) *ObserveProducerController {
	maxBody := deps.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 512 * 1024
	}
	return &ObserveProducerController{
		cfg:          deps.Cfg,
		logger:       deps.Logger,
		producerSvc:  deps.ProducerSvc,
		responder:    deps.Responder,
		maxBodyBytes: maxBody,
	}
}

// requireAppUserID extracts the authenticated user ID from the request
// context and verifies the caller is an app workload via the shared
// requireAppIdentity helper. Returns the user ID and true on success. On
// failure the helper writes a 401 (missing user_id) or 403 (not an app
// workload) response and this returns false.
func (c *ObserveProducerController) requireAppUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	_, userID, ok := requireAppIdentity(c.responder, w, r)
	return userID, ok
}

// requireCLIUserID extracts the authenticated user ID from the request
// context and verifies the caller is a CLI session. The unified auth
// middleware stamps ContextKeyUserID and ContextKeyCLISessionID during
// handleCLIAuth. Returns the user ID and true on success. On failure it
// writes a 401 (missing user_id) or 403 (not a CLI session) response and
// returns false.
func (c *ObserveProducerController) requireCLIUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	cliSessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)
	if cliSessionID == "" {
		c.responder.Error(w, http.StatusForbidden, constants.ErrForbidden.Error())
		return "", false
	}
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrNotAuthenticated.Error())
		return "", false
	}
	return userID, true
}

// buildProducerRoute constructs an SSERoute from the derived user_id and
// the request body's session routing fields. Exactly one of web_session_id
// or cli_session_id must be set; the caller rejects dual routing before
// calling this assembler. The route is validated by the producer service;
// this function only assembles it.
func buildProducerRoute(userID, webSessionID, cliSessionID string) SSERoute {
	route := SSERoute{UserID: userID}
	webSessionID = strings.TrimSpace(webSessionID)
	cliSessionID = strings.TrimSpace(cliSessionID)
	if webSessionID != "" {
		route.WebSessionID = webSessionID
	} else {
		route.CLISessionID = cliSessionID
	}
	return route
}

// validateProducerRouting rejects a producer request that provides both
// routing targets or neither. Exactly one of web_session_id or
// cli_session_id is required at the Gateway boundary so the gateway never
// silently picks one when both are supplied.
func validateProducerRouting(webSessionID, cliSessionID string) error {
	webSessionID = strings.TrimSpace(webSessionID)
	cliSessionID = strings.TrimSpace(cliSessionID)
	n := 0
	if webSessionID != "" {
		n++
	}
	if cliSessionID != "" {
		n++
	}
	switch n {
	case 0:
		return constants.ErrGatewaySSERouteSessionRequired
	case 1:
		return nil
	default:
		return constants.ErrGatewaySSERouteSessionMutuallyExclusive
	}
}

// handleAgentState accepts a typed agent state update from the ensemble,
// derives user_id from the mTLS peer certificate, and delegates to the
// observe producer service which persists the projection and emits the
// app.agent.status.updated SSE event (persist-before-publish).
//
// @Summary		Push observe agent state
// @Description	Accepts a typed agent state update from an mTLS-authenticated app workload (the g8ee ensemble). The gateway derives user_id from the peer certificate, persists the agent state projection, and emits an app.agent.status.updated SSE event after successful persistence.
// @Tags			observe
// @Accept			json
// @Produce		json
// @Param			payload	body		models.ObserveProducerAgentStateRequest	true	"Agent state update"
// @Success		200		{string}	string									"accepted"
// @Failure		400		{string}	string									"Invalid transition, stale update, or missing fields"
// @Failure		401		{string}	string									"Unauthorized — mTLS user identity required"
// @Failure		403		{string}	string									"Forbidden — not an app workload or cross-user"
// @Failure		500		{string}	string									"Persistence failure"
// @Router			/api/v1/observe/producer/agent-state [post]
func (c *ObserveProducerController) handleAgentState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireAppUserID(w, r)
	if !ok {
		return
	}
	body, err := readRequestBody(r, c.maxBodyBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req models.ObserveProducerAgentStateRequest
	if err := decodeProducerRequest(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateAgentProducerRequest(req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateProducerRouting(req.WebSessionID, req.CLISessionID); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	route := buildProducerRoute(userID, req.WebSessionID, req.CLISessionID)
	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: req.SchemaVersion,
		AgentID:       req.AgentID,
		DisplayName:   req.DisplayName,
		Role:          req.Role,
		Status:        req.Status,
		RunID:         req.RunID,
		TaskID:        req.TaskID,
		Model:         req.Model,
		ObservedAt:    req.ObservedAt,
	}
	if err := c.producerSvc.UpdateAgentState(r.Context(), userID, route, payload); err != nil {
		c.mapProducerError(w, err, "agent state")
		return
	}
	c.responder.JSON(w, http.StatusOK, models.ObserveProducerResponse{Accepted: true})
}

// handleRunState accepts a typed run state update from the ensemble,
// derives user_id from the mTLS peer certificate, and delegates to the
// observe producer service which persists the projection and emits the
// app.run.status.updated SSE event (persist-before-publish).
//
// @Summary		Push observe run state
// @Description	Accepts a typed run state update from an mTLS-authenticated app workload (the g8ee ensemble). The gateway derives user_id from the peer certificate, persists the run state projection, and emits an app.run.status.updated SSE event after successful persistence.
// @Tags			observe
// @Accept			json
// @Produce		json
// @Param			payload	body		models.ObserveProducerRunStateRequest	true	"Run state update"
// @Success		200		{string}	string									"accepted"
// @Failure		400		{string}	string									"Invalid transition, stale update, or missing fields"
// @Failure		401		{string}	string									"Unauthorized — mTLS user identity required"
// @Failure		403		{string}	string									"Forbidden — not an app workload or cross-user"
// @Failure		500		{string}	string									"Persistence failure"
// @Router			/api/v1/observe/producer/run-state [post]
func (c *ObserveProducerController) handleRunState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireAppUserID(w, r)
	if !ok {
		return
	}
	body, err := readRequestBody(r, c.maxBodyBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req models.ObserveProducerRunStateRequest
	if err := decodeProducerRequest(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRunProducerRequest(req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateProducerRouting(req.WebSessionID, req.CLISessionID); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	route := buildProducerRoute(userID, req.WebSessionID, req.CLISessionID)
	payload := models.RunStatusUpdatedPayload{
		SchemaVersion:  req.SchemaVersion,
		RunID:          req.RunID,
		RunKind:        req.RunKind,
		DisplayName:    req.DisplayName,
		Status:         req.Status,
		ActiveTaskID:   req.ActiveTaskID,
		CompletedTasks: req.CompletedTasks,
		TotalTasks:     req.TotalTasks,
		StartedAt:      req.StartedAt,
		EndedAt:        req.EndedAt,
		ObservedAt:     req.ObservedAt,
	}
	if err := c.producerSvc.UpdateRunState(r.Context(), userID, route, payload); err != nil {
		c.mapProducerError(w, err, "run state")
		return
	}
	c.responder.JSON(w, http.StatusOK, models.ObserveProducerResponse{Accepted: true})
}

// mapProducerError maps a typed observe producer error to the correct HTTP
// status code. Invalid transitions, stale updates, missing required fields,
// and payload validation failures are 400. Cross-user ownership violations
// (agent or run) are 403 with the same non-disclosing response so the HTTP
// boundary does not reveal record existence. Route validation errors
// (missing session) are 400. Persistence and SSE emission failures are 500.
func (c *ObserveProducerController) mapProducerError(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, constants.ErrObserveInvalidTransition),
		errors.Is(err, constants.ErrObserveStaleUpdate),
		errors.Is(err, constants.ErrObserveAgentIDRequired),
		errors.Is(err, constants.ErrObserveRunIDRequired),
		errors.Is(err, constants.ErrObserveObservedAtRequired),
		errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion),
		errors.Is(err, constants.ErrObserveAgentDisplayNameRequired),
		errors.Is(err, constants.ErrObserveAgentRoleRequired),
		errors.Is(err, constants.ErrObserveRunDisplayNameRequired),
		errors.Is(err, constants.ErrObserveRunKindRequired),
		errors.Is(err, constants.ErrObserveNegativeTaskCount),
		errors.Is(err, constants.ErrObserveCompletedExceedsTotal),
		errors.Is(err, constants.ErrObserveEndBeforeStart),
		errors.Is(err, constants.ErrGatewaySSERouteUserIDRequired),
		errors.Is(err, constants.ErrGatewaySSERouteSessionRequired),
		errors.Is(err, constants.ErrGatewaySSERouteSessionMutuallyExclusive):
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, constants.ErrObserveAgentNotFound),
		errors.Is(err, constants.ErrObserveRunNotFound):
		c.responder.Error(w, http.StatusForbidden, constants.ErrForbidden.Error())
	default:
		c.logger.Error("observe producer: "+op+" failed", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
	}
}

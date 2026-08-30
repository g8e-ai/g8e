// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// HealthController handles health, state, and landing page endpoints.
type HealthController struct {
	cfg               *config.Config
	logger            *slog.Logger
	docStore          *DocumentStoreService
	stateRootSvc      *StateRootService
	responder         *response.Writer
	isReady           func() bool
	isGovernanceReady func() bool
}

// HealthControllerDeps groups all dependencies for HealthController.
type HealthControllerDeps struct {
	Cfg               *config.Config
	Logger            *slog.Logger
	DocStore          *DocumentStoreService
	StateRootSvc      *StateRootService
	Responder         *response.Writer
	IsReady           func() bool
	IsGovernanceReady func() bool
}

func newHealthController(d HealthControllerDeps) *HealthController {
	return &HealthController{
		cfg:               d.Cfg,
		logger:            d.Logger,
		docStore:          d.DocStore,
		stateRootSvc:      d.StateRootSvc,
		responder:         d.Responder,
		isReady:           d.IsReady,
		isGovernanceReady: d.IsGovernanceReady,
	}
}

// @Summary		Landing page
// @Description	Returns the public landing page for the gateway
// @Tags			public
// @Accept			html
// @Produce		html
// @Success		200	{string}	string
// @Router			/ [get]
func (c *HealthController) handleLandingPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/console/", http.StatusFound)
}

// @Summary		Health check
// @Description	Returns the current health status of the gateway
// @Tags			health
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.HealthResponse
// @Router			/health [get]
func (c *HealthController) handleHealth(w http.ResponseWriter, r *http.Request) {
	if c.isReady != nil && !c.isReady() {
		c.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	doc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
	if err != nil {
		c.logger.Error("Health check failed to query platform_settings", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}
	if doc == nil {
		c.logger.Warn("Health check: platform_settings not found")
		c.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}

	root, err := c.stateRootSvc.GetCurrentStateRoot()
	if err != nil {
		c.logger.Error("Health check failed to calculate state root", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusServiceUnavailable, "state root calculation failed")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         c.cfg.Version,
		PID:             os.Getpid(),
		GovernanceReady: c.isGovernanceReady != nil && c.isGovernanceReady(),
		Posture:         string(c.cfg.Gateway.Posture),
		StateMerkleRoot: root,
	})
}

// @Summary		Bootstrap health check
// @Description	Returns the current health status during bootstrap (no state root check)
// @Tags			health
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.HealthResponse
// @Router			/health/bootstrap [get]
func (c *HealthController) handleBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if c.isReady != nil && !c.isReady() {
		c.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         c.cfg.Version,
		PID:             os.Getpid(),
		GovernanceReady: c.isGovernanceReady != nil && c.isGovernanceReady(),
		Posture:         string(c.cfg.Gateway.Posture),
	})
}

// @Summary		State root
// @Description	Returns the current state merkle root for envelope binding
// @Tags			state
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.StateResponse
// @Router			/state [get]
func (c *HealthController) handleState(w http.ResponseWriter, r *http.Request) {
	if c.isReady != nil && !c.isReady() {
		c.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	root, err := c.stateRootSvc.GetCurrentStateRoot()
	if err != nil {
		c.logger.Error("State endpoint failed to calculate state root", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusServiceUnavailable, "state root calculation failed")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.StateResponse{
		StateMerkleRoot: root,
	})
}

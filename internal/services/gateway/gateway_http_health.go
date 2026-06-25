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
	"net/http"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// @Summary		Landing page
// @Description	Returns the public landing page for the gateway
// @Tags			public
// @Accept			html
// @Produce		html
// @Success		200	{string}	string
// @Router			/ [get]
func (h *HTTPHandler) handleLandingPage(w http.ResponseWriter, r *http.Request) {
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
func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	doc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
	if err != nil {
		h.logger.Error("Health check failed to query platform_settings", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}
	if doc == nil {
		h.logger.Warn("Health check: platform_settings not found")
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}

	root, err := h.db.StateRootSvc.GetCurrentStateRoot()
	if err != nil {
		h.logger.Error("Health check failed to calculate state root", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "state root calculation failed")
		return
	}

	h.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
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
func (h *HTTPHandler) handleBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	h.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
	})
}

// @Summary		State root
// @Description	Returns the current state merkle root for envelope binding
// @Tags			state
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.StateResponse
// @Router			/state [get]
func (h *HTTPHandler) handleState(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	root, err := h.db.StateRootSvc.GetCurrentStateRoot()
	if err != nil {
		h.logger.Error("State endpoint failed to calculate state root", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "state root calculation failed")
		return
	}

	h.responder.JSON(w, http.StatusOK, models.StateResponse{
		StateMerkleRoot: root,
	})
}

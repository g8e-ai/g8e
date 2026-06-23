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
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// AdminController handles admin-only endpoints for app policy management.
type AdminController struct {
	cfg       *config.Config
	logger    *slog.Logger
	db        *CanonicalDBService
	userSvc   *UserService
	responder *response.Writer
}

func newAdminController(cfg *config.Config, logger *slog.Logger, db *CanonicalDBService, userSvc *UserService, responder *response.Writer) *AdminController {
	return &AdminController{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		userSvc:   userSvc,
		responder: responder,
	}
}

func (c *AdminController) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

// POST /api/v1/admin/app-policies/{id}/signers
func (c *AdminController) handleAppPolicySigner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	appID := strings.TrimPrefix(r.URL.Path, constants.APIPaths.AdminAppPoliciesPrefix)
	appID = strings.TrimSuffix(appID, "/signers")
	if appID == "" {
		c.responder.Error(w, http.StatusBadRequest, "app_id required")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsBootstrap {
		c.responder.Error(w, http.StatusForbidden, "admin-only: bootstrap user required")
		return
	}

	policyDoc, err := c.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to check app policy")
		return
	}
	if policyDoc == nil {
		c.responder.Error(w, http.StatusForbidden, "app policy not found (deny-all default)")
		return
	}

	body, err := c.readBody(w, r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		PublicKey string `json:"public_key_hex"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.PublicKey == "" {
		c.responder.Error(w, http.StatusBadRequest, "public_key_hex required")
		return
	}

	pubKeyBytes, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid hex public key")
		return
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		c.responder.Error(w, http.StatusBadRequest, "invalid public key size")
		return
	}

	signer := models.TrustedSigner{
		ID:        appID,
		PublicKey: req.PublicKey,
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}

	if err := c.db.SignerStore.AddTrustedSigner(signer); err != nil {
		c.logger.Error("failed to add trusted signer", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to add trusted signer")
		return
	}

	c.logger.Info("External app L2 signer registered by admin", "app_id", appID)
	c.responder.JSON(w, http.StatusCreated, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

// POST /api/v1/admin/apps/revoke
func (c *AdminController) handleRevokeApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsBootstrap {
		c.responder.Error(w, http.StatusForbidden, "admin-only: bootstrap user required")
		return
	}

	body, err := c.readBody(w, r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		AppID string `json:"app_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.AppID == "" {
		c.responder.Error(w, http.StatusBadRequest, "app_id required")
		return
	}

	_, err = c.db.DocStore.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), req.AppID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete app policy")
		return
	}

	_, err = c.db.DocStore.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), req.AppID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete trusted signer")
		return
	}

	c.logger.Info("External app revoked by admin", "app_id", req.AppID)
	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

// POST /api/v1/admin/tribunals
// GET /api/v1/admin/tribunals
func (c *AdminController) handleTribunals(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsBootstrap {
		c.responder.Error(w, http.StatusForbidden, "admin-only: bootstrap user required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		body, err := c.readBody(w, r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "failed to read body")
			return
		}

		var policy models.TribunalPolicy
		if err := json.Unmarshal(body, &policy); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if err := c.db.TribunalStore.AddTribunal(policy); err != nil {
			c.logger.Error("failed to add tribunal", "error", err)
			c.responder.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		c.logger.Info("Tribunal created by admin", "tribunal_id", policy.ID)
		c.responder.JSON(w, http.StatusCreated, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodGet:
		tribunals, err := c.db.TribunalStore.ListTribunals()
		if err != nil {
			c.logger.Error("failed to list tribunals", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to list tribunals")
			return
		}

		c.responder.JSON(w, http.StatusOK, tribunals)

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// DELETE /api/v1/admin/tribunals/{id}
func (c *AdminController) handleDeleteTribunal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsBootstrap {
		c.responder.Error(w, http.StatusForbidden, "admin-only: bootstrap user required")
		return
	}

	if r.Method != http.MethodDelete {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tribunalID := strings.TrimPrefix(r.URL.Path, constants.APIPaths.AdminTribunalsPrefix)
	if tribunalID == "" {
		c.responder.Error(w, http.StatusBadRequest, "tribunal_id required")
		return
	}

	deleted, err := c.db.TribunalStore.DeleteTribunal(tribunalID)
	if err != nil {
		c.logger.Error("failed to delete tribunal", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete tribunal")
		return
	}
	if !deleted {
		c.responder.Error(w, http.StatusNotFound, "tribunal not found")
		return
	}

	c.logger.Info("Tribunal deleted by admin", "tribunal_id", tribunalID)
	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

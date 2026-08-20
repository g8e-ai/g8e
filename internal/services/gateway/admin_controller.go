// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	cfg            *config.Config
	logger         *slog.Logger
	docStore       *DocumentStoreService
	signerStore    *SignerStoreService
	consensusStore *ConsensusStoreService
	userSvc        *UserService
	responder      *response.Writer
}

// AdminControllerDeps groups all dependencies for AdminController.
type AdminControllerDeps struct {
	Cfg            *config.Config
	Logger         *slog.Logger
	DocStore       *DocumentStoreService
	SignerStore    *SignerStoreService
	ConsensusStore *ConsensusStoreService
	UserSvc        *UserService
	Responder      *response.Writer
}

func newAdminController(d AdminControllerDeps) *AdminController {
	return &AdminController{
		cfg:            d.Cfg,
		logger:         d.Logger,
		docStore:       d.DocStore,
		signerStore:    d.SignerStore,
		consensusStore: d.ConsensusStore,
		userSvc:        d.UserSvc,
		responder:      d.Responder,
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
	isFirst, err := c.userSvc.IsFirstUser(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if !isFirst {
		c.responder.Error(w, http.StatusForbidden, "admin-only: first user required")
		return
	}

	policyDoc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
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

	if err := c.signerStore.AddTrustedSigner(signer); err != nil {
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
	isFirst, err := c.userSvc.IsFirstUser(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if !isFirst {
		c.responder.Error(w, http.StatusForbidden, "admin-only: first user required")
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

	_, err = c.docStore.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), req.AppID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete app policy")
		return
	}

	_, err = c.docStore.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), req.AppID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete trusted signer")
		return
	}

	c.logger.Info("External app revoked by admin", "app_id", req.AppID)
	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

// POST /api/v1/admin/consensus
// GET /api/v1/admin/consensus
func (c *AdminController) handleConsensus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	isFirst, err := c.userSvc.IsFirstUser(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if !isFirst {
		c.responder.Error(w, http.StatusForbidden, "admin-only: first user required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		body, err := c.readBody(w, r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "failed to read body")
			return
		}

		var policy models.ConsensusPolicy
		if err := json.Unmarshal(body, &policy); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if err := c.consensusStore.AddConsensus(policy); err != nil {
			c.logger.Error("failed to add consensus", "error", err)
			c.responder.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		c.logger.Info("Consensus created by admin", "consensus_id", policy.ID)
		c.responder.JSON(w, http.StatusCreated, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodGet:
		consensus, err := c.consensusStore.ListConsensus()
		if err != nil {
			c.logger.Error("failed to list consensus", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to list consensus")
			return
		}

		c.responder.JSON(w, http.StatusOK, consensus)

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// DELETE /api/v1/admin/consensus/{id}
func (c *AdminController) handleDeleteConsensus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	isFirst, err := c.userSvc.IsFirstUser(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if !isFirst {
		c.responder.Error(w, http.StatusForbidden, "admin-only: first user required")
		return
	}

	if r.Method != http.MethodDelete {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	consensusID := strings.TrimPrefix(r.URL.Path, constants.APIPaths.AdminConsensusPrefix)
	if consensusID == "" {
		c.responder.Error(w, http.StatusBadRequest, "consensus_id required")
		return
	}

	deleted, err := c.consensusStore.DeleteConsensus(consensusID)
	if err != nil {
		c.logger.Error("failed to delete consensus", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to delete consensus")
		return
	}
	if !deleted {
		c.responder.Error(w, http.StatusNotFound, "consensus not found")
		return
	}

	c.logger.Info("Consensus deleted by admin", "consensus_id", consensusID)
	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

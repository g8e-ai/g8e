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
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// SignerController handles governance trusted signer endpoints.
type SignerController struct {
	cfg         *config.Config
	logger      *slog.Logger
	docStore    *DocumentStoreService
	signerStore *SignerStoreService
	responder   *response.Writer
}

// SignerControllerDeps groups all dependencies for SignerController.
type SignerControllerDeps struct {
	Cfg         *config.Config
	Logger      *slog.Logger
	DocStore    *DocumentStoreService
	SignerStore *SignerStoreService
	Responder   *response.Writer
}

func newSignerController(d SignerControllerDeps) *SignerController {
	return &SignerController{
		cfg:         d.Cfg,
		logger:      d.Logger,
		docStore:    d.DocStore,
		signerStore: d.SignerStore,
		responder:   d.Responder,
	}
}

func (c *SignerController) readBody(r *http.Request) ([]byte, error) {
	return readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
}

func (c *SignerController) handleGovernanceSigners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		signers, err := c.signerStore.ListTrustedSigners()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("signer_controller: handleGovernanceSigners: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.TrustedSignersResponse{
			Success: true,
			Signers: signers,
		})

	case http.MethodPost:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrSignerControllerBodyReadFailed.Error())
			return
		}
		var signer models.TrustedSigner
		if err := json.Unmarshal(body, &signer); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if signer.ID == "" || signer.PublicKey == "" {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
			return
		}
		if err := c.signerStore.AddTrustedSigner(signer); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("signer_controller: handleGovernanceSigners: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusCreated, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

func (c *SignerController) handleGovernanceSignerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, constants.APIPaths.GovernanceSignersPrefix)
	if id == "" || strings.Contains(id, "/") {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrSignerControllerInvalidSignerID.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		pubKey, err := c.signerStore.GetTrustedSigner(id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("signer_controller: handleGovernanceSignerByID: %w", err).Error())
			return
		}
		if pubKey == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		doc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("signer_controller: handleGovernanceSignerByID: %w", err).Error())
			return
		}
		if doc == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		deleted, err := c.signerStore.DeleteTrustedSigner(id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("signer_controller: handleGovernanceSignerByID: %w", err).Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

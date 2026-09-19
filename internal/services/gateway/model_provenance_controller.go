// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func modelProvenanceControllerDeps(logger *slog.Logger, responder *response.Writer, fileSvc fs.RuntimeFileService) ModelProvenanceControllerDeps {
	deps := ModelProvenanceControllerDeps{
		Logger:    logger,
		Responder: responder,
	}
	if fileSvc == nil {
		return deps
	}
	if windows, err := model_provenance.NewWindowStore(fileSvc); err == nil {
		deps.Windows = windows
	}
	return deps
}

// ModelProvenanceController serves owner mTLS reads of model provenance
// attestation evidence stored in the gateway runtime volume.
type ModelProvenanceController struct {
	logger    *slog.Logger
	responder *response.Writer
	windows   model_provenance.WindowStore
}

// ModelProvenanceControllerDeps groups dependencies for ModelProvenanceController.
type ModelProvenanceControllerDeps struct {
	Logger    *slog.Logger
	Responder *response.Writer
	Windows   model_provenance.WindowStore
}

func newModelProvenanceController(d ModelProvenanceControllerDeps) *ModelProvenanceController {
	return &ModelProvenanceController{
		logger:    d.Logger,
		responder: d.Responder,
		windows:   d.Windows,
	}
}

// @Summary		Get model provenance attestation evidence
// @Description	Returns the canonical model provenance attestation window for one provider attempt ID (mTLS owner CLI only).
// @Tags			inference
// @Produce		json
// @Param			provider_attempt_id	path	string	true	"Provider attempt ID"
// @Success		200	{object}	models.ModelProvenanceResponse
// @Failure		404	{string}	string	"Model provenance evidence not found"
// @Failure		405	{string}	string	"Method Not Allowed"
// @Failure		503	{string}	string	"Model provenance store unavailable"
// @Router			/api/v1/inference/model-provenance/attestations/{provider_attempt_id} [get]
func (c *ModelProvenanceController) handleModelProvenance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	if c.windows == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
		return
	}

	providerAttemptID := strings.TrimPrefix(r.URL.Path, constants.APIPaths.InferenceModelProvenanceAttestations)
	if providerAttemptID == "" || strings.Contains(providerAttemptID, "/") {
		c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
		return
	}

	window, err := c.windows.Load(r.Context(), providerAttemptID)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("model provenance: load window: %w", err).Error())
		return
	}

	body, err := evalv1.MarshalCanonical(window)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("model provenance: marshal window: %w", err).Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.ModelProvenanceResponse{Window: body})
}

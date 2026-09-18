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
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func providerObservationControllerDeps(logger *slog.Logger, responder *response.Writer, fileSvc fs.RuntimeFileService) ProviderObservationControllerDeps {
	deps := ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: responder,
	}
	if fileSvc == nil {
		return deps
	}
	if windows, err := provider_observer.NewWindowStore(fileSvc); err == nil {
		deps.Windows = windows
	}
	if attempts, err := inference.NewAttemptStore(fileSvc); err == nil {
		deps.Attempts = attempts
	}
	return deps
}

// ProviderObservationController serves owner mTLS reads of provider-boundary
// observation evidence stored in the gateway runtime volume.
type ProviderObservationController struct {
	logger       *slog.Logger
	responder    *response.Writer
	windows      provider_observer.WindowStore
	attempts     inference.AttemptStore
	preflight    providerObservationPreflight
}

type providerObservationPreflight interface {
	PreflightCommandDelivery(ctx context.Context) error
}

// ProviderObservationControllerDeps groups dependencies for ProviderObservationController.
type ProviderObservationControllerDeps struct {
	Logger                 *slog.Logger
	Responder              *response.Writer
	Windows                provider_observer.WindowStore
	Attempts               inference.AttemptStore
	ObservationCoordinator providerObservationPreflight
}

func newProviderObservationController(d ProviderObservationControllerDeps) *ProviderObservationController {
	return &ProviderObservationController{
		logger:    d.Logger,
		responder: d.Responder,
		windows:   d.Windows,
		attempts:  d.Attempts,
		preflight: d.ObservationCoordinator,
	}
}

// @Summary		Get provider-boundary observation evidence
// @Description	Returns the canonical provider-boundary observation window and durable provider-attempt record for one provider attempt ID (mTLS owner CLI only).
// @Tags			inference
// @Produce		json
// @Param			provider_attempt_id	path	string	true	"Provider attempt ID"
// @Success		200	{object}	models.ProviderObservationResponse
// @Failure		404	{string}	string	"Observation evidence not found"
// @Failure		405	{string}	string	"Method Not Allowed"
// @Failure		503	{string}	string	"Provider observation store unavailable"
// @Router			/api/v1/inference/provider-observations/{provider_attempt_id} [get]
func (c *ProviderObservationController) handleProviderObservation(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, constants.APIPaths.InferenceProviderObservations)
	if suffix == "_preflight" {
		c.handleProviderObservationPreflight(w, r)
		return
	}
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	if c.windows == nil || c.attempts == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
		return
	}

	providerAttemptID := suffix
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
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("provider observation: load window: %w", err).Error())
		return
	}

	attempt, err := c.attempts.Get(r.Context(), providerAttemptID)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			attempt = provider_observer.AttemptRecordFromObservationWindow(window)
			if attempt == nil {
				c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
				return
			}
		} else {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("provider observation: load attempt: %w", err).Error())
			return
		}
	}

	windowBytes, err := evalv1.MarshalCanonical(window)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("provider observation: marshal window: %w", err).Error())
		return
	}
	attemptBytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(attempt)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("provider observation: marshal attempt: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.ProviderObservationResponse{
		Window:          windowBytes,
		ProviderAttempt: attemptBytes,
	})
}

func (c *ProviderObservationController) handleProviderObservationPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	if c.preflight == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
		return
	}
	if err := c.preflight.PreflightCommandDelivery(r.Context()); err != nil {
		if errors.Is(err, constants.ErrEvaluationObservationUnavailable) ||
			errors.Is(err, constants.ErrProviderBoundaryObserverNotFound) ||
			errors.Is(err, constants.ErrProviderBoundaryObserverAmbiguous) {
			c.responder.Error(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

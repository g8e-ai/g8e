// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// EvalCampaignPublicationController exposes gateway-owned campaign publication
// idempotency state to the owner CLI over mTLS.
type EvalCampaignPublicationController struct {
	logger    *slog.Logger
	responder *response.Writer
	service   *EvalCampaignPublicationService
	mirror    func() *PublicMirrorServer
}

type EvalCampaignPublicationControllerDeps struct {
	Logger    *slog.Logger
	Responder *response.Writer
	Service   *EvalCampaignPublicationService
	Mirror    func() *PublicMirrorServer
}

func newEvalCampaignPublicationController(d EvalCampaignPublicationControllerDeps) *EvalCampaignPublicationController {
	return &EvalCampaignPublicationController{
		logger:    d.Logger,
		responder: d.Responder,
		service:   d.Service,
		mirror:    d.Mirror,
	}
}

const evalCampaignPublicationStateSuffix = "/publication-state"

func (c *EvalCampaignPublicationController) handlePublicationState(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, evalCampaignPublicationStateSuffix) {
		c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
		return
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, constants.APIPaths.EvalCampaignPublicationStateByRun), evalCampaignPublicationStateSuffix)
	if runID == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		state, err := c.service.Get(runID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, state)
	case http.MethodDelete:
		if err := c.service.Delete(runID); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if c.mirror != nil {
			mirror := c.mirror()
			if mirror != nil {
				if err := mirror.WithdrawCampaignDataset(r.Context(), runID); err != nil {
					c.responder.Error(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		}
		c.responder.JSON(w, http.StatusOK, map[string]string{"run_id": runID, "status": "discarded"})
	case http.MethodPut:
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, constants.PublicFeedBatchMaxBytes))
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
			return
		}
		state := models.EvalCampaignPublicationState{}
		if err := json.Unmarshal(body, &state); err != nil {
			c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
			return
		}
		if state.RunID == "" {
			state.RunID = runID
		}
		if state.RunID != runID {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrEvidenceScopeMismatch.Error())
			return
		}
		if err := c.service.Put(state); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, state)
	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// PublicFeedController handles owner mTLS publication of signed public-feed batches.
type PublicFeedController struct {
	logger    *slog.Logger
	responder *response.Writer
	spectator func() *PublicSpectatorRuntime
}

// PublicFeedControllerDeps groups dependencies for PublicFeedController.
type PublicFeedControllerDeps struct {
	Logger    *slog.Logger
	Responder *response.Writer
	Spectator func() *PublicSpectatorRuntime
}

func newPublicFeedController(d PublicFeedControllerDeps) *PublicFeedController {
	return &PublicFeedController{
		logger:    d.Logger,
		responder: d.Responder,
		spectator: d.Spectator,
	}
}

// @Summary		Export public feed batch
// @Description	Accepts campaign projection records and signs/publishes them through the gateway-owned public spectator stack (mTLS owner CLI only).
// @Tags			public-feed
// @Accept			json
// @Produce		json
// @Param			records	body	[]models.PublicFeedRecord	true	"Ordered public feed records"
// @Success		200	{object}	models.PublicFeedExportBatchResponse
// @Failure		400	{string}	string	"Bad Request"
// @Failure		405	{string}	string	"Method Not Allowed"
// @Failure		503	{string}	string	"Public spectator unavailable"
// @Router			/api/v1/public-feed/batches [post]
//
// @Summary		Get public feed snapshot
// @Description	Returns the gateway publisher high-water sequence and feed-chain hash (mTLS owner CLI only).
// @Tags			public-feed
// @Produce		json
// @Success		200	{object}	models.PublicFeedSnapshot
// @Failure		404	{string}	string	"Snapshot not found"
// @Failure		405	{string}	string	"Method Not Allowed"
// @Failure		503	{string}	string	"Public spectator unavailable"
// @Router			/api/v1/public-feed/snapshot [get]
func (c *PublicFeedController) handlePublicFeedSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	publisher, err := c.publisher()
	if err != nil {
		c.responder.Error(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	snapshot, err := publisher.GetSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, constants.ErrPublicFeedSnapshotNotFound) {
			c.responder.Error(w, http.StatusNotFound, err.Error())
			return
		}
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("public-feed: snapshot: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, snapshot)
}

func (c *PublicFeedController) handlePublicFeedBatches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	publisher, err := c.publisher()
	if err != nil {
		c.responder.Error(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, constants.PublicFeedBatchMaxBytes))
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	var records []models.PublicFeedRecord
	if err := json.Unmarshal(body, &records); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	if err := publisher.ExportBatch(r.Context(), records); err != nil {
		status, message := publicFeedExportBatchErrorStatus(err)
		c.responder.Error(w, status, message)
		return
	}

	snapshot, err := publisher.GetSnapshot(r.Context())
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("public-feed: snapshot: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PublicFeedExportBatchResponse{
		HighWaterSequence: snapshot.HighWaterSequence,
		FeedChainHash:     snapshot.FeedChainHash,
	})
}

func (c *PublicFeedController) publisher() (*PublicPublisherService, error) {
	if c.spectator == nil {
		return nil, constants.ErrPublicFeedDisabled
	}
	runtime := c.spectator()
	if runtime == nil || !runtime.Running() {
		return nil, constants.ErrPublicFeedDisabled
	}
	publisher := runtime.Publisher()
	if publisher == nil {
		return nil, constants.ErrPublicFeedDisabled
	}
	return publisher, nil
}

func publicFeedExportBatchErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, constants.ErrPublicFeedDisabled):
		return http.StatusServiceUnavailable, err.Error()
	case errors.Is(err, constants.ErrPublicFeedBatchEmpty),
		errors.Is(err, constants.ErrPublicFeedBatchOversized),
		errors.Is(err, constants.ErrPublicFeedSequenceOutOfOrder),
		errors.Is(err, constants.ErrPublicFeedRecordHashMismatch),
		errors.Is(err, constants.ErrPublicFeedRecordTypeInvalid),
		errors.Is(err, constants.ErrPublicFeedRecordSchemaInvalid),
		errors.Is(err, constants.ErrPublicFeedRestrictedField):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

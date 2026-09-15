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
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// DownloadStreamer streams download artifact bytes for the observe read API.
// The producer service implements this interface; the observe controller
// depends on the narrow interface rather than the full producer service so
// the read-only browser surface stays decoupled from the mTLS mutation
// surface. When nil (e.g. unit tests that do not exercise streaming),
// handleGetDownload returns metadata only.
type DownloadStreamer interface {
	StreamDownload(ctx context.Context, userID, artifactID string, w http.ResponseWriter) error
}

// ObserveControllerDeps groups all dependencies for ObserveController.
type ObserveControllerDeps struct {
	Cfg              *config.Config
	Logger           *slog.Logger
	ObserveSvc       *ObserveService
	DownloadStreamer DownloadStreamer
	Responder        *response.Writer
}

// ObserveController handles the passkey-scoped, read-only observability API.
// Every handler derives user_id from the auth middleware context (stamped by
// the web session cookie validation) and applies ownership scoping through
// the ObserveService. No caller-supplied identity is trusted.
type ObserveController struct {
	cfg              *config.Config
	logger           *slog.Logger
	observeSvc       *ObserveService
	downloadStreamer DownloadStreamer
	responder        *response.Writer
}

func newObserveController(deps ObserveControllerDeps) *ObserveController {
	return &ObserveController{
		cfg:              deps.Cfg,
		logger:           deps.Logger,
		observeSvc:       deps.ObserveSvc,
		downloadStreamer: deps.DownloadStreamer,
		responder:        deps.Responder,
	}
}

// requireUserID extracts the authenticated user ID from the request context.
// Returns the user ID and true if present, or empty string and false if the
// request is unauthenticated. On failure it writes a 401 response and returns
// false.
func (c *ObserveController) requireUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrNotAuthenticated.Error())
		return "", false
	}
	return userID, true
}

// parseLimit extracts and validates the limit query parameter. A missing or
// empty limit defaults to ObserveDefaultLimit. An invalid limit returns an
// error and writes a 400 response.
func (c *ObserveController) parseLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return ObserveDefaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < ObserveMinLimit || limit > ObserveMaxLimit {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrObserveLimitOutOfBounds.Error())
		return 0, false
	}
	return limit, true
}

// parseCursor extracts the cursor query parameter. An empty cursor is valid
// (start of list). An invalid cursor returns an error and writes a 400
// response.
func (c *ObserveController) parseCursor(w http.ResponseWriter, r *http.Request) (string, bool) {
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		return "", true
	}
	if _, err := decodeCursor(cursor); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrObserveCursorInvalid.Error())
		return "", false
	}
	return cursor, true
}

// extractID extracts the path parameter after the given prefix. For example,
// extractID("/api/v1/observe/runs/", "/api/v1/observe/runs/run-123") returns
// "run-123".
func extractID(prefix, path string) string {
	return strings.TrimPrefix(path, prefix)
}

// handleBootstrap returns the bounded initial snapshot for first paint.
//
// @Summary		Get observe bootstrap snapshot
// @Description	Returns one bounded initial snapshot for agents, active run, overview counters, recent runs, latest eval summaries, and download metadata.
// @Tags			observe
// @Produce		json
// @Success		200	{object}	models.ObserveBootstrapSnapshot
// @Failure		401	{string}	string	"Unauthorized"
// @Router			/api/v1/observe/bootstrap [get]
func (c *ObserveController) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	snapshot, err := c.observeSvc.GetBootstrapSnapshot(r.Context(), userID)
	if err != nil {
		c.logger.Error("observe: bootstrap failed", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, snapshot)
}

// handleListRuns returns paginated read-only run summaries owned by the
// authenticated user.
//
// @Summary		List observe runs
// @Description	Returns paginated read-only run summaries owned by the authenticated user.
// @Tags			observe
// @Produce		json
// @Param			cursor	query		string	false	"Pagination cursor"
// @Param			limit	query		int		false	"Page size (1-100, default 20)"
// @Success		200		{object}	models.ObservePage
// @Failure		401		{string}	string	"Unauthorized"
// @Failure		400		{string}	string	"Invalid cursor or limit"
// @Router			/api/v1/observe/runs [get]
func (c *ObserveController) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	cursor, ok := c.parseCursor(w, r)
	if !ok {
		return
	}
	limit, ok := c.parseLimit(w, r)
	if !ok {
		return
	}
	page, err := c.observeSvc.ListRuns(r.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, constants.ErrObserveCursorInvalid) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrObserveCursorInvalid.Error())
			return
		}
		c.logger.Error("observe: list runs failed", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, page)
}

// handleGetRun returns a typed run detail projection with tasks and
// evidence-safe links.
//
// @Summary		Get observe run detail
// @Description	Returns a typed run detail projection with tasks and evidence-safe links.
// @Tags			observe
// @Produce		json
// @Param			run_id	path		string	true	"Run ID"
// @Success		200		{object}	models.RunDetail
// @Failure		401		{string}	string	"Unauthorized"
// @Failure		404		{string}	string	"Run not found"
// @Router			/api/v1/observe/runs/{run_id} [get]
func (c *ObserveController) handleGetRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	runID := extractID(constants.APIPaths.ObserveRunsByID, r.URL.Path)
	if runID == "" || runID == constants.APIPaths.ObserveRunsByID {
		c.responder.Error(w, http.StatusNotFound, constants.ErrObserveRunNotFound.Error())
		return
	}
	detail, err := c.observeSvc.GetRun(r.Context(), userID, runID)
	if err != nil {
		if errors.Is(err, constants.ErrObserveRunNotFound) {
			c.responder.Error(w, http.StatusNotFound, constants.ErrObserveRunNotFound.Error())
			return
		}
		c.logger.Error("observe: get run failed", "error", err, "user_id", userID, "run_id", runID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, detail)
}

// handleListEvals returns paginated eval run projections owned by the
// authenticated user.
//
// @Summary		List observe evals
// @Description	Returns paginated eval run projections owned by the authenticated user.
// @Tags			observe
// @Produce		json
// @Param			cursor	query		string	false	"Pagination cursor"
// @Param			limit	query		int		false	"Page size (1-100, default 20)"
// @Success		200		{object}	models.ObservePage
// @Failure		401		{string}	string	"Unauthorized"
// @Failure		400		{string}	string	"Invalid cursor or limit"
// @Router			/api/v1/observe/evals [get]
func (c *ObserveController) handleListEvals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	cursor, ok := c.parseCursor(w, r)
	if !ok {
		return
	}
	limit, ok := c.parseLimit(w, r)
	if !ok {
		return
	}
	page, err := c.observeSvc.ListEvals(r.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, constants.ErrObserveCursorInvalid) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrObserveCursorInvalid.Error())
			return
		}
		c.logger.Error("observe: list evals failed", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, page)
}

// handleGetEval returns a typed eval detail projection with metrics, status,
// and verification boundary.
//
// @Summary		Get observe eval detail
// @Description	Returns a typed eval detail projection with manifest-safe identity, arm/model information, aggregate metrics, status, and verification boundary.
// @Tags			observe
// @Produce		json
// @Param			run_id	path		string	true	"Eval run ID"
// @Success		200		{object}	models.EvalDetail
// @Failure		401		{string}	string	"Unauthorized"
// @Failure		404		{string}	string	"Eval not found"
// @Router			/api/v1/observe/evals/{run_id} [get]
func (c *ObserveController) handleGetEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	runID := extractID(constants.APIPaths.ObserveEvalsByID, r.URL.Path)
	if runID == "" || runID == constants.APIPaths.ObserveEvalsByID {
		c.responder.Error(w, http.StatusNotFound, constants.ErrObserveEvalNotFound.Error())
		return
	}
	detail, err := c.observeSvc.GetEval(r.Context(), userID, runID)
	if err != nil {
		if errors.Is(err, constants.ErrObserveEvalNotFound) {
			c.responder.Error(w, http.StatusNotFound, constants.ErrObserveEvalNotFound.Error())
			return
		}
		c.logger.Error("observe: get eval failed", "error", err, "user_id", userID, "run_id", runID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, detail)
}

// handleListDownloads returns a paginated list of allowlisted download
// artifacts owned by the authenticated user.
//
// @Summary		List observe downloads
// @Description	Returns a paginated list of allowlisted download artifacts owned by the authenticated user.
// @Tags			observe
// @Produce		json
// @Param			cursor	query		string	false	"Pagination cursor"
// @Param			limit	query		int		false	"Page size (1-100, default 20)"
// @Success		200		{object}	models.ObservePage
// @Failure		401		{string}	string	"Unauthorized"
// @Failure		400		{string}	string	"Invalid cursor or limit"
// @Router			/api/v1/observe/downloads [get]
func (c *ObserveController) handleListDownloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	cursor, ok := c.parseCursor(w, r)
	if !ok {
		return
	}
	limit, ok := c.parseLimit(w, r)
	if !ok {
		return
	}
	page, err := c.observeSvc.ListDownloads(r.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, constants.ErrObserveCursorInvalid) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrObserveCursorInvalid.Error())
			return
		}
		c.logger.Error("observe: list downloads failed", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, page)
}

// handleGetDownload returns a single allowlisted download artifact owned by
// the authenticated user. When the ?download=1 query parameter is present
// and a DownloadStreamer is wired, the handler streams the artifact bytes
// with verified ownership, on-disk hash and size checks, and the cataloged
// Content-Type, Content-Length, and Content-Disposition headers. Without the
// query parameter (or when no streamer is wired), it returns the artifact
// metadata as JSON.
//
// @Summary		Get observe download artifact
// @Description	Returns a single allowlisted download artifact owned by the authenticated user. Use ?download=1 to stream the artifact bytes.
// @Tags			observe
// @Produce		json
// @Param			artifact_id	path		string	true	"Artifact ID"
// @Param			download	query		int		false	"Set to 1 to stream the artifact bytes instead of returning metadata"
// @Success		200			{object}	models.DownloadArtifact
// @Failure		401			{string}	string	"Unauthorized"
// @Failure		404			{string}	string	"Download not found"
// @Router			/api/v1/observe/downloads/{artifact_id} [get]
func (c *ObserveController) handleGetDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	userID, ok := c.requireUserID(w, r)
	if !ok {
		return
	}
	artifactID := extractID(constants.APIPaths.ObserveDownloadsByID, r.URL.Path)
	if artifactID == "" || artifactID == constants.APIPaths.ObserveDownloadsByID {
		c.responder.Error(w, http.StatusNotFound, constants.ErrObserveDownloadNotFound.Error())
		return
	}
	if r.URL.Query().Get("download") == "1" && c.downloadStreamer != nil {
		if err := c.downloadStreamer.StreamDownload(r.Context(), userID, artifactID, w); err != nil {
			c.mapDownloadStreamError(w, err, userID, artifactID)
		}
		return
	}
	artifact, err := c.observeSvc.GetDownload(r.Context(), userID, artifactID)
	if err != nil {
		if errors.Is(err, constants.ErrObserveDownloadNotFound) {
			c.responder.Error(w, http.StatusNotFound, constants.ErrObserveDownloadNotFound.Error())
			return
		}
		c.logger.Error("observe: get download failed", "error", err, "user_id", userID, "artifact_id", artifactID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, artifact)
}

// mapDownloadStreamError maps a download streaming error to the correct HTTP
// status code. Not-found and cross-user ownership violations are 404 with the
// same non-disclosing response so the HTTP boundary does not reveal record
// existence. Restricted artifacts, symlinks, oversized files, and hash/size
// mismatches are 404 (the artifact is not safely downloadable). Other errors
// are 500.
func (c *ObserveController) mapDownloadStreamError(w http.ResponseWriter, err error, userID, artifactID string) {
	switch {
	case errors.Is(err, constants.ErrObserveDownloadNotFound),
		errors.Is(err, constants.ErrObserveDownloadRestrictedArtifact),
		errors.Is(err, constants.ErrObserveDownloadSymlinkRejected),
		errors.Is(err, constants.ErrObserveDownloadOversized),
		errors.Is(err, constants.ErrObserveDownloadSizeMismatch),
		errors.Is(err, constants.ErrObserveDownloadHashMismatch):
		c.responder.Error(w, http.StatusNotFound, constants.ErrObserveDownloadNotFound.Error())
	default:
		c.logger.Error("observe: stream download failed", "error", err, "user_id", userID, "artifact_id", artifactID)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
	}
}

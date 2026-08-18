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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// SessionControllerDeps groups all dependencies for SessionController.
type SessionControllerDeps struct {
	Logger      *slog.Logger
	DocStore    *DocumentStoreService
	Responder   *response.Writer
	CrossOrigin bool
}

// SessionController handles web session lifecycle (logout, session info).
type SessionController struct {
	logger      *slog.Logger
	docStore    *DocumentStoreService
	responder   *response.Writer
	crossOrigin bool
}

func newSessionController(deps SessionControllerDeps) *SessionController {
	return &SessionController{
		logger:      deps.Logger,
		docStore:    deps.DocStore,
		responder:   deps.Responder,
		crossOrigin: deps.CrossOrigin,
	}
}

// @Summary		Logout
// @Description	Clears the web session cookie and deletes the session from the database.
// @Tags			auth
// @Produce		json
// @Success		200	{object}	models.StatusResponse
// @Router			/api/v1/auth/logout [post]
func (c *SessionController) handlePublicAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(constants.WebSessionCookieName)
	if err == nil {
		// Best effort delete web session from DB
		if _, err := c.docStore.DocDelete(marshaler.CollectionName(constants.CollectionWebSessions), cookie.Value); err != nil {
			c.logger.Warn("Failed to delete web session during logout", "error", err, "sessionID", cookie.Value)
		}
	}

	// Clear cookie
	clearCookie := &http.Cookie{
		Name:     constants.WebSessionCookieName,
		Value:    "",
		Path:     constants.PathRoot,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	if c.crossOrigin {
		clearCookie.SameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, clearCookie)

	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

// @Summary		Get current web session
// @Description	Returns the authenticated user's ID and web session ID from the session cookie.
// @Tags			auth
// @Produce		json
// @Success		200	{object}	models.WebSessionResponse
// @Failure		401	{string}	string	"Unauthorized"
// @Router			/api/v1/auth/sessions/me [get]
func (c *SessionController) handleWebSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrNotAuthenticated.Error())
		return
	}

	cookie, _ := r.Cookie(constants.WebSessionCookieName)
	webSessionID := ""
	if cookie != nil {
		webSessionID = cookie.Value
	}

	c.responder.JSON(w, http.StatusOK, models.WebSessionResponse{
		Success:      true,
		UserID:       userID,
		WebSessionID: webSessionID,
	})
}

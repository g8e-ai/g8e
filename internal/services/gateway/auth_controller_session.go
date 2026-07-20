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
	"net/http"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// @Summary		Create user
// @Description	Creates a new user with a generated ID. Zero-PII: users are created with only
// @Description	a generated ID and passkey credentials.
// @Tags			users
// @Accept			json
// @Produce		json
// @Success		201		{object}	models.UserCreateResponse
// @Failure		405		{string}	string	"Method Not Allowed"
// @Failure		409		{string}	string	"Conflict — user creation failed"
// @Router			/api/v1/users [post]
func (c *AuthController) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	body, err := c.readBody(w, r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Zero-PII: Email-based user creation removed
	// Users are created with only a generated ID and passkey credentials
	user, err := c.userSvc.CreateUser()
	if err != nil {
		c.logger.Warn("Failed to create user", "error", err)
		c.responder.Error(w, http.StatusConflict, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusCreated, models.UserCreateResponse{
		Success: true,
		UserID:  user.ID,
	})
}

// Browser Auth Handlers (Public Router)
// =============================================================================

// @Summary		Logout
// @Description	Clears the web session cookie and deletes the session from the database.
// @Tags			auth
// @Produce		json
// @Success		200	{object}	models.StatusResponse
// @Router			/api/v1/auth/logout [post]
func (c *AuthController) handlePublicAuthLogout(w http.ResponseWriter, r *http.Request) {
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

// @Summary		Get current user
// @Description	Returns the authenticated user's profile. Identity comes from the auth context
// @Description	(mTLS or web session cookie).
// @Tags			users
// @Produce		json
// @Success		200	{object}	models.UserMeResponse
// @Failure		401	{string}	string	"Unauthorized"
// @Failure		404	{string}	string	"User not found"
// @Router			/api/v1/users/me [get]
func (c *AuthController) handleUserMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrNotAuthenticated.Error())
		return
	}

	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	if user == nil {
		c.responder.Error(w, http.StatusNotFound, constants.ErrUserNotFound.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.UserMeResponse{
		Success: true,
		User:    user,
	})
}

// @Summary		Get current web session
// @Description	Returns the authenticated user's ID and web session ID from the session cookie.
// @Tags			auth
// @Produce		json
// @Success		200	{object}	models.WebSessionResponse
// @Failure		401	{string}	string	"Unauthorized"
// @Router			/api/v1/auth/sessions/me [get]
func (c *AuthController) handleWebSession(w http.ResponseWriter, r *http.Request) {
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

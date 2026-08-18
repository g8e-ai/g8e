// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// UserControllerDeps groups all dependencies for UserController.
type UserControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	UserSvc   *UserService
	Responder *response.Writer
}

// UserController handles user CRUD and profile retrieval.
type UserController struct {
	cfg       *config.Config
	logger    *slog.Logger
	userSvc   *UserService
	responder *response.Writer
}

func newUserController(deps UserControllerDeps) *UserController {
	return &UserController{
		cfg:       deps.Cfg,
		logger:    deps.Logger,
		userSvc:   deps.UserSvc,
		responder: deps.Responder,
	}
}

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
func (c *UserController) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
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

// @Summary		Get current user
// @Description	Returns the authenticated user's profile. Identity comes from the auth context
// @Description	(mTLS or web session cookie).
// @Tags			users
// @Produce		json
// @Success		200	{object}	models.UserMeResponse
// @Failure		401	{string}	string	"Unauthorized"
// @Failure		404	{string}	string	"User not found"
// @Router			/api/v1/users/me [get]
func (c *UserController) handleUserMe(w http.ResponseWriter, r *http.Request) {
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

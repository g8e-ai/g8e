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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

type passkeyRequestSource int

const (
	sourceMTLS passkeyRequestSource = iota
	sourceJWT
	sourceCLIBootstrap
	sourceBrowserBootstrap
)

type passkeyHandlerConfig struct {
	source                     passkeyRequestSource
	enforceFirstCredentialOnly bool
	requireAuthenticatedUser   bool
	enforceSessionUserBinding  bool
	createWebSession           bool
	setCookie                  bool
	createUserOnBootstrap      bool
}

func (s *PasskeyService) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.maxPayload)
	return io.ReadAll(r.Body)
}

// enforceFirstCred checks whether a new registration is allowed. For CLI bootstrap,
// an existing mTLS-authenticated session for the same user bypasses the check
// (re-enrollment scenario). Returns (true, code, msg) to signal forbidden.
func (s *PasskeyService) enforceFirstCred(r *http.Request, userID string, cfg passkeyHandlerConfig) (forbidden bool, code int, msg string) {
	user, err := s.getUser(userID)
	if err != nil {
		return true, http.StatusInternalServerError, "failed to fetch user"
	}
	if user == nil {
		return true, http.StatusNotFound, constants.ErrUserNotFound.Error()
	}
	if len(user.PasskeyCredentials) == 0 {
		return false, 0, ""
	}
	if cfg.source == sourceCLIBootstrap {
		authenticatedUserID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
		if authenticatedUserID == userID {
			return false, 0, ""
		}
		return true, http.StatusForbidden, "first-credential registration only; user already has credentials"
	}
	return true, http.StatusForbidden, "first-credential registration only; user already has credentials, require step-up via existing passkey or mTLS"
}

// @Summary		Generate WebAuthn registration challenge
// @Description	Generates a WebAuthn registration challenge. Config-driven trust posture:
// @Description	mTLS (session-user binding), JIT (first-credential-only), CLI bootstrap (public), or console (public, may auto-create user).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Success		200			{object}	models.PasskeyRegisterChallengeResponse
// @Router			/api/v1/auth/passkeys/register/challenge [post]
func (s *PasskeyService) RegisterChallenge(cfg passkeyHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}

		body, err := s.readBody(r)
		if err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		var req struct {
			UserID       string `json:"user_id"`
			UserName     string `json:"user_name"`
			CLISessionID string `json:"cli_session_id"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		if cfg.enforceSessionUserBinding {
			if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
				if req.UserID != "" && req.UserID != ctxUserID {
					s.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
					return
				}
				req.UserID = ctxUserID
			}
		}

		var createdUserID string
		if req.UserID == "" {
			if !cfg.createUserOnBootstrap {
				s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
				return
			}
			hasUsers, err := s.userStore.HasAnyUsers()
			if err != nil {
				s.logger.Error("Failed to check for existing users during bootstrap", "error", err)
				if cfg.source == sourceBrowserBootstrap {
					s.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{Success: false})
					return
				}
				s.responder.Error(w, http.StatusInternalServerError, "failed to check bootstrap status")
				return
			}
			if hasUsers {
				s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
				return
			}
			newUser, err := s.userStore.CreateUser()
			if err != nil {
				s.logger.Error("Failed to create user during bootstrap", "error", err)
				if cfg.source == sourceBrowserBootstrap {
					s.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{Success: false})
					return
				}
				s.responder.Error(w, http.StatusInternalServerError, "failed to create user")
				return
			}
			req.UserID = newUser.ID
			createdUserID = newUser.ID
			s.logger.Info("[BOOTSTRAP] Auto-created user for browser passkey enrollment", "user_id", newUser.ID)
		}

		if cfg.enforceFirstCredentialOnly {
			if forbidden, code, msg := s.enforceFirstCred(r, req.UserID, cfg); forbidden {
				s.responder.Error(w, code, msg)
				return
			}
		}

		options, err := s.GenerateRegistrationChallenge(req.UserID, req.UserName)
		if err != nil {
			s.logger.Warn("Passkey register challenge failed", "error", err, "userID", req.UserID)
			if cfg.source == sourceBrowserBootstrap {
				s.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{Success: false})
				return
			}
			s.responder.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		s.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{
			Success: true,
			UserID:  createdUserID,
			Options: options,
		})
	}
}

// @Summary		Verify WebAuthn registration
// @Description	Verifies a WebAuthn attestation response and stores the credential. Config-driven trust posture:
// @Description	mTLS (session-user binding), JIT (first-credential-only), CLI bootstrap (public), or console (public, may mint web session + cookie).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Success		200			{object}	models.PasskeyVerifyResponse
// @Router			/api/v1/auth/passkeys/register/verify [post]
func (s *PasskeyService) RegisterVerify(cfg passkeyHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}

		body, err := s.readBody(r)
		if err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		var req struct {
			UserID              string                              `json:"user_id"`
			CLISessionID        string                              `json:"cli_session_id"`
			AttestationResponse *models.WebAuthnAttestationResponse `json:"attestation_response"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		if cfg.enforceSessionUserBinding {
			if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
				if req.UserID != "" && req.UserID != ctxUserID {
					s.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
					return
				}
				req.UserID = ctxUserID
			}
		}

		if req.UserID == "" {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
			return
		}

		if cfg.enforceFirstCredentialOnly {
			if forbidden, code, msg := s.enforceFirstCred(r, req.UserID, cfg); forbidden {
				s.responder.Error(w, code, msg)
				return
			}
		}

		responseJSON, err := json.Marshal(req.AttestationResponse)
		if err != nil {
			s.logger.Warn("Failed to marshal attestation response", "error", err, "userID", req.UserID)
			s.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
				Success: false,
				Error:   "failed to marshal attestation response",
			})
			return
		}

		cred, err := s.VerifyRegistration(req.UserID, responseJSON)
		if err != nil {
			s.logger.Warn("Passkey register verify failed", "error", err, "userID", req.UserID)
			s.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		if cfg.createWebSession {
			webSession, err := s.webSessionSvc.CreateWebSession(req.UserID)
			if err != nil {
				s.logger.Error("Failed to create web session after registration", "error", err, "userID", req.UserID)
				s.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
					Success: false,
					Error:   fmt.Sprintf("registration succeeded but web session creation failed: %v", err),
				})
				return
			}
			if cfg.setCookie {
				http.SetCookie(w, &http.Cookie{
					Name:     constants.WebSessionCookieName,
					Value:    webSession.ID,
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteStrictMode,
				})
			}
		}

		s.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
			Success:    true,
			Credential: cred,
		})
	}
}

// @Summary		Generate WebAuthn authentication challenge
// @Description	Generates a WebAuthn authentication challenge. Config-driven trust posture:
// @Description	mTLS (session-user binding), CLI bootstrap (public), or console (public).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Success		200			{object}	models.PasskeyChallengeResponse
// @Router			/api/v1/auth/passkeys/authenticate/challenge [post]
func (s *PasskeyService) AuthenticateChallenge(cfg passkeyHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}

		body, err := s.readBody(r)
		if err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		userID := req.UserID
		if cfg.enforceSessionUserBinding {
			if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
				if userID != "" && userID != ctxUserID {
					s.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
					return
				}
				userID = ctxUserID
			}
		}

		if userID == "" {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
			return
		}

		options, err := s.GenerateAuthenticationChallenge(userID)
		if err != nil {
			s.logger.Warn("Passkey auth challenge failed", "error", err, "userID", userID)
			s.responder.JSON(w, http.StatusOK, models.PasskeyChallengeResponse{
				Success:    false,
				Error:      err.Error(),
				NeedsSetup: errors.Is(err, constants.ErrNoPasskeysRegistered),
			})
			return
		}

		s.responder.JSON(w, http.StatusOK, models.PasskeyChallengeResponse{
			Success: true,
			Options: options,
		})
	}
}

// @Summary		Verify WebAuthn authentication
// @Description	Verifies a WebAuthn assertion response. Config-driven trust posture:
// @Description	mTLS (session-user binding, may mint web session in body), CLI bootstrap (public), or console (public, may mint web session + cookie).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Success		200			{object}	models.PasskeyAuthVerifyResponse
// @Router			/api/v1/auth/passkeys/authenticate/verify [post]
func (s *PasskeyService) AuthenticateVerify(cfg passkeyHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}

		body, err := s.readBody(r)
		if err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		var req struct {
			UserID            string                            `json:"user_id"`
			AssertionResponse *models.WebAuthnAssertionResponse `json:"assertion_response"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}

		userID := req.UserID
		if cfg.enforceSessionUserBinding {
			if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
				if userID != "" && userID != ctxUserID {
					s.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
					return
				}
				userID = ctxUserID
			}
		}

		if userID == "" {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
			return
		}

		responseJSON, err := json.Marshal(req.AssertionResponse)
		if err != nil {
			s.logger.Warn("Failed to marshal assertion response", "error", err, "userID", userID)
			s.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
				Success: false,
				Error:   "failed to marshal assertion response",
			})
			return
		}

		cred, err := s.VerifyAuthentication(userID, responseJSON)
		if err != nil {
			s.logger.Warn("Passkey auth verify failed", "error", err, "userID", userID)
			s.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		resp := models.PasskeyAuthVerifyResponse{
			Success:    true,
			UserID:     userID,
			Credential: cred,
		}

		if cfg.createWebSession {
			webSession, err := s.webSessionSvc.CreateWebSession(userID)
			if err != nil {
				s.logger.Error("Failed to create web session after auth", "error", err, "userID", userID)
				s.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
					Success: false,
					Error:   "authentication succeeded but session creation failed",
				})
				return
			}
			if cfg.setCookie {
				http.SetCookie(w, &http.Cookie{
					Name:     constants.WebSessionCookieName,
					Value:    webSession.ID,
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteStrictMode,
				})
			} else {
				// mTLS step-up: return session in body for the CLI to consume; no browser cookie.
				resp.WebSession = &models.WebSessionInfo{
					ID:              webSession.ID,
					ExpiresAtUnixMs: webSession.ExpiresAtUnixMs,
				}
			}
		}

		s.responder.JSON(w, http.StatusOK, resp)
	}
}

// @Summary		List passkey credentials
// @Description	Lists all WebAuthn credentials for the authenticated user (WebSession-protected).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Param			user_id		query		string		true		"User ID"
// @Success		200		{object}	models.PasskeyCredentialsResponse
// @Router			/api/v1/auth/passkeys [get]
func (s *PasskeyService) ListCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
		return
	}

	creds, err := s.listCredentials(userID)
	if err != nil {
		s.logger.Error("Failed to list credentials", "error", err, "userID", userID)
		s.responder.Error(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}

	s.responder.JSON(w, http.StatusOK, models.PasskeyCredentialsResponse{
		Success:     true,
		Credentials: creds,
	})
}

// @Summary		Revoke passkey credential
// @Description	Revokes a WebAuthn credential by ID (WebSession-protected).
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Param			user_id		query		string		true		"User ID"
// @Param			id			path		string		true		"Credential ID"
// @Success		200		{object}	models.PasskeyRevokeResponse
// @Router			/api/v1/auth/passkeys/{id} [delete]
func (s *PasskeyService) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		s.responder.Error(w, http.StatusBadRequest, constants.ErrUserIDRequired.Error())
		return
	}

	credentialID := strings.TrimPrefix(r.URL.Path, constants.APIPaths.AuthPasskeysPrefix)
	if credentialID == "" {
		s.responder.Error(w, http.StatusBadRequest, constants.ErrCredentialIDRequired.Error())
		return
	}

	found, remaining, err := s.revokeCredential(userID, credentialID)
	if err != nil {
		s.logger.Error("Failed to revoke credential", "error", err, "userID", userID)
		s.responder.Error(w, http.StatusInternalServerError, "failed to revoke credential")
		return
	}

	s.responder.JSON(w, http.StatusOK, models.PasskeyRevokeResponse{
		Success:   true,
		Found:     found,
		Remaining: remaining,
	})
}

// @Summary		CLI passkey status
// @Description	Returns the passkey credential list for the mTLS-authenticated user. Identity comes from the mTLS auth context; user_id query params are ignored.
// @Tags			passkey
// @Accept			json
// @Produce		json
// @Success		200		{object}	models.PasskeyCredentialsResponse
// @Router			/api/v1/auth/passkeys/cli/status [get]
func (s *PasskeyService) CLIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		s.responder.Error(w, http.StatusUnauthorized, constants.ErrUserIDRequired.Error())
		return
	}

	creds, err := s.listCredentials(userID)
	if err != nil {
		s.logger.Error("Failed to list credentials for CLI status", "error", err, "userID", userID)
		s.responder.Error(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}

	s.responder.JSON(w, http.StatusOK, models.PasskeyCredentialsResponse{
		Success:     true,
		Credentials: creds,
	})
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

type AuditorKeyProvider interface {
	GetAuditorHMACKey() (string, error)
}

// handleReputationSign signs the app's scoreboard commitment with Gateway-held
// key material. It does not attest the app's score or authorize execution.
//
// @Summary Sign an application reputation commitment
// @Description App mTLS only. Returns an HMAC signature over merkle_root || prev_root || tribunal_command_id. The signing key remains in the Gateway keystore. Signing does not validate the application's scores or satisfy governance proof requirements.
// @Tags gateway
// @Accept json
// @Produce json
// @Param request body models.ReputationSignRequest true "Reputation commitment claim"
// @Success 200 {object} models.ReputationSignResponse
// @Failure 400 {string} string "Invalid commitment"
// @Failure 403 {string} string "App identity required"
// @Failure 503 {string} string "Signing unavailable"
// @Router /api/v1/gateway/reputation/sign [post]
func (c *DataController) handleReputationSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	appID, _ := r.Context().Value(constants.ContextKeyAppID).(string)
	if appID == "" {
		c.responder.Error(w, http.StatusForbidden, constants.ErrForbidden.Error())
		return
	}
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	var request models.ReputationSignRequest
	if err := json.Unmarshal(body, &request); err != nil || !validReputationSignRequest(request) {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrReputationCommitmentInvalid.Error())
		return
	}
	if c.auditorKeyProvider == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrReputationSignerUnavailable.Error())
		return
	}
	key, err := c.auditorKeyProvider.GetAuditorHMACKey()
	if err != nil || key == "" {
		c.logger.Error("gateway: reputation signing key unavailable", "error", err)
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrReputationSignerUnavailable.Error())
		return
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(request.MerkleRoot + request.PrevRoot + request.TribunalCommandID))
	c.responder.JSON(w, http.StatusOK, models.ReputationSignResponse{Signature: hex.EncodeToString(mac.Sum(nil))})
}

func validReputationSignRequest(request models.ReputationSignRequest) bool {
	if strings.TrimSpace(request.TribunalCommandID) == "" {
		return false
	}
	for _, root := range []string{request.MerkleRoot, request.PrevRoot} {
		decoded, err := hex.DecodeString(root)
		if err != nil || len(decoded) != sha256.Size || root != strings.ToLower(root) {
			return false
		}
	}
	return true
}

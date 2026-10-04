// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/dispatch"
)

// EnsembleBrowserProxyController forwards browser-authenticated requests to g8ee
// with Gateway-stamped identity. Browsers never call g8ee directly. The stamped
// identity includes the Operators bound to the caller's web session, read from
// the Gateway registry; browser-supplied bound_operators are discarded.
type EnsembleBrowserProxyController struct {
	cfg       *config.Config
	logger    *slog.Logger
	responder *response.Writer
	operators dispatch.OperatorLister
	signer    *BrowserProxySigner
	client    *http.Client
}

type EnsembleBrowserProxyControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	Responder *response.Writer
	Operators dispatch.OperatorLister
	// Signer signs every proxied request. A nil Signer makes the proxy refuse
	// to forward anything: there is no unsigned mode.
	Signer *BrowserProxySigner
}

func newEnsembleBrowserProxyController(d EnsembleBrowserProxyControllerDeps) *EnsembleBrowserProxyController {
	return &EnsembleBrowserProxyController{
		cfg:       d.Cfg,
		logger:    d.Logger,
		responder: d.Responder,
		operators: d.Operators,
		signer:    d.Signer,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *EnsembleBrowserProxyController) upstreamBase() string {
	base := strings.TrimSpace(c.cfg.Gateway.EnsembleUpstreamURL)
	if base == "" {
		base = constants.DefaultEnsembleUpstreamURL
	}
	return strings.TrimRight(base, "/")
}

func (c *EnsembleBrowserProxyController) handleProxy(w http.ResponseWriter, r *http.Request) {
	if c.signer == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrBrowserProxySignerUnavailable.Error())
		return
	}
	userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	webSessionID, _ := r.Context().Value(constants.ContextKeyWebSessionID).(string)
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(webSessionID) == "" {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrProtocolAuthRequired.Error())
		return
	}

	upstreamPath := r.URL.Path
	method := r.Method
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Compatibility: browser GET /api/v1/investigations?... -> g8ee POST /api/v1/investigations/query
	if method == http.MethodGet && upstreamPath == constants.APIPaths.EnsembleInvestigations {
		method = http.MethodPost
		upstreamPath = constants.APIPaths.EnsembleInvestigationsQuery
		body, err = c.investigationsQueryBody(r, userID, webSessionID)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if len(body) > 0 && method != http.MethodGet && method != http.MethodHead {
		body, err = injectBrowserContext(body, userID, webSessionID, c.boundOperators(userID, webSessionID))
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
	}

	target, err := url.Parse(c.upstreamBase() + upstreamPath)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	if r.URL.RawQuery != "" {
		target.RawQuery = r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), method, target.String(), bytes.NewReader(body)) //nolint:gosec // G704: target is constructed from configured upstreamBase and allowlisted path
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}

	if err := c.signer.Apply(req, body, BrowserProxyIdentity{
		UserID:       userID,
		UserEmail:    userID + "@g8e.local",
		WebSessionID: webSessionID,
	}); err != nil {
		c.logger.Error("gateway: ensemble browser proxy could not sign request", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	} else if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", r.Header.Get("Accept"))

	resp, err := c.client.Do(req) //nolint:gosec // G704: proxy call to configured ensemble upstream URL
	if err != nil {
		c.logger.Warn("gateway: ensemble browser proxy upstream failed", "path", upstreamPath, "error", err)
		c.responder.Error(w, http.StatusBadGateway, "ensemble upstream unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.logger.Warn("gateway: ensemble browser proxy rejected proxy stamp", "path", upstreamPath)
		c.responder.Error(w, http.StatusBadGateway, "ensemble upstream authentication failed")
		return
	}

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		c.logger.Warn("gateway: ensemble browser proxy response copy failed", "path", upstreamPath, "error", err)
	}
}

// handleProxySigningKey serves the public half of the proxy signing key so g8ee
// can verify stamps without sharing a volume or a file with the Gateway. The
// route is mTLS-only; the key is public.
//
// @Summary		Browser proxy signing key
// @Description	Returns the Ed25519 public key (and its key ID) the Gateway uses to sign browser-proxy identity stamps. g8ee fetches it over its mTLS client and verifies every proxied request against it.
// @Tags			gateway
// @Produce		json
// @Success		200	{object}	models.ActuatorPublicKeyExport
// @Failure		405	{string}	string	"Method Not Allowed"
// @Failure		503	{string}	string	"Service Unavailable — no signing key is loaded"
// @Router			/api/v1/gateway/proxy-signing-key [get]
func (c *EnsembleBrowserProxyController) handleProxySigningKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	if c.signer == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrBrowserProxySignerUnavailable.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.ActuatorPublicKeyExport{
		KeyID:     c.signer.KeyID(),
		PublicKey: hex.EncodeToString(c.signer.PublicKey()),
		Algorithm: "ed25519",
	})
}

// browserBoundOperator is one entry of the Gateway-stamped bound_operators
// list. It mirrors the protocol BoundOperator shape g8ee parses from the
// request context. Field order matches encoding/json map key order.
type browserBoundOperator struct {
	BoundWebSessionID string `json:"bound_web_session_id"`
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	Status            string `json:"status,omitempty"`
}

// boundOperators returns the caller's Operators that the registry shows bound
// to webSessionID. A registry failure yields an empty list, so the request
// proceeds with no Operator authority rather than with unverified bindings.
func (c *EnsembleBrowserProxyController) boundOperators(userID, webSessionID string) []browserBoundOperator {
	bound := []browserBoundOperator{}
	if c.operators == nil {
		return bound
	}
	ops, err := c.operators.ListUserOperators(userID)
	if err != nil {
		c.logger.Warn("gateway: ensemble browser proxy could not list bound operators", "error", err)
		return bound
	}
	for _, op := range ops {
		if op.BoundWebSessionID != webSessionID {
			continue
		}
		// The registry tracks lifecycle separately from the web-session
		// binding. Binding an active Operator leaves its document status
		// active; g8ee's BoundOperator status describes the binding instead.
		status := op.Status
		if status == constants.OperatorStatusActive {
			status = constants.OperatorStatusBound
		}
		bound = append(bound, browserBoundOperator{
			BoundWebSessionID: op.BoundWebSessionID,
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(status),
		})
	}
	return bound
}

// browserProxyContext is the identity object the browser proxy stamps onto
// ensemble requests. Field order matches encoding/json map key order.
type browserProxyContext struct {
	UserID       string `json:"user_id"`
	WebSessionID string `json:"web_session_id"`
}

// browserInvestigationsQuery is the body of the GET investigations compatibility
// rewrite. UserID is always the session user so g8ee scopes the query to the
// caller; a query-string user_id is never honored. Field order matches
// encoding/json map key order.
type browserInvestigationsQuery struct {
	CaseID            string              `json:"case_id,omitempty"`
	Context           browserProxyContext `json:"context"`
	InvestigationType string              `json:"investigation_type,omitempty"`
	Limit             int                 `json:"limit"`
	OrderBy           string              `json:"order_by,omitempty"`
	OrderDirection    string              `json:"order_direction,omitempty"`
	Priority          string              `json:"priority,omitempty"`
	Status            string              `json:"status,omitempty"`
	UserID            string              `json:"user_id"`
	WebSessionID      string              `json:"web_session_id,omitempty"`
}

func (c *EnsembleBrowserProxyController) investigationsQueryBody(r *http.Request, userID, webSessionID string) ([]byte, error) {
	payload := browserInvestigationsQuery{
		Context: browserProxyContext{
			UserID:       userID,
			WebSessionID: webSessionID,
		},
		Limit:  20,
		UserID: userID,
	}
	for key, vals := range r.URL.Query() {
		if len(vals) == 0 {
			continue
		}
		switch key {
		case "case_id":
			payload.CaseID = vals[0]
		case "web_session_id":
			payload.WebSessionID = vals[0]
		case "status":
			payload.Status = vals[0]
		case "investigation_type":
			payload.InvestigationType = vals[0]
		case "priority":
			payload.Priority = vals[0]
		case "order_by":
			payload.OrderBy = vals[0]
		case "order_direction":
			payload.OrderDirection = vals[0]
		case "limit":
			var n int
			if _, err := fmt.Sscanf(vals[0], "%d", &n); err == nil && n > 0 {
				payload.Limit = n
			}
		}
	}
	return json.Marshal(payload)
}

// injectBrowserContext stamps browser identity and the session's bound
// Operators onto a JSON object body, replacing any caller-supplied values.
// The outer document and any extra context keys are caller-defined JSON with
// no stable schema, so they stay map[string]interface{} and round-trip.
func injectBrowserContext(body []byte, userID, webSessionID string, bound []browserBoundOperator) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	ctx, _ := payload["context"].(map[string]interface{})
	if ctx == nil {
		ctx = map[string]interface{}{}
	}
	ctx["user_id"] = userID
	ctx["web_session_id"] = webSessionID
	ctx["bound_operators"] = bound
	if caseID, ok := payload["case_id"].(string); ok && caseID != "" {
		ctx["case_id"] = caseID
	}
	if invID, ok := payload["investigation_id"].(string); ok && invID != "" {
		ctx["investigation_id"] = invID
	}
	payload["context"] = ctx
	return json.Marshal(payload)
}

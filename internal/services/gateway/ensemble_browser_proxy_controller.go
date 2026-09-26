// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
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
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// EnsembleBrowserProxyController forwards browser-authenticated requests to g8ee
// with Gateway-stamped identity. Browsers never call g8ee directly.
type EnsembleBrowserProxyController struct {
	cfg       *config.Config
	logger    *slog.Logger
	responder *response.Writer
	client    *http.Client
}

type EnsembleBrowserProxyControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	Responder *response.Writer
}

func newEnsembleBrowserProxyController(d EnsembleBrowserProxyControllerDeps) *EnsembleBrowserProxyController {
	return &EnsembleBrowserProxyController{
		cfg:       d.Cfg,
		logger:    d.Logger,
		responder: d.Responder,
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
		body, err = injectBrowserContext(body, userID, webSessionID)
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

	req, err := http.NewRequestWithContext(r.Context(), method, target.String(), bytes.NewReader(body))
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
		return
	}

	req.Header.Set(constants.HeaderGatewayBrowserProxy, constants.GatewayBrowserProxyValue)
	req.Header.Set(constants.HeaderProxyUserID, userID)
	req.Header.Set(constants.HeaderProxyUserEmail, userID+"@g8e.local")
	req.Header.Set(constants.HeaderProxyWebSessionID, webSessionID)
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	} else if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", r.Header.Get("Accept"))

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("gateway: ensemble browser proxy upstream failed", "path", upstreamPath, "error", err)
		c.responder.Error(w, http.StatusBadGateway, "ensemble upstream unavailable")
		return
	}
	defer resp.Body.Close()

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

func (c *EnsembleBrowserProxyController) investigationsQueryBody(r *http.Request, userID, webSessionID string) ([]byte, error) {
	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"user_id":        userID,
			"web_session_id": webSessionID,
		},
		"limit": 20,
	}
	for key, vals := range r.URL.Query() {
		if len(vals) == 0 {
			continue
		}
		switch key {
		case "case_id", "web_session_id", "status", "investigation_type", "priority", "order_by", "order_direction":
			payload[key] = vals[0]
		case "limit":
			var n int
			if _, err := fmt.Sscanf(vals[0], "%d", &n); err == nil && n > 0 {
				payload["limit"] = n
			}
		}
	}
	return json.Marshal(payload)
}

func injectBrowserContext(body []byte, userID, webSessionID string) ([]byte, error) {
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
	if caseID, ok := payload["case_id"].(string); ok && caseID != "" {
		ctx["case_id"] = caseID
	}
	if invID, ok := payload["investigation_id"].(string); ok && invID != "" {
		ctx["investigation_id"] = invID
	}
	payload["context"] = ctx
	return json.Marshal(payload)
}

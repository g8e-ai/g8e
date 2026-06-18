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
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// @Summary		Landing page
// @Description	Returns the public landing page for the gateway
// @Tags			public
// @Accept			html
// @Produce		html
// @Success		200	{string}	string
// @Router			/ [get]
func (h *HTTPHandler) handleLandingPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	if host == "" {
		host = "localhost"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>g8e Operator - Public Entry</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; line-height: 1.6; }
        .container { border: 1px solid #ddd; border-radius: 8px; padding: 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.05); }
        h1 { color: #2c3e50; margin-top: 0; }
        .section { margin-bottom: 30px; }
        .label { font-weight: bold; color: #34495e; }
        code { background: #f8f9fa; padding: 2px 5px; border-radius: 4px; border: 1px solid #e9ecef; }
        .btn { display: inline-block; background: #3498db; color: white; padding: 10px 20px; text-decoration: none; border-radius: 4px; margin-top: 10px; }
        .btn:hover { background: #2980b9; }
        .footer { margin-top: 40px; font-size: 0.9em; color: #7f8c8d; border-top: 1px solid #eee; padding-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>g8e Operator</h1>
        <p>You have reached the public entry point for the g8e Operator Gateway.</p>

        <div class="section">
            <div class="label">Trust & Security</div>
            <p>To use this Operator from your browser or as a BYO client, you must first install the platform's root certificate. If you see a "Not Secure" warning, please provide your own valid client certificate for mTLS operations.</p>
        </div>

        <div class="section">
            <div class="label">Next Steps</div>
            <ul>
                <li><a href="/api/auth/login/challenge">Check Login Capabilities</a></li>
                <li><a href="https://github.com/g8e-ai/g8e/docs" target="_blank">Read Documentation</a></li>
            </ul>
        </div>

        <div class="footer">
            Sovereign g8e Gateway &copy; 2026 Lateralus Labs, LLC.
        </div>
    </div>
</body>
</html>
`, html.EscapeString(host))
}

// @Summary		Health check
// @Description	Returns the current health status of the gateway
// @Tags			health
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.HealthResponse
// @Router			/health [get]
func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	doc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
	if err != nil {
		h.logger.Error("Health check failed to query platform_settings", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}
	if doc == nil {
		h.logger.Warn("Health check: platform_settings not found")
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}

	root, err := h.db.StateRootSvc.GetCurrentStateRoot()
	if err != nil {
		h.logger.Error("Health check failed to calculate state root", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "state root calculation failed")
		return
	}

	h.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
		StateMerkleRoot: root,
	})
}

// @Summary		Bootstrap health check
// @Description	Returns the current health status during bootstrap (no state root check)
// @Tags			health
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.HealthResponse
// @Router			/health/bootstrap [get]
func (h *HTTPHandler) handleBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	h.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeGateway,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
	})
}

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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
)

// PKIController handles PKI and certificate management endpoints.
type PKIController struct {
	cfg           *config.Config
	logger        *slog.Logger
	db            *GatewayDBService
	pki           *PKIAuthority
	appEnrollment *AppEnrollmentService
	registration  *RegistrationService
	responder     *responder.Responder
}

func newPKIController(cfg *config.Config, logger *slog.Logger, db *GatewayDBService, pki *PKIAuthority, appEnrollment *AppEnrollmentService, registration *RegistrationService, responder *responder.Responder) *PKIController {
	return &PKIController{
		cfg:           cfg,
		logger:        logger,
		db:            db,
		pki:           pki,
		appEnrollment: appEnrollment,
		registration:  registration,
		responder:     responder,
	}
}

func (c *PKIController) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

func (c *PKIController) handlePKICABundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pem, err := c.pki.HubTrustBundle()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to read hub bundle")
		return
	}
	w.Header().Set(constants.HeaderContentType, "application/x-pem-file")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pem)
}

func (c *PKIController) handlePKIFingerprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pemData, err := os.ReadFile(filepath.Join(c.pki.PKIDir(), "root", "root_ca.crt"))
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to read root CA")
		return
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		c.responder.Error(w, http.StatusInternalServerError, "invalid root CA PEM")
		return
	}

	hash := sha256.Sum256(block.Bytes)
	fingerprint := hex.EncodeToString(hash[:])

	c.responder.JSON(w, http.StatusOK, map[string]string{
		"root_ca": "sha256:" + fingerprint,
	})
}

func (c *PKIController) handlePKICSRSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		CSR               string `json:"csr_pem"`
		LeafType          string `json:"leaf_type"`
		OrganizationID    string `json:"organization_id"`
		OperatorID        string `json:"operator_id"`
		UserID            string `json:"user_id"`
		WorkloadSessionID string `json:"workload_session_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	certPEM, chainPEM, err := c.pki.SignCSR(req.CSR, req.LeafType, req.OrganizationID, req.OperatorID, req.UserID, req.WorkloadSessionID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, map[string]string{
		"certificate_pem":       certPEM,
		"certificate_chain_pem": chainPEM,
	})
}

func (c *PKIController) handlePKICertificatesRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Serial string `json:"serial"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Serial == "" {
		c.responder.Error(w, http.StatusBadRequest, "serial required")
		return
	}

	if err := c.pki.RevokeCertificate(req.Serial, req.Reason); err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

func (c *PKIController) handlePKIRevocationBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	bundleJSON, signature, err := c.pki.GenerateRevocationBundle()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, map[string]string{
		"bundle_json": bundleJSON,
		"signature":   signature,
	})
}

func (c *PKIController) handlePKIDevicesEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if c.registration == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, "registration service not available")
		return
	}

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		c.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate required")
		return
	}

	userID, err := ExtractUserIDFromCert(r.TLS.PeerCertificates[0])
	if err != nil {
		c.responder.Error(w, http.StatusUnauthorized, "failed to extract user_id from client certificate")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.OperatorRegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	organizationID := ""
	if req.CSR == "" {
		c.responder.Error(w, http.StatusBadRequest, "csr_pem is required")
		return
	}

	resp, err := c.registration.RegisterDeviceCSR(userID, organizationID, req)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusCreated, resp)
}

func (c *PKIController) handleTrustScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	script := `#!/bin/sh
set -e

GATEWAY_HOST="${GATEWAY_HOST:-localhost}"
GATEWAY_PORT="${GATEWAY_PORT:-8441}"
CA_BUNDLE_URL="http://${GATEWAY_HOST}:${GATEWAY_PORT}/.well-known/g8e/pki/ca-bundle"
CA_PATH="/usr/local/share/ca-certificates/g8e-gateway-ca.crt"

echo "[g8e] Fetching platform CA bundle from ${CA_BUNDLE_URL}..."
curl -fsSL "${CA_BUNDLE_URL}" -o "${CA_PATH}"

if [ ! -f "${CA_PATH}" ]; then
    echo "[g8e] ERROR: Failed to download CA bundle"
    exit 1
fi

echo "[g8e] Installing CA bundle to system trust store..."
update-ca-certificates

echo "[g8e] CA bundle installed successfully"
echo "[g8e] You can now use: ./g8e auth login"
`

	w.Header().Set("Content-Type", "application/x-sh")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) handleTrustScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	script := `$ErrorActionPreference = "Stop"

$GatewayHost = if ($env:GATEWAY_HOST) { $env:GATEWAY_HOST } else { "localhost" }
$GatewayPort = if ($env:GATEWAY_PORT) { $env:GATEWAY_PORT } else { "8441" }
$CABundleUrl = "http://${GatewayHost}:${GatewayPort}/.well-known/g8e/pki/ca-bundle"
$TempPath = "$env:TEMP\g8e-gateway-ca.crt"
$CertStorePath = "Cert:\LocalMachine\Root"

Write-Host "[g8e] Fetching platform CA bundle from ${CABundleUrl}..."
Invoke-RestMethod -Uri $CABundleUrl -OutFile $TempPath

if (-not (Test-Path $TempPath)) {
    Write-Host "[g8e] ERROR: Failed to download CA bundle"
    exit 1
}

Write-Host "[g8e] Installing CA bundle to system trust store..."
Import-Certificate -FilePath $TempPath -CertStoreLocation $CertStorePath

Write-Host "[g8e] CA bundle installed successfully"
Write-Host "[g8e] You can now use: .\g8e.exe auth login"

Remove-Item $TempPath -Force
`

	w.Header().Set("Content-Type", "application/x-powershell")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

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
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
)

// PKIController handles PKI and certificate management endpoints.
type PKIController struct {
	cfg           *config.Config
	logger        *slog.Logger
	db            *CanonicalDBService
	pki           *PKIAuthority
	appEnrollment *AppEnrollmentService
	registration  *RegistrationService
	responder     *response.Writer
}

func newPKIController(cfg *config.Config, logger *slog.Logger, db *CanonicalDBService, pki *PKIAuthority, appEnrollment *AppEnrollmentService, registration *RegistrationService, responder *response.Writer) *PKIController {
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
	bundlePath := filepath.Join(c.pki.PKIDir(), "trust", "g8eg-ca-bundle.pem")
	c.logger.Debug("Attempting to read trust bundle", "path", bundlePath, "pki_dir", c.pki.PKIDir())
	pem, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Error("Failed to read trust bundle", "error", err, "bundle_path", bundlePath, "pki_dir", c.pki.PKIDir())
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: read trust bundle: %w", err).Error())
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
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: read root CA: %w", err).Error())
		return
	}

	block, rest := pem.Decode(pemData)
	if block == nil {
		c.responder.Error(w, http.StatusInternalServerError, "invalid root CA PEM")
		return
	}
	if len(rest) > 0 {
		c.logger.Warn("Unexpected data after PEM block", "extra_bytes", len(rest))
	}

	hash := sha256.Sum256(block.Bytes)
	fingerprint := hex.EncodeToString(hash[:])

	c.responder.JSON(w, http.StatusOK, models.PKIFingerprintResponse{
		RootCA: "sha256:" + fingerprint,
	})
}

func (c *PKIController) handlePKICSRSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: read request body: %w", err).Error())
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
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: unmarshal CSR sign request: %w", err).Error())
		return
	}

	certPEM, chainPEM, err := c.pki.SignCSR(req.CSR, req.LeafType, req.OrganizationID, req.OperatorID, req.UserID, req.WorkloadSessionID, "")
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: sign CSR: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PKICSRSignResponse{
		CertificatePEM:       certPEM,
		CertificateChainPEM: chainPEM,
	})
}

func (c *PKIController) handlePKICertificatesRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: read request body: %w", err).Error())
		return
	}

	var req struct {
		Serial string `json:"serial"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: unmarshal revoke request: %w", err).Error())
		return
	}

	if req.Serial == "" {
		c.responder.Error(w, http.StatusBadRequest, "serial required")
		return
	}

	if err := c.pki.RevokeCertificate(req.Serial, req.Reason); err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: revoke certificate: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

func (c *PKIController) handlePKIRevocationBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	crlDER, err := c.pki.GenerateCRL()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: generate CRL: %w", err).Error())
		return
	}

	w.Header().Set("Content-Type", "application/pkix-crl")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(crlDER)
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
		c.responder.Error(w, http.StatusUnauthorized, fmt.Errorf("pki: extract user ID from certificate: %w", err).Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: read request body: %w", err).Error())
		return
	}

	var req models.OperatorRegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("pki: unmarshal enrollment request: %w", err).Error())
		return
	}

	// Device enrollment does not require an organization context
	organizationID := ""
	if req.CSR == "" {
		c.responder.Error(w, http.StatusBadRequest, "csr_pem is required")
		return
	}

	resp, err := c.registration.RegisterDeviceCSR(userID, organizationID, req)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: register device CSR: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusCreated, resp)
}

func (c *PKIController) handleTrustScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	script := `#!/bin/sh
set -e

GATEWAY_HOST="${GATEWAY_HOST:-localhost}"
GATEWAY_PORT="${GATEWAY_PORT:-` + port + `}"
CA_BUNDLE_URL="http://${GATEWAY_HOST}:${GATEWAY_PORT}/.well-known/g8e/pki/ca-bundle"
LOCAL_CA_PATH=".g8e/pki/trust/g8eg-ca-bundle.pem"

echo "[g8e] Fetching platform CA bundle from ${CA_BUNDLE_URL}..."
mkdir -p "$(dirname "${LOCAL_CA_PATH}")"
curl -fsSL "${CA_BUNDLE_URL}" -o "${LOCAL_CA_PATH}"

if [ ! -f "${LOCAL_CA_PATH}" ]; then
    echo "[g8e] ERROR: Failed to download CA bundle"
    exit 1
fi

echo "[g8e] CA bundle installed to ${LOCAL_CA_PATH}"
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

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	script := "$ErrorActionPreference = \"Continue\"\n\n" +
		"$GatewayHost = if ($env:GATEWAY_HOST) { $env:GATEWAY_HOST } else { \"localhost\" }\n" +
		"$GatewayPort = if ($env:GATEWAY_PORT) { $env:GATEWAY_PORT } else { \"" + port + "\" }\n" +
		"$CABundleUrl = \"http://${GatewayHost}:${GatewayPort}/.well-known/g8e/pki/ca-bundle\"\n" +
		"$LocalCAPath = \".g8e/pki/trust/g8eg-ca-bundle.pem\"\n" +
		"$Node BinaryName = \"g8e-windows-amd64.exe\"\n" +
		"$Node BinaryUrl = \"http://${GatewayHost}:${GatewayPort}/.well-known/g8e/bin/g8e-windows-amd64.exe\"\n\n" +
		"Write-Host \"[g8e] Fetching platform CA bundle from ${CABundleUrl}...\"\n" +
		"$LocalDir = Split-Path -Parent $LocalCAPath\n" +
		"if (-not (Test-Path $LocalDir)) {\n" +
		"    New-Item -ItemType Directory -Path $LocalDir -Force | Out-Null\n" +
		"}\n" +
		"try {\n" +
		"    Invoke-RestMethod -Uri $CABundleUrl -OutFile $LocalCAPath\n" +
		"} catch {\n" +
		"    Write-Host \"[g8e] ERROR: Failed to download CA bundle: $_\"\n" +
		"    return\n" +
		"}\n\n" +
		"if (-not (Test-Path $LocalCAPath)) {\n" +
		"    Write-Host \"[g8e] ERROR: Failed to download CA bundle\"\n" +
		"    return\n" +
		"}\n\n" +
		"Write-Host \"[g8e] CA bundle installed to ${LocalCAPath}\"\n\n" +
		"# Download g8e Node\n" +
		"Write-Host \"[g8e] Downloading g8e Node from ${Node BinaryUrl}...\"\n" +
		"try {\n" +
		"    Invoke-RestMethod -Uri $Node BinaryUrl -OutFile $Node BinaryName\n" +
		"} catch {\n" +
		"    Write-Host \"[g8e] ERROR: Failed to download g8e Node: $_\"\n" +
		"    return\n" +
		"}\n" +
		"\n" +
		"if (-not (Test-Path $Node BinaryName)) {\n" +
		"    Write-Host \"[g8e] ERROR: Failed to download g8e Node\"\n" +
		"    return\n" +
		"}\n\n" +
		"Write-Host \"[g8e]Node Node Binary downloaded to ${Node BinaryName}\"\n\n" +
		"# Run enrollment\n" +
		"Write-Host \"[g8e] Running PKI enrollment with endpoint ${GatewayHost}...\"\n" +
		"& .\\$Node BinaryName security pki enroll --endpoint \"${GatewayHost}\"\n" +
		"\n" +
		"if ($LASTEXITCODE -ne 0) {\n" +
		"    Write-Host \"[g8e] ERROR: Enrollment failed with exit code ${LASTEXITCODE}\"\n" +
		"    return\n" +
		"}\n\n" +
		"Write-Host \"[g8e] Enrollment complete\"\n\n" +
		"# Start the operator\n" +
		"Write-Host \"[g8e] Starting Operator with endpoint ${GatewayHost}...\"\n" +
		"Write-Host \"[g8e] The Operator will run in this terminal. Press Ctrl+C to stop.\"\n" +
		"& .\\$Node BinaryName -e $GatewayHost\n"

	w.Header().Set("Content-Type", "application/x-powershell")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) handleTrustScriptWindowsAlias(w http.ResponseWriter, r *http.Request) {
	c.handleTrustScriptWindows(w, r)
}

func (c *PKIController) handleNodeBinaryDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filename := filepath.Base(r.URL.Path)
	if filename == "" || filename == "." {
		c.responder.Error(w, http.StatusBadRequest, "invalid filename")
		return
	}

	binaryPattern := regexp.MustCompile(`^g8e-(linux|darwin|windows)-(amd64|arm64|386)(\.exe)?$`)
	if !binaryPattern.MatchString(filename) {
		c.responder.Error(w, http.StatusBadRequest, "invalid binary name")
		return
	}

	possiblePaths := []string{}

	if execPath, err := os.Executable(); err == nil {
		possiblePaths = append(possiblePaths, filepath.Join(filepath.Dir(execPath), "bin", filename))
	}

	possiblePaths = append(possiblePaths, filepath.Join(c.pki.PKIDir(), "binaries", filename))

	var binaryPath string
	var fileInfo os.FileInfo
	var err error

	for _, path := range possiblePaths {
		fileInfo, err = os.Stat(path)
		if err == nil && !fileInfo.IsDir() {
			binaryPath = path
			break
		}
	}

	if binaryPath == "" {
		c.responder.Error(w, http.StatusNotFound, fmt.Sprintf("binary not found: %s (checked: %v)", filename, possiblePaths))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	http.ServeFile(w, r, binaryPath)
}

func (c *PKIController) handleDeployScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	gatewayHost := c.extractGatewayHost(r.Host)
	data := scripts.TemplateData{
		GatewayHost: gatewayHost,
		GatewayPort: "8080",
	}

	script, err := scripts.RenderLinuxDeployScript(data)
	if err != nil {
		c.logger.Error("Failed to render Linux deploy script", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: render Linux deploy script: %w", err).Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-sh")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) handleDeployScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	gatewayHost := c.extractGatewayHost(r.Host)
	data := scripts.TemplateData{
		GatewayHost: gatewayHost,
		GatewayPort: "8080",
	}

	script, err := scripts.RenderWindowsDeployScript(data)
	if err != nil {
		c.logger.Error("Failed to render Windows deploy script", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: render Windows deploy script: %w", err).Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-powershell")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) extractGatewayHost(host string) string {
	if host == "" {
		return "localhost"
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

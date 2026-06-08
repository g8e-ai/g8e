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

	bundlePath := c.pki.TrustBundlePath()
	c.logger.Debug("Reading gateway trust bundle", "path", bundlePath)

	pemData, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Error("Failed to read gateway trust bundle", "error", err, "path", bundlePath)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: read trust bundle: %w", err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValuePEM)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pemData)
}

func (c *PKIController) handlePKIFingerprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rootCAPath := filepath.Join(c.pki.PKIDir(), constants.PkiSubdirRoot, constants.PkiFileRootCA)
	pemData, err := os.ReadFile(rootCAPath)
	if err != nil {
		c.logger.Error("Failed to read root CA", "error", err, "path", rootCAPath)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: read root CA: %w", err).Error())
		return
	}

	block, rest := pem.Decode(pemData)
	if block == nil {
		c.logger.Error("Invalid root CA PEM", "path", rootCAPath)
		c.responder.Error(w, http.StatusInternalServerError, "pki: invalid root CA PEM")
		return
	}
	if len(rest) > 0 {
		c.logger.Warn("Unexpected data after root CA PEM block", "extra_bytes", len(rest))
	}

	hash := sha256.Sum256(block.Bytes)
	fingerprint := hex.EncodeToString(hash[:])

	c.responder.JSON(w, http.StatusOK, models.PKIFingerprintResponse{
		RootCA: fingerprint,
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
		CertificatePEM:      certPEM,
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
		c.responder.Error(w, http.StatusBadRequest, "pki: serial required")
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

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueCRL)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(crlDER)
}

func (c *PKIController) handlePKIDevicesEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if c.registration == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, "pki: registration service not available")
		return
	}

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		c.responder.Error(w, http.StatusUnauthorized, "pki: mTLS client certificate required")
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
		c.responder.Error(w, http.StatusBadRequest, "pki: csr_pem is required")
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
	caBundleURL := constants.APIPaths.WellKnownPKICABundle
	localCAPath := filepath.ToSlash(constants.Paths.Infra.CaCertPath)

	script := fmt.Sprintf(`#!/bin/sh
set -e

GATEWAY_HOST="${GATEWAY_HOST:-localhost}"
GATEWAY_PORT="${GATEWAY_PORT:-%s}"
CA_BUNDLE_URL="http://${GATEWAY_HOST}:${GATEWAY_PORT}%s"
LOCAL_CA_PATH="%s"

echo "[g8e] Fetching platform CA bundle from ${CA_BUNDLE_URL}..."
mkdir -p "$(dirname "${LOCAL_CA_PATH}")"
curl -fsSL "${CA_BUNDLE_URL}" -o "${LOCAL_CA_PATH}"

if [ ! -f "${LOCAL_CA_PATH}" ]; then
    echo "[g8e] ERROR: Failed to download CA bundle"
    exit 1
fi

echo "[g8e] CA bundle installed to ${LOCAL_CA_PATH}"
echo "[g8e] You can now use: ./g8e auth login"
`, port, caBundleURL, localCAPath)

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) handleTrustScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	caBundleURL := constants.APIPaths.WellKnownPKICABundle
	localCAPath := filepath.ToSlash(constants.Paths.Infra.CaCertPath)
	binaryName := constants.BinaryNameWindows
	binaryURL := constants.APIPaths.WellKnownBinPrefix + binaryName

	script := fmt.Sprintf(`$ErrorActionPreference = "Continue"

$GatewayHost = if ($env:GATEWAY_HOST) { $env:GATEWAY_HOST } else { "localhost" }
$GatewayPort = if ($env:GATEWAY_PORT) { $env:GATEWAY_PORT } else { "%s" }
$CABundleUrl = "http://${GatewayHost}:${GatewayPort}%s"
$LocalCAPath = "%s"
$NodeBinaryName = "%s"
$NodeBinaryUrl = "http://${GatewayHost}:${GatewayPort}%s"

Write-Host "[g8e] Fetching platform CA bundle from ${CABundleUrl}..."
$LocalDir = Split-Path -Parent $LocalCAPath
if (-not (Test-Path $LocalDir)) {
    New-Item -ItemType Directory -Path $LocalDir -Force | Out-Null
}
try {
    Invoke-RestMethod -Uri $CABundleUrl -OutFile $LocalCAPath
} catch {
    Write-Host "[g8e] ERROR: Failed to download CA bundle: $_"
    return
}

if (-not (Test-Path $LocalCAPath)) {
    Write-Host "[g8e] ERROR: Failed to download CA bundle"
    return
}

Write-Host "[g8e] CA bundle installed to ${LocalCAPath}"

# Download g8e Node
Write-Host "[g8e] Downloading g8e Node from ${NodeBinaryUrl}..."
try {
    Invoke-RestMethod -Uri $NodeBinaryUrl -OutFile $NodeBinaryName
} catch {
    Write-Host "[g8e] ERROR: Failed to download g8e Node: $_"
    return
}

if (-not (Test-Path $NodeBinaryName)) {
    Write-Host "[g8e] ERROR: Failed to download g8e Node"
    return
}

Write-Host "[g8e] Node Binary downloaded to ${NodeBinaryName}"

# Run enrollment
Write-Host "[g8e] Running PKI enrollment with endpoint ${GatewayHost}..."
& .\$NodeBinaryName security pki enroll --endpoint "${GatewayHost}"

if ($LASTEXITCODE -ne 0) {
    Write-Host "[g8e] ERROR: Enrollment failed with exit code ${LASTEXITCODE}"
    return
}

Write-Host "[g8e] Enrollment complete"

# Start the operator
Write-Host "[g8e] Starting Operator with endpoint ${GatewayHost}..."
Write-Host "[g8e] The Operator will run in this terminal. Press Ctrl+C to stop."
& .\$NodeBinaryName -e $GatewayHost
`, port, caBundleURL, localCAPath, binaryName, binaryURL)

	w.Header().Set(constants.HeaderContentType, constants.HeaderValuePowerShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
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
		c.responder.Error(w, http.StatusBadRequest, "pki: invalid filename")
		return
	}

	// Validate binary name pattern for security
	binaryPattern := regexp.MustCompile(`^g8e-(linux|darwin|windows)-(amd64|arm64|386)(\.exe)?$`)
	if !binaryPattern.MatchString(filename) {
		c.responder.Error(w, http.StatusBadRequest, "pki: invalid binary name")
		return
	}

	possiblePaths := []string{}

	// Check relative to executable
	if execPath, err := os.Executable(); err == nil {
		possiblePaths = append(possiblePaths, filepath.Join(filepath.Dir(execPath), "bin", filename))
	}

	// Check in PKI binaries directory
	possiblePaths = append(possiblePaths, filepath.Join(c.pki.PKIDir(), constants.PkiSubdirBinaries, filename))

	var binaryPath string
	for _, path := range possiblePaths {
		fileInfo, err := os.Stat(path)
		if err == nil && !fileInfo.IsDir() {
			binaryPath = path
			break
		}
	}

	if binaryPath == "" {
		c.logger.Error("Binary not found", "filename", filename, "checked_paths", possiblePaths)
		c.responder.Error(w, http.StatusNotFound, fmt.Sprintf("pki: binary not found: %s", filename))
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueOctetStream)
	w.Header().Set(constants.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
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
		GatewayPort: strconv.Itoa(constants.Ports.OperatorHttp),
	}

	script, err := scripts.RenderLinuxDeployScript(data)
	if err != nil {
		c.logger.Error("Failed to render Linux deploy script", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: render Linux deploy script: %w", err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) handleDeployScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Log what we're getting
	c.logger.Info("Deploy script request", "r.Host", r.Host, "RemoteAddr", r.RemoteAddr)

	// Try to get the host from the Host header or X-Forwarded-Host
	gatewayHost := r.Header.Get("X-Forwarded-Host")
	if gatewayHost == "" {
		gatewayHost = r.Host
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(gatewayHost); err == nil {
		gatewayHost = h
	}

	// If we still have localhost or empty, try to use the server's own IP address
	// but only if it's not a loopback request.
	if gatewayHost == "" || gatewayHost == "localhost" || gatewayHost == "127.0.0.1" || gatewayHost == "::1" {
		// If it's a loopback request, we might still want to use the actual IP if possible
		// so other nodes can use the same script.
		if ip, _, err := net.SplitHostPort(r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()); err == nil {
			if ip != "" && ip != "127.0.0.1" && ip != "::1" && ip != "0.0.0.0" {
				gatewayHost = ip
			}
		}
	}

	// Final fallback to constants if we really can't find anything
	if gatewayHost == "" {
		gatewayHost = constants.DefaultEndpoint
	}

	c.logger.Info("Using gateway host for script", "gatewayHost", gatewayHost)

	data := scripts.TemplateData{
		GatewayHost: gatewayHost,
		GatewayPort: strconv.Itoa(constants.Ports.OperatorHttp),
	}

	script, err := scripts.RenderWindowsDeployScript(data)
	if err != nil {
		c.logger.Error("Failed to render Windows deploy script", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("pki: render Windows deploy script: %w", err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValuePowerShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func (c *PKIController) extractGatewayHost(host string) string {
	if host == "" {
		return constants.DefaultEndpoint
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

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
	"crypto/x509"
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
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
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

// @Summary		Get CA bundle
// @Description	Returns the platform's root CA certificate bundle for trust establishment
// @Tags			pki
// @Accept			json
// @Produce		application/x-pem-file
// @Success		200	{string}	string
// @Router			/.well-known/g8e/pki/ca-bundle [get]
func (c *PKIController) handlePKICABundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	bundlePath := c.pki.TrustBundlePath()
	c.logger.Debug("Reading gateway trust bundle", "path", bundlePath)

	pemData, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Error("Failed to read gateway trust bundle", "error", err, "path", bundlePath)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrTrustBundleStale, err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValuePEM)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pemData)
}

// @Summary		Get PKI fingerprint
// @Description	Returns the platform's PKI fingerprint for verification
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]string
// @Router			/.well-known/g8e/pki/fingerprint [get]
func (c *PKIController) handlePKIFingerprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	rootCAPath := c.pki.RootCAPath()
	pemData, err := os.ReadFile(rootCAPath)
	if err != nil {
		c.logger.Error("Failed to read root CA", "error", err, "path", rootCAPath)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrPKILoadRootCA, err).Error())
		return
	}

	block, rest := pem.Decode(pemData)
	if block == nil {
		c.logger.Error("Invalid root CA PEM", "path", rootCAPath)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrPEMDecodeFailed.Error())
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

// @Summary		Sign CSR
// @Description	Signs a certificate signing request (internal mTLS endpoint)
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.PKICSRSignResponse
// @Router			/api/v1/pki/csr/sign [post]
func (c *PKIController) handlePKICSRSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
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
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	certPEM, chainPEM, err := c.pki.SignCSR(req.CSR, req.LeafType, req.OrganizationID, req.OperatorID, req.UserID, req.WorkloadSessionID, "")
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrPKISignCSR, err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PKICSRSignResponse{
		CertificatePEM:      certPEM,
		CertificateChainPEM: chainPEM,
	})
}

// @Summary		Revoke certificate
// @Description	Revokes a certificate by serial number (internal mTLS endpoint)
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.StatusResponse
// @Router			/api/v1/pki/certificates/revoke [post]
func (c *PKIController) handlePKICertificatesRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	var req struct {
		Serial string `json:"serial"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	if req.Serial == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}

	if err := c.pki.RevokeCertificate(req.Serial, req.Reason); err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrPKIRevokeCertificate, err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

// @Summary		Get CRL
// @Description	Returns the certificate revocation list
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]string
// @Router			/.well-known/g8e/pki/crl [get]
func (c *PKIController) handlePKIRevocationBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	crlDER, err := c.pki.GenerateCRL()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrPKIGenerateCRL, err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueCRL)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(crlDER)
}

// @Summary		PKI device enrollment
// @Description	Enrolls a device via PKI endpoint
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]string
// @Router			/api/v1/pki/devices/enroll [post]
func (c *PKIController) handlePKIDevicesEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	if c.registration == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
		return
	}

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrMissingCertificate.Error())
		return
	}

	userID, err := ExtractUserIDFromCert(r.TLS.PeerCertificates[0])
	if err != nil {
		c.responder.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: %v", constants.ErrCertParseFailed, err).Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	var req models.OperatorRegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	// Device enrollment does not require an organization context
	organizationID := ""
	if req.CSR == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}

	resp, err := c.registration.RegisterDeviceCSR(userID, organizationID, req)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrEnrollmentFailed, err).Error())
		return
	}

	c.responder.JSON(w, http.StatusCreated, resp)
}

// @Summary		PKI app enrollment
// @Description	Enrolls an external app via PKI endpoint (identity-only enrollment)
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	AppEnrollResponse
// @Router			/api/v1/pki/apps/enroll [post]
func (c *PKIController) handlePKIAppsEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	if c.appEnrollment == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	var req AppEnrollRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	resp, err := c.appEnrollment.EnrollApp(req)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrEnrollmentFailed, err).Error())
		return
	}

	if !resp.Success {
		c.responder.Error(w, http.StatusBadRequest, resp.Error)
		return
	}

	c.responder.JSON(w, http.StatusCreated, resp)
}

// @Summary		Mint delegated app credential
// @Description	Mints a short-lived delegated credential for an app, binding both app and requestor identities (mTLS-authenticated)
// @Tags			pki
// @Accept			json
// @Produce		json
// @Success		200	{object}	AppEnrollResponse
// @Router			/api/v1/pki/apps/delegated [post]
func (c *PKIController) handlePKIAppsDelegated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	// Require mTLS authentication from a human CLI session
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrMissingCertificate.Error())
		return
	}

	// Extract user ID from the CLI certificate
	userID, err := ExtractUserIDFromCert(r.TLS.PeerCertificates[0])
	if err != nil {
		c.responder.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: %v", constants.ErrCertParseFailed, err).Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	var req AppEnrollRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", constants.ErrInvalidJSONBody, err).Error())
		return
	}

	// Validate request
	if req.CSR == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}
	if req.AppName == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}

	// Sanitize app name
	sanitizedName := req.AppName
	if !isValidAppName(sanitizedName) {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrValidationFailed.Error())
		return
	}

	// Validate CSR format
	block, _ := pem.Decode([]byte(req.CSR))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPKIInvalidCSR.Error())
		return
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPKIParseCSR.Error())
		return
	}

	if err := csr.CheckSignature(); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPKICSRSignatureCheck.Error())
		return
	}

	// Sign the CSR with dual SANs (app + requestor)
	// Use a short TTL (1 hour) for delegated credentials
	certPEM, chainPEM, err := c.pki.SignDelegatedCSR(req.CSR, sanitizedName, userID)
	if err != nil {
		c.logger.Error("Failed to sign delegated CSR", "app_name", sanitizedName, "user_id", userID, "error", err)
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrPKISignCSR.Error())
		return
	}

	// Extract the appID from the signed certificate
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrPEMDecodeFailed.Error())
		return
	}
	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrCertParseFailed, err).Error())
		return
	}
	if len(parsedCert.URIs) == 0 {
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrValidationFailed.Error())
		return
	}
	appID := parsedCert.URIs[0].String()

	// Fetch trust bundle
	trustBundle, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Error("Failed to fetch trust bundle", "error", err)
		trustBundle = []byte{}
	}

	c.logger.Info("[DELEGATED_CREDENTIAL] Minted delegated credential",
		"app_id", appID,
		"app_name", sanitizedName,
		"user_id", userID,
		"expires_at", parsedCert.NotAfter.UTC().Format(time.RFC3339))

	c.responder.JSON(w, http.StatusCreated, AppEnrollResponse{
		Success:     true,
		AppCert:     certPEM,
		CertChain:   chainPEM,
		TrustBundle: string(trustBundle),
		AppID:       appID,
		ExpiresAt:   parsedCert.NotAfter.UTC().Format(time.RFC3339),
	})
}

// @Summary		Bootstrap CA Linux script
// @Description	Returns the Linux CA trust bootstrap script
// @Tags			bootstrap
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/bootstrap-ca [get]
func (c *PKIController) handleTrustScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	// Extract the gateway host from the request
	gatewayHost := r.Header.Get("X-Forwarded-Host")
	if gatewayHost == "" {
		gatewayHost = r.Host
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(gatewayHost); err == nil {
		gatewayHost = h
	}

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	caBundleURL := constants.APIPaths.WellKnownPKICABundle
	localCAPath := filepath.ToSlash(paths.Infra.CaCertPath)

	script := fmt.Sprintf(`#!/bin/sh
set -e

GATEWAY_HOST="%s"
GATEWAY_PORT="%s"
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
echo "[g8e] IMPORTANT: Please restart all open browsers for changes to take effect."
echo "[g8e] You can now use: ./g8e -e ${GATEWAY_HOST} auth enroll"
`, gatewayHost, port, caBundleURL, localCAPath)

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// @Summary		Bootstrap CA macOS script
// @Description	Returns the macOS CA trust bootstrap script
// @Tags			bootstrap
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/bootstrap-ca-macos [get]
func (c *PKIController) handleTrustScriptMacos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	// Extract the gateway host from the request
	gatewayHost := r.Header.Get("X-Forwarded-Host")
	if gatewayHost == "" {
		gatewayHost = r.Host
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(gatewayHost); err == nil {
		gatewayHost = h
	}

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	caBundleURL := constants.APIPaths.WellKnownPKICABundle

	script := fmt.Sprintf(`#!/bin/sh
set -e

GATEWAY_HOST="%s"
GATEWAY_PORT="%s"
CA_BUNDLE_URL="http://${GATEWAY_HOST}:${GATEWAY_PORT}%s"
TEMP_CA_PATH="/tmp/g8e-ca.crt"

echo "[g8e] Fetching platform CA bundle from ${CA_BUNDLE_URL}..."
curl -fsSL "${CA_BUNDLE_URL}" -o "${TEMP_CA_PATH}"

if [ ! -f "${TEMP_CA_PATH}" ]; then
    echo "[g8e] ERROR: Failed to download CA bundle"
    exit 1
fi

echo "[g8e] Installing CA bundle to macOS Keychain..."
echo "[g8e] Sudo password may be required to trust the certificate."
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "${TEMP_CA_PATH}"

echo "[g8e] CA bundle installed and trusted."
echo "[g8e] IMPORTANT: Please restart all open browsers for changes to take effect."
echo "[g8e] You can now use: ./g8e -e ${GATEWAY_HOST} auth enroll"
`, gatewayHost, port, caBundleURL)

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// @Summary		Bootstrap CA Windows script
// @Description	Returns the Windows CA trust bootstrap script
// @Tags			bootstrap
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/bootstrap-ca.ps1 [get]
func (c *PKIController) handleTrustScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	// Extract the gateway host from the request
	gatewayHost := r.Header.Get("X-Forwarded-Host")
	if gatewayHost == "" {
		gatewayHost = r.Host
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(gatewayHost); err == nil {
		gatewayHost = h
	}

	// Substitute loopback with actual server IP so the script works on other nodes
	if gatewayHost == "" || gatewayHost == "localhost" || gatewayHost == "127.0.0.1" || gatewayHost == "::1" {
		if localAddr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && localAddr != nil {
			if ip, _, err := net.SplitHostPort(localAddr.String()); err == nil {
				if ip != "" && ip != "127.0.0.1" && ip != "::1" && ip != "0.0.0.0" {
					gatewayHost = ip
				}
			}
		}
	}

	port := strconv.Itoa(constants.Ports.OperatorHttp)
	caBundleURL := constants.APIPaths.WellKnownPKICABundle
	localCAPath := filepath.ToSlash(paths.Infra.CaCertPath)
	binaryName := constants.BinaryNameWindows
	binaryURL := constants.APIPaths.WellKnownBinPrefix + binaryName

	script := fmt.Sprintf(`$ErrorActionPreference = "Continue"

$GatewayHost = "%s"
$GatewayPort = "%s"
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

Write-Host "[g8e] IMPORTANT: Please restart all open browsers for changes to take effect."
Write-Host "[g8e] You can now run: .\\${NodeBinaryName} -e ${GatewayHost} auth enroll"
`, gatewayHost, port, caBundleURL, localCAPath, binaryName, binaryURL)

	w.Header().Set(constants.HeaderContentType, constants.HeaderValuePowerShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// @Summary		Bootstrap CA Windows script (alias)
// @Description	Returns the Windows CA trust bootstrap script (alias endpoint)
// @Tags			bootstrap
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/.well-known/g8e/pki/trust-windows [get]
func (c *PKIController) handleTrustScriptWindowsAlias(w http.ResponseWriter, r *http.Request) {
	c.handleTrustScriptWindows(w, r)
}

// @Summary		Download node binary
// @Description	Downloads the g8e node binary for the current platform (internal endpoint)
// @Tags			bootstrap
// @Produce		application/octet-stream
// @Success		200	{file}	file
// @Router			/.well-known/g8e/bin/{filename} [get]
func (c *PKIController) handleNodeBinaryDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	filename := filepath.Base(r.URL.Path)
	if filename == "" || filename == "." {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPathValidation.Error())
		return
	}

	// Validate binary name pattern for security
	binaryPattern := regexp.MustCompile(`^g8e-(linux|darwin|windows)-(amd64|arm64|386)(\.exe)?$`)
	if !binaryPattern.MatchString(filename) {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPathValidation.Error())
		return
	}

	possiblePaths := []string{}

	// Check relative to executable
	if execPath, err := os.Executable(); err == nil {
		possiblePaths = append(possiblePaths, filepath.Join(filepath.Dir(execPath), constants.BinDirname, filename))
	}

	// Check in PKI binaries directory
	possiblePaths = append(possiblePaths, filepath.Join(c.pki.BinariesDir(), filename))

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
		c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueOctetStream)
	w.Header().Set(constants.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	http.ServeFile(w, r, binaryPath)
}

// @Summary		Deploy script Linux
// @Description	Returns the Linux operator deployment script
// @Tags			deploy
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/g8e-operator.sh [get]
func (c *PKIController) handleDeployScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
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
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrInternal, err).Error())
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueShell)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.Header().Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// @Summary		Deploy script Windows
// @Description	Returns the Windows operator deployment script
// @Tags			deploy
// @Produce		text/plain
// @Success		200	{string}	string
// @Router			/g8e-operator.ps1 [get]
func (c *PKIController) handleDeployScriptWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
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
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrInternal, err).Error())
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

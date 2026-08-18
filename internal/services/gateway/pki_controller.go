// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
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
	pki           *PKIAuthority
	appEnrollment *AppEnrollmentService
	registration  *RegistrationService
	responder     *response.Writer
}

// PKIControllerDeps groups all dependencies for PKIController.
type PKIControllerDeps struct {
	Cfg           *config.Config
	Logger        *slog.Logger
	PKI           *PKIAuthority
	AppEnrollment *AppEnrollmentService
	Registration  *RegistrationService
	Responder     *response.Writer
}

func newPKIController(d PKIControllerDeps) *PKIController {
	return &PKIController{
		cfg:           d.Cfg,
		logger:        d.Logger,
		pki:           d.PKI,
		appEnrollment: d.AppEnrollment,
		registration:  d.Registration,
		responder:     d.Responder,
	}
}

func (c *PKIController) gatewayHTTPPort() string {
	if c.cfg != nil && c.cfg.Gateway.HTTPPort != 0 {
		return strconv.Itoa(c.cfg.Gateway.HTTPPort)
	}
	return strconv.Itoa(constants.Ports.OperatorHttp)
}

func (c *PKIController) readBody(r *http.Request) ([]byte, error) {
	return readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
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

	pemData, err := c.pki.RootCAPEM()
	if err != nil {
		c.logger.Error("Failed to read root CA", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("%w: %v", constants.ErrPKILoadRootCA, err).Error())
		return
	}

	block, rest := pem.Decode(pemData)
	if block == nil {
		c.logger.Error("Invalid root CA PEM")
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

	if c.appEnrollment == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrServiceUnavailable.Error())
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

	resp, err := c.appEnrollment.EnrollDelegatedApp(req, userID)
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

// @Summary		Download node binary
// @Description	Downloads the g8e binary file binary for the current platform (internal endpoint)
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
		c.responder.Error(w, http.StatusNotFound, fmt.Sprintf("binary %q not found on the Gateway. Run 'make build-all' on the Gateway host to build all platform binaries, then restart the Gateway.", filename))
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
// @Router			/g8e-deploy.sh [get]
func (c *PKIController) handleDeployScriptLinux(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	gatewayHost := c.extractGatewayHost(r.Host)
	data := scripts.TemplateData{
		GatewayHost: gatewayHost,
		GatewayPort: c.gatewayHTTPPort(),
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
// @Router			/g8e-deploy.ps1 [get]
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
		GatewayPort: c.gatewayHTTPPort(),
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

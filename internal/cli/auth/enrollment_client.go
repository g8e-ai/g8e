// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// EnrollmentClient is the gateway enrollment transport. It performs ONLY
// HTTP I/O and response validation: it receives a context.Context, returns
// typed EnrollmentArtifacts, and writes no files, performs no UI/platform
// work, and never opens a browser. Per §4.1, this replaces the transport
// slices of BootstrapWithURL/CLIEnroll/ReEnroll/EnrollWithGateway with one
// typed client surface per operation.
//
// All methods centralize:
//   - HTTP status + JSON parsing
//   - Success/structured-error handling
//   - Required session/user/certificate field validation
//   - PEM/cert validity and cert/key public-key matching
//   - Full trust-bundle parse and root-anchor discovery
//   - Fingerprint pin verification when the caller supplies a pin
//
// The client is safe for concurrent use (it holds no mutable state).
type EnrollmentClient struct {
	httpClient        *http.Client
	cfg               *config.Config
	systemFingerprint func() (string, error)
}

// NewEnrollmentClient returns an enrollment client using a plain HTTP
// client (no mTLS) for bootstrap, recovery, and remote operator
// enrollment. The CLI rotation method builds its own mTLS client from the
// local identity via BuildMTLSClient.
//
// systemFingerprint, when nil, defaults to the production fingerprint
// generator. It is injectable for tests.
func NewEnrollmentClient(cfg *config.Config, systemFingerprint func() (string, error)) *EnrollmentClient {
	if systemFingerprint == nil {
		systemFingerprint = defaultSystemFingerprint
	}
	return &EnrollmentClient{
		httpClient:        &http.Client{Timeout: httpTimeout, Transport: httpclient.NewIPv4Transport(nil)},
		cfg:               cfg,
		systemFingerprint: systemFingerprint,
	}
}

// defaultSystemFingerprint generates the host system fingerprint used by
// bootstrap/recovery/remote enrollment to stamp the request with host
// metadata. Extracted here so tests can inject a no-op fingerprint.
func defaultSystemFingerprint() (string, error) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	return fp.Fingerprint, nil
}

// localOSUser returns the current OS user metadata for gateway requests.
// Reuses the package-level getLocalOSUser helper.

// Bootstrap performs the initial unbootstrapped-gateway CLI enrollment
// (POST /api/v1/auth/bootstrap over the discovery/plain-HTTP surface).
// The caller supplies the CLI CSR (and its private key, for staging) and
// an optional operator CSR. The gateway returns the first user/session
// and the full runtime trust bundle.
//
// baseURL, when non-empty, overrides the discovery URL (used by demos and
// tests). caFingerprint, when non-empty, pins the expected root CA
// fingerprint.
func (c *EnrollmentClient) Bootstrap(ctx context.Context, cliCSR string, cliKey *ecdsa.PrivateKey, operatorCSR, caFingerprint, baseURL string) (EnrollmentArtifacts, error) {
	systemFp, err := c.systemFingerprint()
	if err != nil {
		return EnrollmentArtifacts{}, err
	}

	req := models.BootstrapRequest{
		CSR:               operatorCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp,
		LocalOSUser:       getLocalOSUser(),
	}

	discoveryURL := c.cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}

	var resp models.BootstrapResponse
	if err := postJSON(ctx, c.httpClient, discoveryURL+constants.APIPaths.AuthBootstrap, req, &resp); err != nil {
		return EnrollmentArtifacts{}, err
	}
	if !resp.Success {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: bootstrap unsuccessful", constants.ErrEnrollmentFailed)
	}

	artifacts := EnrollmentArtifacts{
		Source:               EnrollmentSourceBootstrap,
		CLISessionID:         resp.CLISessionID,
		UserID:               resp.UserID,
		OperatorSessionID:    resp.OperatorSessionID,
		OperatorID:           resp.OperatorID,
		CLICertPEM:           resp.CLICert,
		CLICertChainPEM:      resp.CLICertChain,
		CLIKey:               cliKey,
		TrustBundlePEM:       resp.HubTrustBundle,
		OperatorCertPEM:      resp.OperatorCert,
		OperatorCertChainPEM: resp.OperatorCertChain,
	}

	if err := validateLocalCLI(artifacts, caFingerprint); err != nil {
		return EnrollmentArtifacts{}, err
	}
	return artifacts, nil
}

// CreateRecoveryRequest posts a CLI CSR to the recovery request endpoint
// (POST /api/v1/auth/cli/recovery/request over the discovery surface) and
// returns the opaque one-time token, the request ID, the browser approval
// URL, and the request expiry. No certificate is issued yet — the caller
// must poll CompleteRecovery after the user approves in the console.
//
// baseURL, when non-empty, overrides the discovery URL.
func (c *EnrollmentClient) CreateRecoveryRequest(ctx context.Context, cliCSR, baseURL string) (requestID, token, approvalURL string, expiresAt time.Time, err error) {
	systemFp, ferr := c.systemFingerprint()
	if ferr != nil {
		return "", "", "", time.Time{}, ferr
	}

	req := models.CLIRecoveryRequestRequest{
		CLICSRPEM:         cliCSR,
		SystemFingerprint: systemFp,
		LocalOSUser:       getLocalOSUser(),
	}

	discoveryURL := c.cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}

	var resp models.CLIRecoveryRequestResponse
	if err = postJSON(ctx, c.httpClient, discoveryURL+constants.APIPaths.AuthCLIRecoveryRequest, req, &resp); err != nil {
		return "", "", "", time.Time{}, err
	}
	if !resp.Success {
		return "", "", "", time.Time{}, fmt.Errorf("%w: recovery request unsuccessful", constants.ErrEnrollmentFailed)
	}
	if resp.Token == "" || resp.RequestID == "" {
		return "", "", "", time.Time{}, constants.ErrMissingRequiredField
	}
	return resp.RequestID, resp.Token, resp.ApprovalURL, resp.ExpiresAt, nil
}

// RecoveryStatus polls the recovery request state
// (GET /api/v1/auth/cli/recovery/status?token=...). Returns the current
// lifecycle state. Used by the coordinator's bounded-backoff poll loop.
func (c *EnrollmentClient) RecoveryStatus(ctx context.Context, token, baseURL string) (models.CLIRecoveryState, error) {
	discoveryURL := c.cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}

	url := discoveryURL + constants.APIPaths.AuthCLIRecoveryStatus + "?token=" + token
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}
	var status models.CLIRecoveryStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return status.State, nil
}

// CompleteRecovery posts the proof-of-possession signature to the recovery
// completion endpoint (POST /api/v1/auth/cli/recovery/complete) and
// returns the issued CLI identity. The caller must have generated the CSR
// and signed the request ID with the CSR private key.
//
// The signature is over the requestID bytes (UTF-8), base64-encoded. The
// caller's cliKey is staged into the returned artifacts so CredentialStore
// can commit it.
//
// baseURL, when non-empty, overrides the discovery URL.
func (c *EnrollmentClient) CompleteRecovery(ctx context.Context, requestID, token string, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint, baseURL string) (EnrollmentArtifacts, error) {
	sig, err := signRecoveryProof(cliKey, requestID)
	if err != nil {
		return EnrollmentArtifacts{}, err
	}

	req := models.CLIRecoveryCompleteRequest{
		Token:     token,
		Signature: sig,
	}

	discoveryURL := c.cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}

	var resp models.CLIRecoveryCompleteResponse
	if err := postJSON(ctx, c.httpClient, discoveryURL+constants.APIPaths.AuthCLIRecoveryComplete, req, &resp); err != nil {
		return EnrollmentArtifacts{}, err
	}
	if !resp.Success {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: recovery completion unsuccessful", constants.ErrEnrollmentFailed)
	}

	artifacts := EnrollmentArtifacts{
		Source:            EnrollmentSourceRecovery,
		CLISessionID:      resp.CLISessionID,
		UserID:            resp.UserID,
		OperatorSessionID: resp.OperatorSessionID,
		OperatorID:        resp.OperatorID,
		CLICertPEM:        resp.CLICert,
		CLICertChainPEM:   resp.CLICertChain,
		CLIKey:            cliKey,
		TrustBundlePEM:    resp.HubTrustBundle,
	}
	if err := validateLocalCLI(artifacts, caFingerprint); err != nil {
		return EnrollmentArtifacts{}, err
	}
	return artifacts, nil
}

// Rotate performs the mTLS CLI rotation
// (POST /api/v1/auth/cli/rotate over the public HTTPS surface). The
// caller's existing CLI identity is used to build the mTLS client; the
// gateway derives the user and CLI session from the authenticated
// certificate context. The request body carries only the new CLI CSR.
//
// The caller supplies the new CLI CSR and its private key for staging.
// caFingerprint, when non-empty, pins the expected root CA fingerprint.
func (c *EnrollmentClient) Rotate(ctx context.Context, fileSvc fs.RuntimeFileService, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint string) (EnrollmentArtifacts, error) {
	mtlsClient, err := BuildMTLSClient(fileSvc, c.cfg, httpTimeout)
	if err != nil {
		return EnrollmentArtifacts{}, err
	}

	req := models.CLIRotationRequest{CLICSRPEM: cliCSR}
	publicURL := c.cfg.OperatorPublicURL()

	store := NewCredentialStore(fileSvc, c.cfg)
	creds, _ := store.LoadCredentials(ctx)
	headers := map[string]string{}
	if creds != nil && creds.CLISessionID != "" {
		headers[constants.HeaderCLISessionID] = creds.CLISessionID
	}

	var resp models.CLIRotationResponse
	if err := postJSON(ctx, mtlsClient, publicURL+constants.APIPaths.AuthCLIRotate, req, &resp, headers); err != nil {
		return EnrollmentArtifacts{}, err
	}
	if !resp.Success {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: rotation unsuccessful", constants.ErrEnrollmentFailed)
	}

	artifacts := EnrollmentArtifacts{
		Source:          EnrollmentSourceRotation,
		CLISessionID:    resp.CLISessionID,
		UserID:          resp.UserID,
		CLICertPEM:      resp.CLICert,
		CLICertChainPEM: resp.CLICertChain,
		CLIKey:          cliKey,
		TrustBundlePEM:  resp.HubTrustBundle,
	}
	if err := validateLocalCLI(artifacts, caFingerprint); err != nil {
		return EnrollmentArtifacts{}, err
	}
	return artifacts, nil
}

// Refresh performs the mTLS CLI session refresh
// (POST /api/v1/auth/cli/refresh over the public HTTPS surface). The
// caller's existing CLI cert is used to build the mTLS client; the
// gateway derives the user from the authenticated certificate context.
// The request body is empty — the cert is the proof of identity. A new
// CLI session is issued bound to the same user, with a fresh TTL. The
// cert is NOT rotated.
//
// This is the recovery path for an expired CLI session with a still-valid
// cert. The caller must hold a valid (non-expired) cert; an expired cert
// cannot authenticate via mTLS and must use the recovery flow instead.
func (c *EnrollmentClient) Refresh(ctx context.Context, fileSvc fs.RuntimeFileService) (CLISessionRefresh, error) {
	mtlsClient, err := BuildMTLSClient(fileSvc, c.cfg, httpTimeout)
	if err != nil {
		return CLISessionRefresh{}, err
	}

	publicURL := c.cfg.OperatorPublicURL()

	store := NewCredentialStore(fileSvc, c.cfg)
	creds, _ := store.LoadCredentials(ctx)
	headers := map[string]string{}
	if creds != nil && creds.CLISessionID != "" {
		headers[constants.HeaderCLISessionID] = creds.CLISessionID
	}

	var resp models.CLIRefreshResponse
	if err := postJSON(ctx, mtlsClient, publicURL+constants.APIPaths.AuthCLIRefresh, models.CLIRefreshRequest{}, &resp, headers); err != nil {
		return CLISessionRefresh{}, err
	}
	if !resp.Success {
		return CLISessionRefresh{}, fmt.Errorf("%w: refresh unsuccessful", constants.ErrCLIRefreshFailed)
	}
	if resp.CLISessionID == "" || resp.UserID == "" {
		return CLISessionRefresh{}, constants.ErrMissingRequiredField
	}
	return CLISessionRefresh{
		CLISessionID: resp.CLISessionID,
		UserID:       resp.UserID,
	}, nil
}

// CLISessionRefresh is the result of a successful CLI session refresh.
type CLISessionRefresh struct {
	CLISessionID string
	UserID       string
}

// ProbeCLISession issues a lightweight authenticated mTLS GET to
// ApprovalsCLIList (/api/v1/approvals/pending) to determine whether the
// local CLI session is still valid server-side. The coordinator calls
// this on the complete-reuse path to detect an expired or invalidated
// session before printing "Reusing existing CLI identity".
//
// Returns nil when the session is healthy (HTTP 200). Returns
// constants.ErrCLISessionExpired or constants.ErrCLISessionInvalid when
// the gateway rejects the session with 401. Returns a wrapped
// constants.ErrHTTPRequestExecuteFailed on network failure.
func (c *EnrollmentClient) ProbeCLISession(ctx context.Context, fileSvc fs.RuntimeFileService) error {
	mtlsClient, err := BuildMTLSClient(fileSvc, c.cfg, httpTimeout)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}

	publicURL := c.cfg.OperatorPublicURL()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL+constants.APIPaths.ApprovalsCLIList, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	store := NewCredentialStore(fileSvc, c.cfg)
	creds, _ := store.LoadCredentials(ctx)
	if creds != nil && creds.CLISessionID != "" {
		httpReq.Header.Set(constants.HeaderCLISessionID, creds.CLISessionID)
	}

	resp, err := mtlsClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, readErr)
		}
		body := string(respBytes)
		if strings.Contains(body, constants.ErrCLISessionExpired.Error()) {
			return constants.ErrCLISessionExpired
		}
		if strings.Contains(body, constants.ErrCLISessionInvalid.Error()) {
			return constants.ErrCLISessionInvalid
		}
		return constants.ErrCLISessionInvalid
	}
	return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
}

// CheckBootstrapStatus reports whether the gateway has been bootstrapped
// (started and at least one owner user exists). Used by the coordinator to
// choose between bootstrap and recovery for an absent local identity.
//
// baseURL, when non-empty, overrides the discovery URL.
func (c *EnrollmentClient) CheckBootstrapStatus(ctx context.Context, baseURL string) (bool, error) {
	discoveryURL := c.cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}

	url := discoveryURL + constants.APIPaths.AuthBootstrapStatus
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}
	var status models.BootstrapStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return status.Bootstrapped, nil
}

// DiscoverGatewayCA fetches the live gateway root CA bundle from the
// unauthenticated discovery surface (plain-HTTP
// /.well-known/g8e/pki/ca-bundle). It returns the raw PEM bundle and the
// SHA-256 fingerprint of the primary root anchor (the first self-signed CA
// in the bundle).
//
// The call is best-effort: a network failure returns a non-nil err and
// empty bundle/fingerprint. The coordinator decides whether to abort. No
// fingerprint pin is applied — the live bundle IS the source of truth for
// the pin, so pinning against the local bundle would be circular.
//
// See EnrollmentGateway.DiscoverGatewayCA for the contract.
func (c *EnrollmentClient) DiscoverGatewayCA(ctx context.Context) ([]byte, string, error) {
	discoveryURL := c.cfg.OperatorDiscoveryURL()
	caURL := discoveryURL + constants.APIPaths.WellKnownPKICABundle

	// Use the IPv4-only transport so `localhost` resolves to 127.0.0.1 on
	// Windows (where the OS resolver returns ::1 first and the IDE's
	// port-forward only listens on IPv4). The discovery surface is plain
	// HTTP, so no TLS config is needed.
	bundlePEM, err := certs.FetchTrustBundleWithClient(ctx, caURL, "", &http.Client{
		Timeout:   15 * time.Second,
		Transport: httpclient.NewIPv4Transport(nil),
	})
	if err != nil {
		return nil, "", err
	}

	roots, err := platform.ExtractRootAnchors(bundlePEM, time.Now)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", constants.ErrSystemTrustInvalidAnchor, err)
	}
	if len(roots) == 0 {
		return nil, "", constants.ErrSystemTrustInvalidAnchor
	}
	return bundlePEM, platform.CertFingerprint(roots[0]), nil
}

// postJSON is the centralized HTTP POST + JSON decode + status check
// helper used by all enrollment transport methods. It enforces the
// §4.2 centralized response validation rules: HTTP status, JSON parsing,
// and (via the caller's validateLocalCLI) required fields and cert/key
// matching.
func postJSON(ctx context.Context, httpClient *http.Client, url string, reqBody any, respBody any, headers ...map[string]string) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
	for _, h := range headers {
		for k, v := range h {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}
	if err := json.Unmarshal(respBytes, respBody); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return nil
}

// validateLocalCLI centralizes the post-response validation for local CLI
// enrollment artifacts (bootstrap, recovery, rotation). It checks:
//   - required session/user/cert fields
//   - trust bundle parses and root anchors are discoverable
//   - cert/key public-key matching (when key is present)
//   - fingerprint pin (when caFingerprint is non-empty)
func validateLocalCLI(a EnrollmentArtifacts, caFingerprint string) error {
	if a.CLISessionID == "" || a.UserID == "" || a.CLICertPEM == "" {
		return constants.ErrMissingRequiredField
	}
	if a.TrustBundlePEM == "" {
		return constants.ErrEmptyTrustBundle
	}
	bundle, err := ParseTrustBundle([]byte(a.TrustBundlePEM), time.Now())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
	}
	if err := bundle.VerifyFingerprintPin(caFingerprint); err != nil {
		return err
	}
	if a.CLIKey != nil {
		cert, err := parseCertPEMBytes([]byte(a.CLICertPEM))
		if err != nil {
			return fmt.Errorf("%w: staged CLI cert: %w", constants.ErrValidationFailed, err)
		}
		if !pubKeyMatchesCert(cert, &a.CLIKey.PublicKey) {
			return fmt.Errorf("%w: CLI cert/key mismatch", constants.ErrValidationFailed)
		}
	}
	return nil
}

// signRecoveryProof signs the requestID bytes with the CSR private key
// and returns the base64-encoded signature. Used by CompleteRecovery.
func signRecoveryProof(key *ecdsa.PrivateKey, requestID string) (string, error) {
	if key == nil || requestID == "" {
		return "", constants.ErrCLIRecoveryProofInvalid
	}
	sig, err := ecdsa.SignASN1(randReader(), key, []byte(requestID))
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrCLIRecoveryProofInvalid, err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// parseCertPEMBytes parses a single PEM-encoded certificate. Local helper
// used by validateLocalCLI (the certutil version requires the whole file).
func parseCertPEMBytes(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, constants.ErrPEMDecodeFailed
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: type=%s", constants.ErrInvalidPEMType, block.Type)
	}
	return x509.ParseCertificate(block.Bytes)
}

// osHostname wraps os.Hostname for testability.
func osHostname() (string, error) {
	return os.Hostname()
}

// randReader returns the default crypto/rand reader for signature
// generation. Indirectable in tests by replacing signRecoveryProof's
// caller rather than this package-level var.
func randReader() interface {
	Read(b []byte) (n int, err error)
} {
	return rand.Reader
}

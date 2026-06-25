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

package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/auth"
)

// httpTimeout is the default timeout for all HTTP clients in the auth package.
const httpTimeout = 30 * time.Second

type RegistrationRequest struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
	CSRPEM            string `json:"csr_pem"`
	CLICSRPEM         string `json:"cli_csr_pem"`
}

type RegistrationResponse struct {
	Success           bool   `json:"success"`
	OperatorSessionID string `json:"operator_session_id"`
	CLISessionID      string `json:"cli_session_id"`
	OperatorID        string `json:"operator_id"`
	OperatorCert      string `json:"operator_cert"`
	OperatorCertChain string `json:"operator_cert_chain,omitempty"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain,omitempty"`
	HubTrustBundle    string `json:"hub_trust_bundle,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	Error             string `json:"error,omitempty"`
}

type Credentials struct {
	OperatorSessionID string `json:"operator_session_id"`
	UserID            string `json:"user_id"`
	OperatorID        string `json:"operator_id"`
	CLISessionID      string `json:"cli_session_id"`
}

// webauthnClientData is the client data JSON for WebAuthn ceremonies.
type webauthnClientData struct {
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
	Type      string `json:"type"`
}

// cliAssertionVerifyRequest is the typed request for CLI passkey authentication verification.
type cliAssertionVerifyRequest struct {
	UserID            string                       `json:"user_id"`
	AssertionResponse models.WebAuthnAssertionResponse `json:"assertion_response"`
}

// cliAttestationVerifyRequest is the typed request for CLI passkey registration verification.
type cliAttestationVerifyRequest struct {
	UserID              string                          `json:"user_id"`
	AttestationResponse models.WebAuthnAttestationResponse `json:"attestation_response"`
}

// getLocalOSUser retrieves the current OS user information.
func getLocalOSUser() *models.LocalOSUser {
	currentUser, err := user.Current()
	if err != nil {
		return nil
	}

	var domain, username string
	parts := strings.SplitN(currentUser.Username, "\\", 2)
	if len(parts) == 2 {
		domain = parts[0]
		username = parts[1]
	} else {
		username = currentUser.Username
	}

	var sid string
	if runtime.GOOS == "windows" {
		sid = currentUser.Uid
	}

	return &models.LocalOSUser{
		Domain:   domain,
		Username: username,
		UID:      currentUser.Uid,
		GID:      currentUser.Gid,
		SID:      sid,
	}
}

func GenerateCSR(commonName string) (csrPEM string, privKey *ecdsa.PrivateKey, err error) {
	privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
		DNSNames: []string{"localhost", "g8e.local"},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	csrPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEMBytes), privKey, nil
}

// NewSecureHTTPClient creates an HTTP client bound to the Operator's CA trust bundle.
// This ensures the CLI can validate the Operator's TLS certificate during CSR-based enrollment.
func NewSecureHTTPClient(cfg *config.Config) (*http.Client, error) {
	trustBundlePath := cfg.TrustBundlePath()
	if trustBundlePath == "" {
		return nil, constants.ErrGatewayURLRequired
	}

	caPEM, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, constants.ErrCAParseFailed
	}

	tlsConfig := &tls.Config{
		RootCAs: caPool,
		// Require TLS 1.3 for secure communication
		MinVersion: tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{Transport: transport}, nil
}

// FetchRootCAFingerprint fetches the root CA fingerprint from the gateway.
// This is used for OOB pinning verification during bootstrap.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL().
func FetchRootCAFingerprint(cfg *config.Config, baseURL string) (string, error) {
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	fingerprintURL := fmt.Sprintf("%s/.well-known/g8e/pki/fingerprint", discoveryURL)
	resp, err := http.Get(fingerprintURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var fpResp struct {
		RootCA string `json:"root_ca"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fpResp); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	return fpResp.RootCA, nil
}

// VerifyCAFingerprint verifies that a PEM-encoded CA bundle matches the expected fingerprint.
// The fingerprint should be a hex-encoded SHA-256 hash (64 characters).
func VerifyCAFingerprint(caPEM []byte, expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return nil
	}

	// Parse the PEM to extract the DER-encoded certificate
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return constants.ErrPEMDecodeFailed
	}

	if block.Type != "CERTIFICATE" {
		return constants.ErrInvalidPEMType
	}

	// Compute SHA-256 hash of the DER-encoded certificate
	hash := sha256.Sum256(block.Bytes)
	actualFP := hex.EncodeToString(hash[:])

	if actualFP != expectedFingerprint {
		return constants.ErrValidationFailed
	}

	return nil
}

// BootstrapWithURL allows overriding the gateway URL for testing.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL().
func BootstrapWithURL(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string, baseURL string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// Get local OS user information to send to gateway
	localOSUser := getLocalOSUser()

	req := models.BootstrapRequest{
		CSR:               operatorCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		LocalOSUser:       localOSUser,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	// Use bootstrap port (plain HTTP) for initial bootstrap
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	url := fmt.Sprintf("%s/api/v1/auth/bootstrap", discoveryURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Use plain HTTP client for bootstrap (no TLS required)
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" && regResp.HubTrustBundle != "" {
		if err := VerifyCAFingerprint([]byte(regResp.HubTrustBundle), caFingerprint); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
		}
	}

	return &regResp, nil
}

// CLIEnroll performs CLI-only enrollment after bootstrap when local CLI credentials are missing.
// This is used when the gateway is already bootstrapped but the CLI has lost its credentials.
// It uses the plain HTTP bootstrap port since the CLI has no mTLS credentials.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL().
func CLIEnroll(cfg *config.Config, cliCSR string, baseURL string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// Get local OS user information to send to gateway
	localOSUser := getLocalOSUser()

	req := models.CLIEnrollRequest{
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		LocalOSUser:       localOSUser,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	// Use bootstrap port (plain HTTP) for CLI enrollment
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	url := fmt.Sprintf("%s/api/v1/auth/cli/enroll", discoveryURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Use plain HTTP client for enrollment (no TLS required)
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	return &regResp, nil
}

// ReEnroll performs CSR-based re-enrollment using existing mTLS credentials.
// This is used when the platform is already bootstrapped and the CLI has valid certificates.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL() and cfg.OperatorPublicURL().
func ReEnroll(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string, baseURL string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// Fetch current trust bundle from Operator bootstrap endpoint to handle CA rotation
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	trustBundleURL := fmt.Sprintf("%s/.well-known/g8e/pki/ca-bundle", discoveryURL)
	trustBundleResp, err := http.Get(trustBundleURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer trustBundleResp.Body.Close()

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if trustBundleResp.StatusCode < http.StatusOK || trustBundleResp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, trustBundleResp.StatusCode)
	}

	currentTrustBundle, err := io.ReadAll(trustBundleResp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	if len(currentTrustBundle) == 0 {
		return nil, constants.ErrEmptyTrustBundle
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" {
		if err := VerifyCAFingerprint(currentTrustBundle, caFingerprint); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
		}
	}

	// Update local trust bundle with current version from operator
	trustBundlePath := cfg.TrustBundlePath()
	if err := os.MkdirAll(filepath.Dir(trustBundlePath), 0755); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	if err := os.WriteFile(trustBundlePath, currentTrustBundle, 0644); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}

	// Load existing CLI certificate for mTLS
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Use the freshly fetched trust bundle for TLS verification
	caPEM := currentTrustBundle

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, constants.ErrCAParseFailed
	}

	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cliCert},
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{Transport: transport}

	req := models.BootstrapRequest{
		CSR:               operatorCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	publicURL := cfg.OperatorPublicURL()
	if baseURL != "" {
		publicURL = baseURL
	}
	url := fmt.Sprintf("%s/api/v1/pki/devices/enroll", publicURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		// Check if this is a TLS certificate verification error (stale trust bundle)
		if isCertificateVerificationError(err) {
			return nil, fmt.Errorf("%w: %w", constants.ErrTrustBundleStale, err)
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	return &regResp, nil
}

// isCertificateVerificationError checks if an error is a TLS certificate verification error
// without using string matching on error messages.
func isCertificateVerificationError(err error) bool {
	if err == nil {
		return false
	}

	// Check for x509 certificate errors by unwrapping the error chain
	for {
		// Check for common x509 error types
		switch err.(type) {
		case x509.UnknownAuthorityError:
			return true
		case x509.HostnameError:
			return true
		case x509.CertificateInvalidError:
			return true
		}

		// Unwrap to check wrapped errors
		if unwrapped := errors.Unwrap(err); unwrapped != nil {
			err = unwrapped
			continue
		}
		break
	}

	return false
}

func SaveCredentials(cfg *config.Config, creds *Credentials) error {
	if err := os.MkdirAll(cfg.CredentialsDir, 0700); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	credsFile := cfg.CredentialsFile()
	credsData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	if err := os.WriteFile(credsFile, credsData, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	return nil
}

func LoadCredentials(cfg *config.Config) (*Credentials, error) {
	credsFile := cfg.CredentialsFile()
	credsData, err := os.ReadFile(credsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}

	var creds Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	return &creds, nil
}

// PerformNativeWindowsAuth attempts to authenticate using Windows Hello without a browser.
func PerformNativeWindowsAuth(cfg *config.Config) error {
	fmt.Printf("→ Starting native Windows Hello authentication...\n")
	creds, err := LoadCredentials(cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	if creds == nil || creds.UserID == "" {
		return constants.ErrNotAuthenticated
	}
	if creds.CLISessionID == "" {
		return constants.ErrNotAuthenticated
	}

	fmt.Printf("→ Loaded credentials - User ID: %s, CLI Session ID: %s\n", creds.UserID, creds.CLISessionID)

	// Load CLI certificate for mTLS authentication
	fmt.Printf("→ Loading CLI certificate from: %s\n", cfg.CLICertFile())
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Load trust bundle for TLS verification
	fmt.Printf("→ Loading trust bundle from: %s\n", cfg.TrustBundleFile())
	caPEM, err := os.ReadFile(cfg.TrustBundleFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return constants.ErrCAParseFailed
	}

	// Create HTTP client with mTLS (client certificate + trust bundle)
	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cliCert},
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{Transport: transport}

	gatewayURL := cfg.OperatorPublicURL()
	fmt.Printf("→ Gateway URL: %s\n", gatewayURL)

	// 1. Get Authentication Challenge
	challengeURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli/authenticate/challenge", gatewayURL)
	fmt.Printf("→ Requesting authentication challenge from: %s\n", challengeURL)
	reqBody, err := json.Marshal(models.PasskeyChallengeRequest{UserID: creds.UserID})
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}
	req, err := http.NewRequest("POST", challengeURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Add CLI session ID header for auth middleware
	req.Header.Set("X-G8E-CLI-Session-ID", creds.CLISessionID)

	fmt.Printf("→ Sending authentication challenge request...\n")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	fmt.Printf("→ Challenge response status: %d\n", resp.StatusCode)
	if resp.StatusCode == http.StatusOK {
		var challengeData models.PasskeyChallengeResponse
		if err := json.NewDecoder(resp.Body).Decode(&challengeData); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}

		if !challengeData.Success {
			if challengeData.NeedsSetup {
				fmt.Printf("No passkey registered for user %s. Starting registration...\n", creds.UserID)
				// Use browser-based registration on all platforms (including Windows)
				// The browser's WebAuthn API properly handles Windows Hello integration
				if err := RegisterPasskeyViaLocalhost(cfg, creds.UserID, creds.CLISessionID); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
				}

				// Re-attempt authentication after registration
				return PerformNativeWindowsAuth(cfg)
			}
			return fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, challengeData.Error)
		}

		// 2. Trigger Windows Hello
		origin := fmt.Sprintf("https://%s", challengeData.Options.Response.RelyingPartyID)
		clientData := webauthnClientData{
			Challenge: challengeData.Options.Response.Challenge.String(),
			Origin:    origin,
			Type:      "webauthn.get",
		}
		clientDataBytes, err := json.Marshal(clientData)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
		}

		assertion, err := AuthenticateWithWindowsHello(challengeData.Options.Response.RelyingPartyID, clientDataBytes)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
		}

		// 3. Verify Authentication
		verifyURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli/authenticate/verify", gatewayURL)
		verifyReq := cliAssertionVerifyRequest{
			UserID: creds.UserID,
			AssertionResponse: models.WebAuthnAssertionResponse{
				ID:                assertion.Id,
				RawID:             base64.RawURLEncoding.EncodeToString(assertion.RawId),
				ClientDataJSON:    base64.RawURLEncoding.EncodeToString(clientDataBytes),
				AuthenticatorData: base64.RawURLEncoding.EncodeToString(assertion.AuthenticatorData),
				Signature:         base64.RawURLEncoding.EncodeToString(assertion.Signature),
				UserHandle:        base64.RawURLEncoding.EncodeToString(assertion.UserHandle),
			},
		}

		verifyBody, err := json.Marshal(verifyReq)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
		}
		verifyReqHTTP, err := http.NewRequest("POST", verifyURL, bytes.NewReader(verifyBody))
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
		}
		verifyReqHTTP.Header.Set("Content-Type", "application/json")
		// Add CLI session ID header for auth middleware
		verifyReqHTTP.Header.Set("X-G8E-CLI-Session-ID", creds.CLISessionID)

		verifyResp, err := client.Do(verifyReqHTTP)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
		}
		defer verifyResp.Body.Close()

		if verifyResp.StatusCode != http.StatusOK {
			return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, verifyResp.StatusCode)
		}

		var verifyResult struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.NewDecoder(verifyResp.Body).Decode(&verifyResult); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}

		if !verifyResult.Success {
			return fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, verifyResult.Error)
		}

		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}
	fmt.Printf("→ Challenge request failed - Response body: %s\n", string(body))
	return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
}

// RegisterPasskeyWithWindowsHello performs native passkey registration using Windows Hello APIs.
func RegisterPasskeyWithWindowsHello(cfg *config.Config, userID, cliSessionID string) error {
	gatewayURL := cfg.OperatorPublicURL()
	fmt.Printf("→ Starting Windows Hello passkey registration for user %s\n", userID)
	fmt.Printf("→ Gateway URL: %s\n", gatewayURL)
	fmt.Printf("→ CLI Session ID: %s\n", cliSessionID)

	// Get local OS user information directly
	localOSUser := getLocalOSUser()
	if localOSUser == nil || localOSUser.Username == "" {
		return constants.ErrUserNotFound
	}
	userName := localOSUser.Username
	fmt.Printf("→ OS username: %s\n", userName)

	// Load CLI certificate for mTLS authentication
	fmt.Printf("→ Loading CLI certificate from: %s\n", cfg.CLICertFile())
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Load trust bundle for TLS verification
	fmt.Printf("→ Loading trust bundle from: %s\n", cfg.TrustBundleFile())
	caPEM, err := os.ReadFile(cfg.TrustBundleFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return constants.ErrCAParseFailed
	}

	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cliCert},
		MinVersion:   tls.VersionTLS13,
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}

	// 1. Get Registration Challenge
	challengeURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli-register/challenge", gatewayURL)
	fmt.Printf("→ Requesting registration challenge from: %s\n", challengeURL)
	reqBody, err := json.Marshal(models.PasskeyRegisterChallengeRequest{
		UserID:   userID,
		UserName: userName,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}
	req, err := http.NewRequest("POST", challengeURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-G8E-CLI-Session-ID", cliSessionID)

	// Reuse the existing mTLS client for registration challenge
	fmt.Printf("→ Sending registration challenge request...\n")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	fmt.Printf("→ Challenge response status: %d\n", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
		}
		fmt.Printf("→ Challenge response body: %s\n", string(body))
		return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var challengeData struct {
		Success bool `json:"success"`
		Options struct {
			PublicKey struct {
				Challenge    string `json:"challenge"`
				RelyingParty struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"rp"`
				User struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"user"`
			} `json:"publicKey"`
		} `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&challengeData); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	// 2. Trigger Windows Hello Registration
	fmt.Printf("→ Triggering Windows Hello registration prompt...\n")
	fmt.Printf("→ Relying Party ID: %s\n", challengeData.Options.PublicKey.RelyingParty.ID)
	fmt.Printf("→ Relying Party Name: %s\n", challengeData.Options.PublicKey.RelyingParty.Name)
	// The gateway provides a base64url-encoded user ID
	// Windows Hello API expects raw bytes for the user ID, so we need to decode it
	// Windows Hello requires user ID to be 1-64 bytes per WEBAUTHN_MAX_USER_ID_LENGTH
	userIDBase64 := challengeData.Options.PublicKey.User.ID
	userIDBytes, err := base64.RawURLEncoding.DecodeString(userIDBase64)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}
	if len(userIDBytes) > 64 {
		return constants.ErrValidationFailed
	}
	fmt.Printf("→ Windows Hello user ID (decoded): %x (%d bytes)\n", userIDBytes, len(userIDBytes))

	// Construct proper clientDataJSON for Windows Hello API
	// WebAuthn requires clientDataJSON to contain: challenge, origin, type
	origin := fmt.Sprintf("https://%s", challengeData.Options.PublicKey.RelyingParty.ID)
	clientData := webauthnClientData{
		Challenge: challengeData.Options.PublicKey.Challenge,
		Origin:    origin,
		Type:      "webauthn.create",
	}
	clientDataBytes, err := json.Marshal(clientData)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	attestation, err := RegisterWithWindowsHello(
		challengeData.Options.PublicKey.RelyingParty.ID,
		challengeData.Options.PublicKey.RelyingParty.Name,
		userIDBytes,
		challengeData.Options.PublicKey.User.Name,
		clientDataBytes,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	fmt.Printf("→ Windows Hello registration successful, verifying with gateway...\n")

	// 3. Verify Registration
	verifyURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli-register/verify", gatewayURL)
	verifyReq := cliAttestationVerifyRequest{
		UserID: userID,
		AttestationResponse: models.WebAuthnAttestationResponse{
			ID:                attestation.Id,
			RawID:             base64.RawURLEncoding.EncodeToString(attestation.RawId),
			ClientDataJSON:    base64.RawURLEncoding.EncodeToString(clientDataBytes),
			AttestationObject: base64.RawURLEncoding.EncodeToString(attestation.AttestationObject),
		},
	}

	verifyBody, err := json.Marshal(verifyReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}
	verifyReqHTTP, err := http.NewRequest("POST", verifyURL, bytes.NewReader(verifyBody))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	verifyReqHTTP.Header.Set("Content-Type", "application/json")
	verifyReqHTTP.Header.Set("X-G8E-CLI-Session-ID", cliSessionID)

	verifyResp, err := client.Do(verifyReqHTTP)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, verifyResp.StatusCode)
	}

	fmt.Println("✓ Passkey registered successfully via Windows Hello!")
	return nil
}

func DeleteCredentials(cfg *config.Config) error {
	credsFile := cfg.CredentialsFile()
	if err := os.Remove(credsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	certFiles := []string{
		cfg.CLICertFile(),
		cfg.CLIKeyFile(),
		cfg.TrustBundlePath(),
	}

	for _, file := range certFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
		}
	}

	return nil
}

func SaveCertAndKey(certPEM, chainPEM string, key *ecdsa.PrivateKey, certFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0700); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	certContent := certPEM
	if chainPEM != "" {
		certContent += "\n" + chainPEM
	}

	if err := os.WriteFile(certFile, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	return nil
}

func CheckOperatorRunning(cfg *config.Config) error {
	return CheckOperatorRunningAtURL(cfg.OperatorDiscoveryURL())
}

func CheckOperatorRunningAtURL(operatorURL string) error {
	// Parse the URL to extract host:port
	parsedURL, err := url.Parse(operatorURL)
	if err != nil {
		return fmt.Errorf("%w: %s", constants.ErrGatewayURLRequired, operatorURL)
	}

	hostPort := parsedURL.Host
	if hostPort == "" {
		return fmt.Errorf("%w: %s", constants.ErrGatewayURLRequired, operatorURL)
	}
	// Force IPv4 by replacing localhost with 127.0.0.1 to prevent IPv6 resolution
	if strings.HasPrefix(hostPort, "localhost:") {
		hostPort = "127.0.0.1" + hostPort[9:]
	}
	// Try to connect to the port
	conn, err := net.Dial(string(constants.NetworkProtocolTCP), hostPort)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}
	conn.Close()

	return nil
}

// CheckBootstrapStatus returns whether the platform has been bootstrapped.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL().
func CheckBootstrapStatus(cfg *config.Config, baseURL string) (bool, error) {
	// Check remote bootstrap status via bootstrap port (plain HTTP)
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	url := fmt.Sprintf("%s/api/v1/auth/bootstrap/status", discoveryURL)
	resp, err := http.Get(url)
	if err != nil {
		// If Operator is not reachable, we cannot confirm bootstrap status
		// Return false (not bootstrapped) without error to allow tests to proceed
		return false, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	var statusResp struct {
		Bootstrapped bool `json:"bootstrapped"`
	}
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	return statusResp.Bootstrapped, nil
}

// parseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func parseCertPEM(certFile string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertReadFailed, err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, constants.ErrPEMDecodeFailed
	}

	if block.Type != "CERTIFICATE" {
		return nil, constants.ErrInvalidPEMType
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertParseFailed, err)
	}

	return cert, nil
}

// isCertExpiringSoon checks if a certificate is expiring within the renewal threshold.
// The threshold is set to 24 hours before expiry to allow ample time for renewal.
func isCertExpiringSoon(cert *x509.Certificate) bool {
	renewalThreshold := 24 * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry <= renewalThreshold
}

// CheckCertExpiry checks if the local CLI or Operator certificate is expiring soon.
// Returns true if the certificate is expiring within the renewal threshold.
func CheckCertExpiry(certFile string) (bool, error) {
	cert, err := parseCertPEM(certFile)
	if err != nil {
		return false, err
	}

	return isCertExpiringSoon(cert), nil
}

// AutoRenewCertificate performs automatic re-enrollment if the certificate is expiring soon.
// This is a fail-closed operation: if renewal fails, it returns an error rather than falling back.
func AutoRenewCertificate(cfg *config.Config, certType string, caFingerprint string) error {
	var certFile string
	switch certType {
	case "cli":
		certFile = cfg.CLICertFile()
	case "operator":
		certFile = cfg.OperatorCertFile()
	default:
		return constants.ErrValidationFailed
	}

	expiringSoon, err := CheckCertExpiry(certFile)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertParseFailed, err)
	}

	if !expiringSoon {
		return nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}

	cliCSR, cliKey, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	regResp, err := ReEnroll(cfg, "", cliCSR, caFingerprint, "")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return constants.ErrMissingRequiredField
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	if regResp.HubTrustBundle != "" {
		if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
	}

	creds := &Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := SaveCredentials(cfg, creds); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	return nil
}

// EnrollWithGateway enrolls a device with a remote Gateway via CSR-based enrollment.
// This is used for deploying operators on remote hosts that need to connect to a central Gateway.
func EnrollWithGateway(cfg *config.Config, gatewayEndpoint, operatorCSR, cliCSR string, caFingerprint string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}

	req := models.DeviceEnrollRequest{
		CSR:               operatorCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		Hostname:          hostname,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	// Use the device enrollment endpoint for initial enrollment (no mTLS required)
	url := fmt.Sprintf("http://%s/api/v1/auth/device/enroll", gatewayEndpoint)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// For initial enrollment without mTLS, use plain HTTP client
	client := &http.Client{}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	if !regResp.Success {
		return nil, fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" && regResp.HubTrustBundle != "" {
		if err := VerifyCAFingerprint([]byte(regResp.HubTrustBundle), caFingerprint); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
		}
	}

	return &regResp, nil
}

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

func GenerateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC in GenerateCSR: %v\n", r)
			fmt.Fprintf(os.Stderr, "Windows CSP (Cryptographic Service Provider) error.\n")
			fmt.Fprintf(os.Stderr, "Run PowerShell as Administrator and try again.\n")
		}
	}()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
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
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

// NewSecureHTTPClient creates an HTTP client bound to the Operator's CA trust bundle.
// This ensures the CLI can validate the Operator's TLS certificate during CSR-based enrollment.
func NewSecureHTTPClient(cfg *config.Config) (*http.Client, error) {
	trustBundlePath := cfg.TrustBundlePath()
	if trustBundlePath == "" {
		return nil, fmt.Errorf("trust bundle path not configured")
	}

	caPEM, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read trust bundle from %s: %w", trustBundlePath, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA certificates from trust bundle")
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
func FetchRootCAFingerprint(cfg *config.Config) (string, error) {
	fingerprintURL := fmt.Sprintf("%s/.well-known/g8e/pki/fingerprint", cfg.OperatorDiscoveryURL())
	resp, err := http.Get(fingerprintURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch root CA fingerprint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fingerprint fetch returned HTTP %d", resp.StatusCode)
	}

	var fpResp struct {
		RootCA string `json:"root_ca"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fpResp); err != nil {
		return "", fmt.Errorf("failed to decode fingerprint response: %w", err)
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
		return fmt.Errorf("failed to decode CA PEM")
	}

	if block.Type != "CERTIFICATE" {
		return fmt.Errorf("PEM block is not a certificate (type: %s)", block.Type)
	}

	// Compute SHA-256 hash of the DER-encoded certificate
	hash := sha256.Sum256(block.Bytes)
	actualFP := hex.EncodeToString(hash[:])

	if actualFP != expectedFingerprint {
		return fmt.Errorf("CA fingerprint mismatch: expected %s, got %s", expectedFingerprint, actualFP)
	}

	return nil
}

func Bootstrap(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use bootstrap port (plain HTTP) for initial bootstrap
	url := fmt.Sprintf("%s/api/v1/auth/bootstrap", cfg.OperatorDiscoveryURL())
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Use plain HTTP client for bootstrap (no TLS required)
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("bootstrap failed: %s", regResp.Error)
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" && regResp.HubTrustBundle != "" {
		if err := VerifyCAFingerprint([]byte(regResp.HubTrustBundle), caFingerprint); err != nil {
			return nil, fmt.Errorf("CA fingerprint verification failed: %w", err)
		}
	}

	return &regResp, nil
}

// CLIEnroll performs CLI-only enrollment after bootstrap when local CLI credentials are missing.
// This is used when the gateway is already bootstrapped but the CLI has lost its credentials.
// It uses the plain HTTP bootstrap port since the CLI has no mTLS credentials.
func CLIEnroll(cfg *config.Config, cliCSR string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use bootstrap port (plain HTTP) for CLI enrollment
	url := fmt.Sprintf("%s/api/v1/auth/cli/enroll", cfg.OperatorDiscoveryURL())
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Use plain HTTP client for enrollment (no TLS required)
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to enroll CLI: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("CLI enrollment failed: %s", regResp.Error)
	}

	return &regResp, nil
}

// ReEnroll performs CSR-based re-enrollment using existing mTLS credentials.
// This is used when the platform is already bootstrapped and the CLI has valid certificates.
func ReEnroll(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string) (*RegistrationResponse, error) {
	// Generate proper system fingerprint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
	}

	// Fetch current trust bundle from Operator bootstrap endpoint to handle CA rotation
	trustBundleURL := fmt.Sprintf("%s/.well-known/g8e/pki/ca-bundle", cfg.OperatorDiscoveryURL())
	trustBundleResp, err := http.Get(trustBundleURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trust bundle from operator: %w", err)
	}
	defer trustBundleResp.Body.Close()

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if trustBundleResp.StatusCode < http.StatusOK || trustBundleResp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("trust bundle fetch returned HTTP %d", trustBundleResp.StatusCode)
	}

	currentTrustBundle, err := io.ReadAll(trustBundleResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read trust bundle response: %w", err)
	}

	if len(currentTrustBundle) == 0 {
		return nil, fmt.Errorf("fetched trust bundle is empty")
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" {
		if err := VerifyCAFingerprint(currentTrustBundle, caFingerprint); err != nil {
			return nil, fmt.Errorf("CA fingerprint verification failed: %w", err)
		}
	}

	// Update local trust bundle with current version from operator
	trustBundlePath := cfg.TrustBundlePath()
	if err := os.MkdirAll(filepath.Dir(trustBundlePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create trust directory: %w", err)
	}
	if err := os.WriteFile(trustBundlePath, currentTrustBundle, 0644); err != nil {
		return nil, fmt.Errorf("failed to write trust bundle: %w", err)
	}

	// Load existing CLI certificate for mTLS
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load CLI certificate: %w", err)
	}

	// Use the freshly fetched trust bundle for TLS verification
	caPEM := currentTrustBundle

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA certificates")
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/pki/devices/enroll", cfg.OperatorPublicURL())
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		// Check if this is a TLS certificate verification error (stale trust bundle)
		if isCertificateVerificationError(err) {
			return nil, fmt.Errorf("%w: %w", constants.ErrTrustBundleStale, err)
		}
		return nil, fmt.Errorf("failed to re-enroll: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("re-enrollment failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d): %w\nBody: %s", resp.StatusCode, err, string(respBody))
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("re-enrollment failed: %s", regResp.Error)
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
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	credsFile := cfg.CredentialsFile()
	credsData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credsFile, credsData, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
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
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

// PerformNativeWindowsAuth attempts to authenticate using Windows Hello without a browser.
func PerformNativeWindowsAuth(cfg *config.Config) error {
	fmt.Printf("→ Starting native Windows Hello authentication...\n")
	creds, err := LoadCredentials(cfg)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}
	if creds == nil || creds.UserID == "" {
		return fmt.Errorf("no user ID found in credentials; run 'g8e auth login' to enroll first")
	}
	if creds.CLISessionID == "" {
		return fmt.Errorf("no CLI session ID found in credentials; run 'g8e auth login' to enroll first")
	}

	fmt.Printf("→ Loaded credentials - User ID: %s, CLI Session ID: %s\n", creds.UserID, creds.CLISessionID)

	// Load CLI certificate for mTLS authentication
	fmt.Printf("→ Loading CLI certificate from: %s\n", cfg.CLICertFile())
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("failed to load CLI certificate: %w", err)
	}

	// Load trust bundle for TLS verification
	fmt.Printf("→ Loading trust bundle from: %s\n", cfg.TrustBundleFile())
	caPEM, err := os.ReadFile(cfg.TrustBundleFile())
	if err != nil {
		return fmt.Errorf("failed to read trust bundle from %s: %w", cfg.TrustBundleFile(), err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA certificates from trust bundle")
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
		return fmt.Errorf("failed to marshal challenge request: %w", err)
	}
	req, err := http.NewRequest("POST", challengeURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Add CLI session ID header for auth middleware
	req.Header.Set("X-G8E-CLI-Session-ID", creds.CLISessionID)

	fmt.Printf("→ Sending authentication challenge request...\n")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get challenge: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("→ Challenge response status: %d\n", resp.StatusCode)
	if resp.StatusCode == http.StatusOK {
		var challengeData models.PasskeyChallengeResponse
		if err := json.NewDecoder(resp.Body).Decode(&challengeData); err != nil {
			return fmt.Errorf("failed to decode challenge: %w", err)
		}

		if !challengeData.Success {
			if challengeData.NeedsSetup {
				fmt.Printf("No passkey registered for user %s. Starting registration...\n", creds.UserID)
				// Use browser-based registration on all platforms (including Windows)
				// The browser's WebAuthn API properly handles Windows Hello integration
				if err := RegisterPasskeyViaLocalhost(cfg, creds.UserID, creds.CLISessionID); err != nil {
					return fmt.Errorf("passkey registration failed: %w", err)
				}

				// Re-attempt authentication after registration
				return PerformNativeWindowsAuth(cfg)
			}
			return fmt.Errorf("gateway returned failure for challenge request: %s", challengeData.Error)
		}

		// 2. Trigger Windows Hello
		origin := fmt.Sprintf("https://%s", challengeData.Options.Response.RelyingPartyID)
		clientDataJSON := map[string]interface{}{
			"challenge": challengeData.Options.Response.Challenge,
			"origin":    origin,
			"type":      "webauthn.get",
		}
		clientDataBytes, err := json.Marshal(clientDataJSON)
		if err != nil {
			return fmt.Errorf("failed to marshal clientDataJSON: %w", err)
		}

		assertion, err := AuthenticateWithWindowsHello(challengeData.Options.Response.RelyingPartyID, clientDataBytes)
		if err != nil {
			return fmt.Errorf("windows Hello authentication failed: %w", err)
		}

		// 3. Verify Authentication
		verifyURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli/authenticate/verify", gatewayURL)
		verifyReq := map[string]interface{}{
			"user_id": creds.UserID,
			"assertion_response": map[string]interface{}{
				"id":                assertion.Id,
				"rawId":             base64.RawURLEncoding.EncodeToString(assertion.RawId),
				"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataBytes),
				"authenticatorData": base64.RawURLEncoding.EncodeToString(assertion.AuthenticatorData),
				"signature":         base64.RawURLEncoding.EncodeToString(assertion.Signature),
				"userHandle":        base64.RawURLEncoding.EncodeToString(assertion.UserHandle),
			},
		}

		verifyBody, _ := json.Marshal(verifyReq)
		verifyReqHTTP, err := http.NewRequest("POST", verifyURL, bytes.NewReader(verifyBody))
		if err != nil {
			return fmt.Errorf("failed to create verify request: %w", err)
		}
		verifyReqHTTP.Header.Set("Content-Type", "application/json")
		// Add CLI session ID header for auth middleware
		verifyReqHTTP.Header.Set("X-G8E-CLI-Session-ID", creds.CLISessionID)

		verifyResp, err := client.Do(verifyReqHTTP)
		if err != nil {
			return fmt.Errorf("failed to verify assertion: %w", err)
		}
		defer verifyResp.Body.Close()

		if verifyResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(verifyResp.Body)
			return fmt.Errorf("verification failed (%d): %s", verifyResp.StatusCode, string(body))
		}

		var verifyResult struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.NewDecoder(verifyResp.Body).Decode(&verifyResult); err != nil {
			return fmt.Errorf("failed to decode verification result: %w", err)
		}

		if !verifyResult.Success {
			return fmt.Errorf("authentication failed: %s", verifyResult.Error)
		}

		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("→ Challenge request failed - Response body: %s\n", string(body))
	return fmt.Errorf("challenge request failed (%d): %s", resp.StatusCode, string(body))
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
		return fmt.Errorf("failed to get local OS user information")
	}
	userName := localOSUser.Username
	fmt.Printf("→ OS username: %s\n", userName)

	// Load CLI certificate for mTLS authentication
	fmt.Printf("→ Loading CLI certificate from: %s\n", cfg.CLICertFile())
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("failed to load CLI cert: %w", err)
	}

	// Load trust bundle for TLS verification
	fmt.Printf("→ Loading trust bundle from: %s\n", cfg.TrustBundleFile())
	caPEM, err := os.ReadFile(cfg.TrustBundleFile())
	if err != nil {
		return fmt.Errorf("failed to read trust bundle from %s: %w", cfg.TrustBundleFile(), err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA certificates from trust bundle")
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
		return fmt.Errorf("failed to marshal challenge request: %w", err)
	}
	req, err := http.NewRequest("POST", challengeURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create challenge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-G8E-CLI-Session-ID", cliSessionID)

	// Reuse the existing mTLS client for registration challenge
	fmt.Printf("→ Sending registration challenge request...\n")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get registration challenge: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("→ Challenge response status: %d\n", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("→ Challenge response body: %s\n", string(body))
		return fmt.Errorf("failed to get registration challenge (%d): %s", resp.StatusCode, string(body))
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
		return fmt.Errorf("failed to decode registration challenge: %w", err)
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
		return fmt.Errorf("failed to decode user ID: %w", err)
	}
	if len(userIDBytes) > 64 {
		return fmt.Errorf("user ID too long for Windows Hello: %d bytes (max 64)", len(userIDBytes))
	}
	fmt.Printf("→ Windows Hello user ID (decoded): %x (%d bytes)\n", userIDBytes, len(userIDBytes))

	// Construct proper clientDataJSON for Windows Hello API
	// WebAuthn requires clientDataJSON to contain: challenge, origin, type
	origin := fmt.Sprintf("https://%s", challengeData.Options.PublicKey.RelyingParty.ID)
	clientDataJSON := map[string]interface{}{
		"challenge": challengeData.Options.PublicKey.Challenge,
		"origin":    origin,
		"type":      "webauthn.create",
	}
	clientDataBytes, err := json.Marshal(clientDataJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal clientDataJSON: %w", err)
	}

	attestation, err := RegisterWithWindowsHello(
		challengeData.Options.PublicKey.RelyingParty.ID,
		challengeData.Options.PublicKey.RelyingParty.Name,
		userIDBytes,
		challengeData.Options.PublicKey.User.Name,
		clientDataBytes,
	)
	if err != nil {
		return fmt.Errorf("windows Hello registration failed: %w", err)
	}

	fmt.Printf("→ Windows Hello registration successful, verifying with gateway...\n")

	// 3. Verify Registration
	verifyURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli-register/verify", gatewayURL)
	verifyReq := map[string]interface{}{
		"user_id": userID,
		"attestation_response": map[string]interface{}{
			"id":                attestation.Id,
			"rawId":             base64.RawURLEncoding.EncodeToString(attestation.RawId),
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataBytes),
			"attestationObject": base64.RawURLEncoding.EncodeToString(attestation.AttestationObject),
		},
	}

	verifyBody, _ := json.Marshal(verifyReq)
	verifyReqHTTP, err := http.NewRequest("POST", verifyURL, bytes.NewReader(verifyBody))
	if err != nil {
		return fmt.Errorf("failed to create verify request: %w", err)
	}
	verifyReqHTTP.Header.Set("Content-Type", "application/json")
	verifyReqHTTP.Header.Set("X-G8E-CLI-Session-ID", cliSessionID)

	verifyResp, err := client.Do(verifyReqHTTP)
	if err != nil {
		return fmt.Errorf("failed to verify registration: %w", err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(verifyResp.Body)
		return fmt.Errorf("registration verification failed (%d): %s", verifyResp.StatusCode, string(body))
	}

	fmt.Println("✓ Passkey registered successfully via Windows Hello!")
	return nil
}

func DeleteCredentials(cfg *config.Config) error {
	credsFile := cfg.CredentialsFile()
	if err := os.Remove(credsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credentials file: %w", err)
	}

	certFiles := []string{
		cfg.CLICertFile(),
		cfg.CLIKeyFile(),
		cfg.TrustBundlePath(),
	}

	for _, file := range certFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", file, err)
		}
	}

	return nil
}

// BootstrapCLIWithoutPasskey performs CLI enrollment without requiring passkey registration.
// This is used for automatic bootstrap during gateway start to provide a seamless first-time experience.
func BootstrapCLIWithoutPasskey(cfg *config.Config) error {
	// Check if platform is already bootstrapped
	bootstrapped, err := CheckBootstrapStatus(cfg)
	if err != nil {
		return fmt.Errorf("failed to check bootstrap status: %w", err)
	}

	// Check if local credentials already exist
	hasLocalCreds := true
	if _, err := os.Stat(cfg.CredentialsFile()); os.IsNotExist(err) {
		hasLocalCreds = false
	}
	if _, err := os.Stat(cfg.CLICertFile()); os.IsNotExist(err) {
		hasLocalCreds = false
	}

	// If platform is already bootstrapped and has local credentials, nothing to do
	if bootstrapped && hasLocalCreds {
		return nil
	}

	// Generate keys and CSRs
	hostname, _ := os.Hostname()
	cliCSR, cliKey, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate CLI CSR: %w", err)
	}

	var regResp *RegistrationResponse
	if !bootstrapped {
		// First-time bootstrap
		regResp, err = Bootstrap(cfg, "", cliCSR, "")
		if err != nil {
			return fmt.Errorf("failed to bootstrap: %w", err)
		}
	} else {
		// Platform bootstrapped but CLI credentials missing
		regResp, err = CLIEnroll(cfg, cliCSR)
		if err != nil {
			return fmt.Errorf("failed to enroll CLI: %w", err)
		}
	}

	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return fmt.Errorf("unexpected response (missing required fields)")
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("failed to save CLI credentials: %w", err)
	}

	if regResp.HubTrustBundle != "" {
		if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("failed to save hub trust bundle: %w", err)
		}
	}

	creds := &Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := SaveCredentials(cfg, creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

func SaveCertAndKey(certPEM, chainPEM string, key *ecdsa.PrivateKey, certFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	certContent := certPEM
	if chainPEM != "" {
		certContent += "\n" + chainPEM
	}

	if err := os.WriteFile(certFile, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("failed to write cert file: %w", err)
	}

	return nil
}

func CheckOperatorRunning(cfg *config.Config) error {
	return CheckOperatorRunningAtURL(cfg.OperatorDiscoveryURL())
}

func CheckOperatorRunningAtURL(operatorURL string) error {
	// Parse the URL to extract host:port
	parts := strings.Split(operatorURL, "://")
	if len(parts) != 2 {
		return fmt.Errorf("invalid Operator URL: %s", operatorURL)
	}

	hostPort := parts[1]
	// Force IPv4 by replacing localhost with 127.0.0.1 to prevent IPv6 resolution
	if strings.HasPrefix(hostPort, "localhost:") {
		hostPort = "127.0.0.1" + hostPort[9:]
	}
	// Try to connect to the port
	conn, err := net.Dial(string(constants.NetworkProtocolTCP), hostPort)
	if err != nil {
		return fmt.Errorf("g8e Gateway is not running or not responding at %s: %w", operatorURL, err)
	}
	conn.Close()

	return nil
}

// CheckBootstrapStatus returns whether the platform has been bootstrapped
func CheckBootstrapStatus(cfg *config.Config) (bool, error) {
	// Check remote bootstrap status via bootstrap port (plain HTTP)
	url := fmt.Sprintf("%s/api/v1/auth/bootstrap/status", cfg.OperatorDiscoveryURL())
	resp, err := http.Get(url)
	if err != nil {
		// If Operator is not reachable, we cannot confirm bootstrap status
		// Return false (not bootstrapped) without error to allow tests to proceed
		return false, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var statusResp struct {
		Bootstrapped bool `json:"bootstrapped"`
	}
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	return statusResp.Bootstrapped, nil
}

// parseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func parseCertPEM(certFile string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from certificate file")
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("PEM block is not a certificate (type: %s)", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
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
		return fmt.Errorf("unknown certificate type: %s", certType)
	}

	expiringSoon, err := CheckCertExpiry(certFile)
	if err != nil {
		return fmt.Errorf("failed to check certificate expiry: %w", err)
	}

	if !expiringSoon {
		return nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	cliCSR, cliKey, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate CLI CSR: %w", err)
	}

	regResp, err := ReEnroll(cfg, "", cliCSR, caFingerprint)
	if err != nil {
		return fmt.Errorf("automatic re-enrollment failed: %w", err)
	}

	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return fmt.Errorf("unexpected re-enrollment response (missing required fields)")
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("failed to save renewed CLI credentials: %w", err)
	}

	if regResp.HubTrustBundle != "" {
		if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("failed to save renewed hub trust bundle: %w", err)
		}
	}

	creds := &Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := SaveCredentials(cfg, creds); err != nil {
		return fmt.Errorf("failed to save renewed credentials: %w", err)
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
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	req := models.DeviceEnrollRequest{
		CSR:               operatorCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		Hostname:          hostname,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use the device enrollment endpoint for initial enrollment (no mTLS required)
	url := fmt.Sprintf("http://%s/api/v1/auth/device/enroll", gatewayEndpoint)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// For initial enrollment without mTLS, use plain HTTP client
	client := &http.Client{}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Accept 2xx status codes as success (200 OK, 201 Created, etc.)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("enrollment failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !regResp.Success {
		return nil, fmt.Errorf("enrollment failed: %s", regResp.Error)
	}

	// Verify CA bundle fingerprint if pin is provided
	if caFingerprint != "" && regResp.HubTrustBundle != "" {
		if err := VerifyCAFingerprint([]byte(regResp.HubTrustBundle), caFingerprint); err != nil {
			return nil, fmt.Errorf("CA fingerprint verification failed: %w", err)
		}
	}

	return &regResp, nil
}

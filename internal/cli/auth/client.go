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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// httpTimeout is the default timeout for all HTTP clients in the auth package.
const httpTimeout = 30 * time.Second

// readFileWithFS reads a file via fileSvc if the path is within .g8e/, otherwise via os.ReadFile.
func readFileWithFS(fileSvc fs.RuntimeFileService, absPath string) ([]byte, error) {
	if relPath, err := fileSvc.Rel(absPath); err == nil {
		return fileSvc.ReadFile(context.Background(), relPath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFileReadFailed, err)
	}
	return data, nil
}

// writeFileWithFS writes a file via fileSvc if the path is within .g8e/, otherwise via os.WriteFile.
func writeFileWithFS(fileSvc fs.RuntimeFileService, absPath string, data []byte, mode os.FileMode) error {
	if relPath, err := fileSvc.Rel(absPath); err == nil {
		return fileSvc.WriteFile(context.Background(), relPath, data, mode)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	if err := os.WriteFile(absPath, data, mode); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return nil
}

// removeFileWithFS removes a file via fileSvc if the path is within .g8e/, otherwise via os.Remove.
// No-op if the file does not exist.
func removeFileWithFS(fileSvc fs.RuntimeFileService, absPath string) error {
	if relPath, err := fileSvc.Rel(absPath); err == nil {
		return fileSvc.Remove(context.Background(), relPath)
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	return nil
}

// isNotFound checks if an error indicates the file does not exist.
func isNotFound(err error) bool {
	return errors.Is(err, constants.ErrNotFound) || errors.Is(err, os.ErrNotExist)
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
func NewSecureHTTPClient(fileSvc fs.RuntimeFileService, cfg *config.Config) (*http.Client, error) {
	trustBundlePath := cfg.TrustBundlePath()
	if trustBundlePath == "" {
		return nil, constants.ErrGatewayURLRequired
	}

	caPEM, err := readFileWithFS(fileSvc, trustBundlePath)
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

	return &http.Client{Transport: transport, Timeout: httpTimeout}, nil
}

// FetchRootCAFingerprint fetches the root CA fingerprint from the gateway.
// This is used for OOB pinning verification during bootstrap.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL().
func FetchRootCAFingerprint(cfg *config.Config, baseURL string) (string, error) {
	discoveryURL := cfg.OperatorDiscoveryURL()
	if baseURL != "" {
		discoveryURL = baseURL
	}
	fingerprintURL := fmt.Sprintf("%s%s", discoveryURL, constants.APIPaths.WellKnownPKIFingerprint)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(fingerprintURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var fpResp models.PKIFingerprintResponse
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
	url := fmt.Sprintf("%s%s", discoveryURL, constants.APIPaths.AuthBootstrap)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)

	// Use plain HTTP client for bootstrap (no TLS required)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

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
	url := fmt.Sprintf("%s%s", discoveryURL, constants.APIPaths.AuthCLIEnroll)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)

	// Use plain HTTP client for enrollment (no TLS required)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

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

// ReEnroll performs CSR-based re-enrollment using existing mTLS credentials.
// This is used when the platform is already bootstrapped and the CLI has valid certificates.
// If baseURL is empty, it uses cfg.OperatorDiscoveryURL() and cfg.OperatorPublicURL().
func ReEnroll(fileSvc fs.RuntimeFileService, cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string, baseURL string) (*RegistrationResponse, error) {
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
	trustBundleURL := fmt.Sprintf("%s%s", discoveryURL, constants.APIPaths.WellKnownPKICABundle)
	client := &http.Client{Timeout: httpTimeout}
	trustBundleResp, err := client.Get(trustBundleURL)
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

	client = &http.Client{Transport: transport, Timeout: httpTimeout}

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
	url := fmt.Sprintf("%s%s", publicURL, constants.APIPaths.PKIDevicesEnroll)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)

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

	// Write updated trust bundle only after successful mTLS enrollment
	trustBundlePath := cfg.TrustBundlePath()
	if err := writeFileWithFS(fileSvc, trustBundlePath, currentTrustBundle, constants.PermFilePublic); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
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

func SaveCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config, creds *Credentials) error {
	credsFile := cfg.CredentialsFile()
	credsData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	if err := writeFileWithFS(fileSvc, credsFile, credsData, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}

	return nil
}

func LoadCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) (*Credentials, error) {
	credsFile := cfg.CredentialsFile()
	credsData, err := readFileWithFS(fileSvc, credsFile)
	if err != nil {
		if isNotFound(err) {
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

func DeleteCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) error {
	credsFile := cfg.CredentialsFile()
	if err := removeFileWithFS(fileSvc, credsFile); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	certFiles := []string{
		cfg.CLICertFile(),
		cfg.CLIKeyFile(),
		cfg.TrustBundlePath(),
	}

	for _, file := range certFiles {
		if err := removeFileWithFS(fileSvc, file); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
		}
	}

	return nil
}

func SaveCertAndKey(fileSvc fs.RuntimeFileService, certPEM, chainPEM string, key *ecdsa.PrivateKey, certFile, keyFile string) error {
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := writeFileWithFS(fileSvc, keyFile, keyPEM, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	certContent := certPEM
	if chainPEM != "" {
		certContent += "\n" + chainPEM
	}

	if err := writeFileWithFS(fileSvc, certFile, []byte(certContent), constants.PermFilePrivate); err != nil {
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
	conn, err := net.DialTimeout(string(constants.NetworkProtocolTCP), hostPort, 5*time.Second)
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
	url := fmt.Sprintf("%s%s", discoveryURL, constants.APIPaths.AuthBootstrapStatus)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	var statusResp models.BootstrapStatusResponse
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	return statusResp.Bootstrapped, nil
}

// parseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func parseCertPEM(fileSvc fs.RuntimeFileService, certFile string) (*x509.Certificate, error) {
	certPEM, err := readFileWithFS(fileSvc, certFile)
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
func CheckCertExpiry(fileSvc fs.RuntimeFileService, certFile string) (bool, error) {
	cert, err := parseCertPEM(fileSvc, certFile)
	if err != nil {
		return false, err
	}

	return isCertExpiringSoon(cert), nil
}

// AutoRenewCertificate performs automatic re-enrollment if the certificate is expiring soon.
// This is a fail-closed operation: if renewal fails, it returns an error rather than falling back.
func AutoRenewCertificate(fileSvc fs.RuntimeFileService, cfg *config.Config, certType string, caFingerprint string) error {
	var certFile string
	switch certType {
	case "cli":
		certFile = cfg.CLICertFile()
	case "operator":
		certFile = cfg.OperatorCertFile()
	default:
		return constants.ErrValidationFailed
	}

	expiringSoon, err := CheckCertExpiry(fileSvc, certFile)
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

	regResp, err := ReEnroll(fileSvc, cfg, "", cliCSR, caFingerprint, "")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return constants.ErrMissingRequiredField
	}

	if err := SaveCertAndKey(fileSvc, regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	if regResp.HubTrustBundle != "" {
		trustPath := cfg.TrustBundlePath()
		if err := writeFileWithFS(fileSvc, trustPath, []byte(regResp.HubTrustBundle), constants.PermFilePublic); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
	}

	creds := &Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := SaveCredentials(fileSvc, cfg, creds); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
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
	url := fmt.Sprintf("http://%s%s", gatewayEndpoint, constants.APIPaths.AuthDeviceEnroll)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)

	// For initial enrollment without mTLS, use plain HTTP client
	client := &http.Client{Timeout: httpTimeout}

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

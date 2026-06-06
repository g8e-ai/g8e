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
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
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

func GenerateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
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
// The fingerprint should be in the format "sha256:<hex>" or just hex.
func VerifyCAFingerprint(caPEM []byte, expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return nil
	}

	// Normalize fingerprint: strip "sha256:" prefix if present
	expectedFP := strings.TrimPrefix(expectedFingerprint, "sha256:")

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

	if actualFP != expectedFP {
		return fmt.Errorf("CA fingerprint mismatch: expected %s, got %s", expectedFP, actualFP)
	}

	return nil
}

func Bootstrap(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string) (*RegistrationResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	req := map[string]string{
		"csr_pem":            operatorCSR,
		"cli_csr_pem":        cliCSR,
		"system_fingerprint": fmt.Sprintf("g8e-cli-%s", hostname),
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

// ReEnroll performs CSR-based re-enrollment using existing mTLS credentials.
// This is used when the platform is already bootstrapped and the CLI has valid certificates.
func ReEnroll(cfg *config.Config, operatorCSR, cliCSR string, caFingerprint string) (*RegistrationResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
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

	req := map[string]string{
		"csr_pem":            operatorCSR,
		"cli_csr_pem":        cliCSR,
		"system_fingerprint": fmt.Sprintf("g8e-cli-%s", hostname),
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

func DeleteCredentials(cfg *config.Config) error {
	credsFile := cfg.CredentialsFile()
	if err := os.Remove(credsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credentials file: %w", err)
	}

	certFiles := []string{
		cfg.CLICertFile(),
		cfg.CLIKeyFile(),
		cfg.OperatorCertFile(),
		cfg.OperatorKeyFile(),
		cfg.TrustBundlePath(),
	}

	for _, file := range certFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", file, err)
		}
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
	// Try to connect to the port
	conn, err := net.Dial(string(constants.NetworkProtocolTCP), hostPort)
	if err != nil {
		return fmt.Errorf("g8e Gateway is not running or not responding at %s: %w", operatorURL, err)
	}
	conn.Close()

	return nil
}

// CheckBootstrapStatus returns whether the platform has been bootstrapped and local credentials exist
func CheckBootstrapStatus(cfg *config.Config) (bool, error) {
	// 1. Check local credential state first
	credsFile := cfg.CredentialsFile()
	if _, err := os.Stat(credsFile); os.IsNotExist(err) {
		return false, nil
	}

	if _, err := os.Stat(cfg.CLICertFile()); os.IsNotExist(err) {
		return false, nil
	}

	// 2. Check remote bootstrap status via bootstrap port (plain HTTP)
	url := fmt.Sprintf("%s/api/v1/auth/bootstrap/status", cfg.OperatorDiscoveryURL())
	resp, err := http.Get(url)
	if err != nil {
		// If Operator is not reachable, we cannot confirm bootstrap status
		return false, fmt.Errorf("failed to check remote bootstrap status: %w", err)
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

	opCSR, opKey, err := GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate Operator CSR: %w", err)
	}

	cliCSR, cliKey, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate CLI CSR: %w", err)
	}

	regResp, err := ReEnroll(cfg, opCSR, cliCSR, caFingerprint)
	if err != nil {
		return fmt.Errorf("automatic re-enrollment failed: %w", err)
	}

	if regResp.OperatorSessionID == "" || regResp.OperatorID == "" || regResp.OperatorCert == "" || regResp.CLISessionID == "" || regResp.CLICert == "" {
		return fmt.Errorf("unexpected re-enrollment response (missing required fields)")
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("failed to save renewed CLI credentials: %w", err)
	}

	if err := SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, opKey, cfg.OperatorCertFile(), cfg.OperatorKeyFile()); err != nil {
		return fmt.Errorf("failed to save renewed Operator credentials: %w", err)
	}

	if regResp.HubTrustBundle != "" {
		hubBundlePath := filepath.Join(cfg.CredentialsDir, "g8eg-ca-bundle.pem")
		if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
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
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	req := map[string]string{
		"csr_pem":            operatorCSR,
		"cli_csr_pem":        cliCSR,
		"system_fingerprint": fmt.Sprintf("g8e-operator-%s", hostname),
		"hostname":           hostname,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use the bootstrap endpoint for initial enrollment (no mTLS required)
	url := fmt.Sprintf("http://%s/api/v1/auth/bootstrap", gatewayEndpoint)
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

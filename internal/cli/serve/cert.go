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

package serve

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
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
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/auth"
)

// ParseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func ParseCertPEM(certFile string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertReadFailed, err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("%w: %s", constants.ErrPEMDecodeFailed, "certificate file")
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: type=%s", constants.ErrInvalidPEMType, block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertParseFailed, err)
	}

	return cert, nil
}

// IsCertExpiringSoon checks if a certificate is expiring within the renewal threshold.
// The threshold is set to 24 hours before expiry to allow ample time for renewal.
func IsCertExpiringSoon(cert *x509.Certificate) bool {
	renewalThreshold := 24 * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry <= renewalThreshold
}

// GenerateCSR generates a new ECDSA P-256 keypair and CSR for the given common name.
func GenerateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

// CertPaths holds the filesystem paths needed by enrollment and renewal functions.
// It decouples these functions from the global paths.Infra singleton.
type CertPaths struct {
	PkiTrustDir       string
	OperatorKeyPath   string
	OperatorCertPath  string
	CaCertPath        string
	TrustedSignersDir string
}

// PerformAutomaticEnrollment handles automatic enrollment with a Gateway when -e flag is provided.
// It fetches the trust bundle, generates a CSR, enrolls with the Gateway, and saves certificates.
// Returns the operator session ID so the caller can set it at the top level.
func PerformAutomaticEnrollment(ctx context.Context, gatewayIP string, certPaths CertPaths, logger *slog.Logger) (sessionID string, err error) {
	// Create PKI directory
	if err := os.MkdirAll(certPaths.PkiTrustDir, 0700); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	// Remove any stale certs so enrollment always issues fresh ones tied to
	// the current gateway PKI (e.g. after gateway restart/regen).
	if err := os.Remove(certPaths.OperatorKeyPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("enrollment: remove stale operator key: %w", err)
	}
	if err := os.Remove(certPaths.OperatorCertPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("enrollment: remove stale operator cert: %w", err)
	}

	// Fetch trust bundle from Gateway HTTP endpoint
	trustURL := fmt.Sprintf("http://%s:%d%s", gatewayIP, constants.Ports.OperatorHttp, constants.WellKnownPKICABundle)
	logger.Info("Fetching trust bundle from Gateway", "url", trustURL)
	trustBundle, err := certs.FetchTrustBundle(ctx, trustURL, "")
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	// Save trust bundle
	if err := os.WriteFile(certPaths.CaCertPath, trustBundle, 0644); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}
	logger.Info("Trust bundle saved", "path", certPaths.CaCertPath)

	// Generate system fingerprint for enrollment
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
	}

	// Generate CSR for enrollment
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}
	opCSR, opKey, err := GenerateCSR(hostname)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Generate CLI CSR (required by device enrollment endpoint even for operator-only deployment)
	cliCSR, _, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Enroll with Gateway
	gatewayEndpoint := fmt.Sprintf("%s:%d", gatewayIP, constants.Ports.OperatorHttp)
	logger.Info("Enrolling with Gateway", "endpoint", gatewayEndpoint)

	// Use the HTTP device enrollment endpoint (not PKI mTLS endpoint)
	enrollURL := fmt.Sprintf("http://%s%s", gatewayEndpoint, constants.APIPathAuthDeviceEnroll)
	reqBody := models.DeviceEnrollRequest{
		CSR:               opCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		Hostname:          hostname,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, enrollURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, resp.StatusCode, string(respBody))
	}

	var enrollResp models.DeviceEnrollmentResponse
	if err := json.Unmarshal(respBody, &enrollResp); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrResponseParseFailed, err)
	}

	if enrollResp.Error != "" {
		return "", fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, enrollResp.Error)
	}

	if enrollResp.OperatorCert == "" {
		return "", fmt.Errorf("%w: operator certificate", constants.ErrMissingCertificate)
	}

	// Save operator private key
	keyBytes, err := x509.MarshalECPrivateKey(opKey)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
	logger.Info("Saving operator private key", "path", certPaths.OperatorKeyPath)
	if err := os.WriteFile(certPaths.OperatorKeyPath, keyPEM, 0600); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyWriteFailed, err)
	}
	logger.Info("Operator private key saved successfully")

	// Save operator certificate
	certContent := enrollResp.OperatorCert
	if enrollResp.OperatorCertChain != "" {
		certContent += "\n" + enrollResp.OperatorCertChain
	}
	logger.Info("Saving operator certificate", "path", certPaths.OperatorCertPath)
	if err := os.WriteFile(certPaths.OperatorCertPath, []byte(certContent), 0600); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	logger.Info("Operator certificate saved successfully")

	// Update trust bundle if Gateway returned a new one
	if enrollResp.HubTrustBundle != "" {
		if err := os.WriteFile(certPaths.CaCertPath, []byte(enrollResp.HubTrustBundle), 0644); err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
		logger.Info("Updated trust bundle from Gateway")
	}

	// Save Actuator public key to trusted_signers so the operator can verify L2 signatures.
	if enrollResp.ActuatorKeyID != "" && enrollResp.ActuatorPubKey != "" {
		if err := os.MkdirAll(certPaths.TrustedSignersDir, 0700); err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		signerPath := filepath.Join(certPaths.TrustedSignersDir, enrollResp.ActuatorKeyID+constants.PublicKeySuffix)
		if err := os.WriteFile(signerPath, []byte(enrollResp.ActuatorPubKey), 0600); err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		logger.Info("Actuator public key saved", "path", signerPath)
	}

	logger.Info("Enrollment successful", "operator_id", enrollResp.OperatorID, "operator_session_id", enrollResp.OperatorSessionID)

	return enrollResp.OperatorSessionID, nil
}

// checkCertExpiry parses the cert file and returns true if it is expiring within 24h.
func checkCertExpiry(certFile string) (bool, error) {
	cert, err := ParseCertPEM(certFile)
	if err != nil {
		return false, err
	}
	return IsCertExpiringSoon(cert), nil
}

// fetchAndSaveTrustBundle fetches the trust bundle from the given endpoint URL,
// saves it to caCertPath, and returns the PEM bytes.
func fetchAndSaveTrustBundle(ctx context.Context, trustBundleURL, caCertPath string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trustBundleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("%w: trust bundle response body was empty", constants.ErrEmptyTrustBundle)
	}

	if err := os.WriteFile(caCertPath, body, 0644); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}

	return body, nil
}

// buildMTLSClient constructs an http.Client configured for mTLS using the given
// CA PEM and client certificate.
func buildMTLSClient(caPEM []byte, cliCert tls.Certificate) (*http.Client, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("%w: failed to parse CA certificates from trust bundle", constants.ErrCAParseFailed)
	}

	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cliCert},
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{Transport: transport}, nil
}

// submitRenewal POSTs a re-enrollment request and returns the parsed response.
func submitRenewal(ctx context.Context, client *http.Client, enrollURL, opCSR, cliCSR, hostname string) (*models.OperatorRegistrationResponse, error) {
	reqBody := models.DeviceEnrollRequest{
		CSR:               opCSR,
		CLICSR:            cliCSR,
		SystemFingerprint: fmt.Sprintf("g8e-operator-%s", hostname),
		Hostname:          hostname,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, enrollURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, resp.StatusCode, string(respBody))
	}

	var regResp models.OperatorRegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrResponseParseFailed, err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	if regResp.OperatorCert == "" || regResp.CLICert == "" {
		return nil, fmt.Errorf("%w: re-enrollment response missing certificates", constants.ErrMissingRequiredField)
	}

	return &regResp, nil
}

// saveRenewedCerts writes the renewed operator cert and key to disk and returns
// the PEM-encoded key and cert content for in-memory reload.
func saveRenewedCerts(certFile, keyFile string, certContent string, opKey *ecdsa.PrivateKey) (keyPEM []byte, err error) {
	keyBytes, err := x509.MarshalECPrivateKey(opKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyWriteFailed, err)
	}

	if err := os.WriteFile(certFile, []byte(certContent), 0600); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	return keyPEM, nil
}

// RenewOperatorCertificate performs automatic re-enrollment for the Operator certificate.
// This is a fail-closed operation: if renewal fails, it returns an error.
func RenewOperatorCertificate(ctx context.Context, cfg *config.Config, clientCertFile, clientKeyFile string, clientIdentity *certs.ClientIdentity) error {
	expiringSoon, err := checkCertExpiry(clientCertFile)
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

	opCSR, opKey, err := GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	cliCSR, _, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Load existing CLI certificate for mTLS
	cliCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Fetch and save trust bundle using a timeout client
	trustBundleURL := fmt.Sprintf("%s%s", cfg.Endpoint, constants.WellKnownPKICABundle)
	caPEM, err := fetchAndSaveTrustBundle(ctx, trustBundleURL, paths.Infra.CaCertPath)
	if err != nil {
		return err
	}

	// Build mTLS client
	client, err := buildMTLSClient(caPEM, cliCert)
	if err != nil {
		return err
	}

	// Submit re-enrollment request
	enrollURL := fmt.Sprintf("%s%s", cfg.Endpoint, constants.APIPathPKIDevicesEnroll)
	regResp, err := submitRenewal(ctx, client, enrollURL, opCSR, cliCSR, hostname)
	if err != nil {
		return err
	}

	// Save renewed certificates
	certContent := regResp.OperatorCert
	if regResp.OperatorCertChain != "" {
		certContent += "\n" + regResp.OperatorCertChain
	}

	keyPEM, err := saveRenewedCerts(clientCertFile, clientKeyFile, certContent, opKey)
	if err != nil {
		return err
	}

	// Update the client certificate via DI
	newCert, err := tls.X509KeyPair([]byte(certContent), keyPEM)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	clientIdentity.SetCertificate(newCert)

	return nil
}

// RunClientCertRenewalLoop runs a background goroutine that periodically checks
// and renews the client certificate if it is expiring soon.
func RunClientCertRenewalLoop(ctx context.Context, cfg *config.Config, clientCertFile, clientKeyFile string, logger *slog.Logger, clientIdentity *certs.ClientIdentity) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Check immediately on startup
	if err := RenewOperatorCertificate(ctx, cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
		logger.Error("Failed to renew client certificate on startup", string(constants.ConnectionStateError), err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Client certificate renewal loop stopped")
			return
		case <-ticker.C:
			if err := RenewOperatorCertificate(ctx, cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
				logger.Error("Failed to renew client certificate", string(constants.ConnectionStateError), err)
			} else {
				logger.Info("Client certificate renewal check completed")
			}
		}
	}
}

// LoadTrustBundle attempts to read a trust bundle from:
// 1. Explicit path provided via --trust-bundle
// 2. Local PKI path (from paths.Infra.CaCertPath)
// Returns true on the first valid PEM found, which is installed via
// trustStore.SetCA. Returns false if no valid trust bundle is found.
func LoadTrustBundle(logger *slog.Logger, explicitPath string, trustStore *certs.TrustStore) bool {
	pathsToCheck := []string{}

	if explicitPath != "" {
		pathsToCheck = append(pathsToCheck, explicitPath)
	}

	pathsToCheck = append(pathsToCheck, paths.Infra.CaCertPath)

	for _, path := range pathsToCheck {
		pemData, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		logger.Info("Loading trust bundle from local path", "path", path, "bytes", len(pemData))
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			logger.Warn("CA file exists but contains invalid certificate", "path", path)
			continue
		}
		LogCertBundle(logger, "trust-bundle", pemData)
		trustStore.SetCA(pemData)
		logger.Info("CA certificate loaded from local file")
		return true
	}
	return false
}

// LogCertBundle parses every PEM certificate in pemData and logs its details.
func LogCertBundle(logger *slog.Logger, label string, pemData []byte) {
	rest := pemData
	idx := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			logger.Info("[TLS-DEBUG] non-cert PEM block in bundle", "label", label, "type", block.Type)
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			logger.Warn("[TLS-DEBUG] failed to parse cert in bundle", "label", label, "idx", idx, "error", err)
			idx++
			continue
		}
		fp := sha256.Sum256(block.Bytes)
		logger.Info("[TLS-DEBUG] bundle cert",
			"label", label,
			"idx", idx,
			"subject", cert.Subject.String(),
			"issuer", cert.Issuer.String(),
			"serial", cert.SerialNumber.String(),
			"not_before", cert.NotBefore.Format(time.RFC3339),
			"not_after", cert.NotAfter.Format(time.RFC3339),
			"is_ca", cert.IsCA,
			"key_algo", cert.PublicKeyAlgorithm.String(),
			"sig_algo", cert.SignatureAlgorithm.String(),
			"sha256", hex.EncodeToString(fp[:]),
		)
		idx++
	}
	logger.Info("[TLS-DEBUG] bundle parsed", "label", label, "cert_count", idx)
}

// ExportActuatorPublicKey writes the Actuator's public key to both PEM and JSON formats
// in the PKI directory for receipt verification by the evals harness.
func ExportActuatorPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if pkiDir == "" {
		return constants.ErrPKIDirRequired
	}
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	// Write PEM format
	pemPath := filepath.Join(pkiDir, constants.ActuatorPubPEMFilename)
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKey,
	})
	if err := os.WriteFile(pemPath, pemData, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", pemPath, "format", "PEM")
	}

	// Write JSON format
	jsonPath := filepath.Join(pkiDir, constants.ActuatorPubJSONFilename)
	jsonData := models.ActuatorPublicKeyExport{
		KeyID:     keyID,
		PublicKey: hex.EncodeToString(pubKey),
		Algorithm: "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrJSONMarshalFailed, err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", jsonPath, "format", "JSON")
	}

	return nil
}

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

package main

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
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/auth"
)

// parseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func parseCertPEM(certFile string) (*x509.Certificate, error) {
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

// isCertExpiringSoon checks if a certificate is expiring within the renewal threshold.
// The threshold is set to 24 hours before expiry to allow ample time for renewal.
func isCertExpiringSoon(cert *x509.Certificate) bool {
	renewalThreshold := 24 * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry <= renewalThreshold
}

// generateCSR generates a new ECDSA P-256 keypair and CSR for the given common name.
func generateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
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

// performAutomaticEnrollment handles automatic enrollment with a Gateway when -e flag is provided.
// It fetches the trust bundle, generates a CSR, enrolls with the Gateway, and saves certificates.
func performAutomaticEnrollment(gatewayIP, workDir string, logger *slog.Logger) error {
	// Create PKI directory
	if err := os.MkdirAll(paths.Infra.PkiTrustDir, 0700); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	// Remove any stale certs so enrollment always issues fresh ones tied to
	// the current gateway PKI (e.g. after gateway restart/regen).
	_ = os.Remove(paths.Infra.OperatorKeyPath)
	_ = os.Remove(paths.Infra.OperatorCertPath)

	// Fetch trust bundle from Gateway HTTP endpoint
	trustURL := fmt.Sprintf("http://%s:%d%s", gatewayIP, constants.Ports.OperatorHttp, constants.WellKnownPKICABundle)
	logger.Info("Fetching trust bundle from Gateway", "url", trustURL)
	trustBundle, err := certs.FetchTrustBundle(context.Background(), trustURL, "")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	// Save trust bundle
	if err := os.WriteFile(paths.Infra.CaCertPath, trustBundle, 0644); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}
	logger.Info("Trust bundle saved", "path", paths.Infra.CaCertPath)

	// Generate system fingerprint for enrollment
	systemFp, err := auth.GenerateSystemFingerprint(logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
	}

	// Generate CSR for enrollment
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}
	opCSR, opKey, err := generateCSR(hostname)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Generate CLI CSR (required by device enrollment endpoint even for operator-only deployment)
	cliCSR, _, err := generateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Enroll with Gateway
	gatewayEndpoint := fmt.Sprintf("%s:%d", gatewayIP, constants.Ports.OperatorHttp)
	logger.Info("Enrolling with Gateway", "endpoint", gatewayEndpoint)

	// Use the HTTP device enrollment endpoint (not PKI mTLS endpoint)
	enrollURL := fmt.Sprintf("http://%s%s", gatewayEndpoint, constants.APIPathAuthDeviceEnroll)
	reqBody := struct {
		CSRPEM            string `json:"csr_pem"`
		CLICSRPEM         string `json:"cli_csr_pem"`
		SystemFingerprint string `json:"system_fingerprint"`
		Hostname          string `json:"hostname"`
	}{
		CSRPEM:            opCSR,
		CLICSRPEM:         cliCSR,
		SystemFingerprint: systemFp.Fingerprint,
		Hostname:          hostname,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	httpReq, err := http.NewRequest("POST", enrollURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, resp.StatusCode, string(respBody))
	}

	var enrollResp struct {
		OperatorCert      string `json:"operator_cert"`
		OperatorCertChain string `json:"operator_cert_chain,omitempty"`
		HubTrustBundle    string `json:"hub_trust_bundle,omitempty"`
		OperatorID        string `json:"operator_id"`
		OperatorSessionID string `json:"operator_session_id"`
		ActuatorKeyID     string `json:"actuator_key_id,omitempty"`
		ActuatorPubKey    string `json:"actuator_pub_key,omitempty"`
		Error             string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &enrollResp); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrResponseParseFailed, err)
	}

	if enrollResp.Error != "" {
		return fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, enrollResp.Error)
	}

	if enrollResp.OperatorCert == "" {
		return fmt.Errorf("%w: operator certificate", constants.ErrMissingCertificate)
	}

	// Save operator private key
	keyBytes, err := x509.MarshalECPrivateKey(opKey)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
	logger.Info("Saving operator private key", "path", paths.Infra.OperatorKeyPath)
	if err := os.WriteFile(paths.Infra.OperatorKeyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyReadFailed, err)
	}
	logger.Info("Operator private key saved successfully")

	// Save operator certificate
	certContent := enrollResp.OperatorCert
	if enrollResp.OperatorCertChain != "" {
		certContent += "\n" + enrollResp.OperatorCertChain
	}
	logger.Info("Saving operator certificate", "path", paths.Infra.OperatorCertPath)
	if err := os.WriteFile(paths.Infra.OperatorCertPath, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	logger.Info("Operator certificate saved successfully")

	// Update trust bundle if Gateway returned a new one
	if enrollResp.HubTrustBundle != "" {
		if err := os.WriteFile(paths.Infra.CaCertPath, []byte(enrollResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
		logger.Info("Updated trust bundle from Gateway")
	}

	// Save Actuator public key to trusted_signers so the operator can verify L2 signatures.
	if enrollResp.ActuatorKeyID != "" && enrollResp.ActuatorPubKey != "" {
		if err := os.MkdirAll(paths.Infra.TrustedSignersDir, 0700); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		signerPath := filepath.Join(paths.Infra.TrustedSignersDir, enrollResp.ActuatorKeyID+constants.PublicKeySuffix)
		if err := os.WriteFile(signerPath, []byte(enrollResp.ActuatorPubKey), 0600); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		logger.Info("Actuator public key saved", "path", signerPath)
	}

	logger.Info("Enrollment successful", "operator_id", enrollResp.OperatorID, "operator_session_id", enrollResp.OperatorSessionID)

	// Set environment variable for operator session ID
	os.Setenv("G8E_OPERATOR_SESSION_ID", enrollResp.OperatorSessionID)

	return nil
}

// renewOperatorCertificate performs automatic re-enrollment for the Operator certificate.
// This is a fail-closed operation: if renewal fails, it returns an error.
func renewOperatorCertificate(cfg *config.Config, clientCertFile, clientKeyFile string, clientIdentity *certs.ClientIdentity) error {
	expiringSoon, err := func() (bool, error) {
		cert, err := parseCertPEM(clientCertFile)
		if err != nil {
			return false, err
		}
		return isCertExpiringSoon(cert), nil
	}()
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

	opCSR, opKey, err := generateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	cliCSR, _, err := generateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	// Load existing CLI certificate for mTLS
	cliCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Fetch current trust bundle from operator
	trustBundleURL := fmt.Sprintf("%s%s", cfg.Endpoint, constants.WellKnownPKICABundle)
	trustBundleResp, err := http.Get(trustBundleURL)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	defer trustBundleResp.Body.Close()

	if trustBundleResp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", constants.ErrHTTPStatusError, trustBundleResp.StatusCode)
	}

	currentTrustBundle, err := io.ReadAll(trustBundleResp.Body)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if len(currentTrustBundle) == 0 {
		return fmt.Errorf("%w", constants.ErrEmptyTrustBundle)
	}

	// Update local trust bundle
	trustBundlePath := paths.Infra.CaCertPath
	if err := os.WriteFile(trustBundlePath, currentTrustBundle, 0644); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}

	// Create mTLS client
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(currentTrustBundle) {
		return fmt.Errorf("%w", constants.ErrCAParseFailed)
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

	// Submit re-enrollment request
	reqBody := map[string]string{
		"csr_pem":            opCSR,
		"cli_csr_pem":        cliCSR,
		"system_fingerprint": fmt.Sprintf("g8e-operator-%s", hostname),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	enrollURL := fmt.Sprintf("%s%s", cfg.Endpoint, constants.APIPathPKIDevicesEnroll)
	httpReq, err := http.NewRequest("POST", enrollURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, resp.StatusCode, string(respBody))
	}

	var regResp struct {
		OperatorCert      string `json:"operator_cert"`
		OperatorCertChain string `json:"operator_cert_chain,omitempty"`
		CLICert           string `json:"cli_cert"`
		CLICertChain      string `json:"cli_cert_chain,omitempty"`
		HubTrustBundle    string `json:"hub_trust_bundle,omitempty"`
		Error             string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrResponseParseFailed, err)
	}

	if regResp.Error != "" {
		return fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, regResp.Error)
	}

	if regResp.OperatorCert == "" || regResp.CLICert == "" {
		return fmt.Errorf("%w: re-enrollment response", constants.ErrMissingRequiredField)
	}

	// Save renewed certificates
	keyBytes, err := x509.MarshalECPrivateKey(opKey)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(clientKeyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyReadFailed, err)
	}

	certContent := regResp.OperatorCert
	if regResp.OperatorCertChain != "" {
		certContent += "\n" + regResp.OperatorCertChain
	}

	if err := os.WriteFile(clientCertFile, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	// Update the client certificate via DI
	newCert, err := tls.X509KeyPair([]byte(certContent), keyPEM)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	clientIdentity.SetCertificate(newCert)

	return nil
}

// runClientCertRenewalLoop runs a background goroutine that periodically checks
// and renews the client certificate if it is expiring soon.
func runClientCertRenewalLoop(ctx context.Context, cfg *config.Config, clientCertFile, clientKeyFile string, logger *slog.Logger, clientIdentity *certs.ClientIdentity) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Check immediately on startup
	if err := renewOperatorCertificate(cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
		logger.Error("Failed to renew client certificate on startup", string(constants.ConnectionStateError), err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Client certificate renewal loop stopped")
			return
		case <-ticker.C:
			if err := renewOperatorCertificate(cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
				logger.Error("Failed to renew client certificate", string(constants.ConnectionStateError), err)
			} else {
				logger.Info("Client certificate renewal check completed")
			}
		}
	}
}

// loadTrustBundle attempts to read a trust bundle from:
// 1. Explicit path provided via --trust-bundle
// 2. Local PKI path (from paths.Infra.CaCertPath)
// Returns true on the first valid PEM found, which is installed via
// trustStore.SetCA. Returns false if no valid trust bundle is found.
func loadTrustBundle(logger *slog.Logger, explicitPath string, trustStore *certs.TrustStore) bool {
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
		logCertBundle(logger, "trust-bundle", pemData)
		trustStore.SetCA(pemData)
		logger.Info("CA certificate loaded from local file")
		return true
	}
	return false
}

// logCertBundle parses every PEM certificate in pemData and logs its details.
func logCertBundle(logger *slog.Logger, label string, pemData []byte) {
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

// exportActuatorPublicKey writes the Actuator's public key to both PEM and JSON formats
// in the PKI directory for receipt verification by the evals harness.
func exportActuatorPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
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
	jsonData := map[string]string{
		"key_id":     keyID,
		"public_key": hex.EncodeToString(pubKey),
		"algorithm":  "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}
	// Ensure the directory for the JSON file exists
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0700); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0600); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", jsonPath, "format", "JSON")
	}

	return nil
}

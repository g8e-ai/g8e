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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/protocol"
)

const (
	rootValidityDays         = 3650
	intermediateValidityDays = 3650
	serviceValidityDays      = 1 // 1-day TTL for operator mTLS certificates

	rootCommonName     = "g8e Root CA"
	hubCommonName      = "g8e Hub Intermediate CA"
	operatorCommonName = "g8e Operator Intermediate CA"
)

// PKIAuthority manages the full PKI hierarchy for the Operator.
// It generates and stores:
// - Root CA under pki/root/ (cert only, key in keystore)
// - Intermediate CAs under pki/authorities/ (cert only, keys in keystore)
// - Service certificates under pki/issued/
// - Trust bundles under pki/trust/
// - Revocation state under pki/revocation/
type PKIAuthority struct {
	mu     sync.RWMutex
	logger *slog.Logger
	db     *GatewayDBService

	pkiDir        string
	secretManager *SecretManager

	// Root CA
	rootCert *x509.Certificate
	rootKey  *ecdsa.PrivateKey

	// Intermediate CAs
	hubCert      *x509.Certificate
	hubKey       *ecdsa.PrivateKey
	operatorCert *x509.Certificate
	operatorKey  *ecdsa.PrivateKey

	// Service certificate for operator-gateway
	serviceCert tls.Certificate
}

func newPKIAuthority(dataDir, pkiDir string, db *GatewayDBService, secretManager *SecretManager, logger *slog.Logger) *PKIAuthority {
	if pkiDir == "" {
		pkiDir = filepath.Join(dataDir, "pki")
	}
	return &PKIAuthority{
		pkiDir:        pkiDir,
		db:            db,
		secretManager: secretManager,
		logger:        logger,
	}
}

// EnsurePKI initializes the full PKI hierarchy. Must be called before TLSConfig().
func (pki *PKIAuthority) EnsurePKI(extraIPs []net.IP) error {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	// Create directory structure
	dirs := []string{
		filepath.Join(pki.pkiDir, "root"),
		filepath.Join(pki.pkiDir, "authorities"),
		filepath.Join(pki.pkiDir, "issued", "hub"),
		filepath.Join(pki.pkiDir, "issued", "apps"),
		filepath.Join(pki.pkiDir, "trust"),
		filepath.Join(pki.pkiDir, "revocation"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create PKI directory %s: %w", dir, err)
		}
	}

	// Generate or load Root CA
	if err := pki.ensureRootCA(); err != nil {
		return fmt.Errorf("root CA setup failed: %w", err)
	}

	// Generate or load Intermediate CAs
	if err := pki.ensureIntermediateCAs(); err != nil {
		return fmt.Errorf("intermediate CA setup failed: %w", err)
	}

	// Generate or load operator-gateway service certificate
	if err := pki.ensureServiceCert(extraIPs); err != nil {
		return fmt.Errorf("service certificate setup failed: %w", err)
	}

	// Generate or load certificates for reference ensembles
	if err := pki.ensureAppCerts(); err != nil {
		return fmt.Errorf("app certificates setup failed: %w", err)
	}

	// Generate trust bundles
	if err := pki.generateTrustBundles(); err != nil {
		return fmt.Errorf("trust bundle generation failed: %w", err)
	}

	pki.logger.Info("[PKI] PKI hierarchy initialized", "pki_dir", pki.pkiDir)
	return nil
}

// TLSConfig returns a tls.Config that serves the managed server certificate.
// It also configures the client CA pool to enable mTLS verification.
func (pki *PKIAuthority) TLSConfig() *tls.Config {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	// Create client CA pool from our root and hub authorities
	pool := x509.NewCertPool()
	if pki.rootCert != nil {
		pool.AddCert(pki.rootCert)
	}
	if pki.hubCert != nil {
		pool.AddCert(pki.hubCert)
	}
	if pki.operatorCert != nil {
		pool.AddCert(pki.operatorCert)
	}

	return &tls.Config{
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			pki.mu.RLock()
			defer pki.mu.RUnlock()
			c := pki.serviceCert
			return &c, nil
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		MinVersion: tls.VersionTLS13,
	}
}

// TrustBundlePath returns the path to the hub trust bundle.
func (pki *PKIAuthority) TrustBundlePath() string {
	return filepath.Join(pki.pkiDir, "trust", "g8e-gw-ca-bundle.pem")
}

// PKIDir returns the path to the pki directory.
func (pki *PKIAuthority) PKIDir() string {
	return pki.pkiDir
}

// ─── PKI hierarchy management ─────────────────────────────────────────────

func (pki *PKIAuthority) ensureRootCA() error {
	rootCertPath := filepath.Join(pki.pkiDir, "root", "root_ca.crt")

	if fileExists(rootCertPath) {
		if err := pki.loadCACertificate(rootCertPath, &pki.rootCert); err != nil {
			return fmt.Errorf("root CA exists but is corrupt: %w", err)
		}
		return nil
	}

	pki.logger.Info("[PKI] Generating root CA")
	return pki.generateRootCA("", rootCertPath)
}

func (pki *PKIAuthority) ensureIntermediateCAs() error {
	// Hub Intermediate CA
	hubCertPath := filepath.Join(pki.pkiDir, "authorities", "hub_ca.crt")
	if fileExists(hubCertPath) {
		if err := pki.loadCACertificate(hubCertPath, &pki.hubCert); err != nil {
			return fmt.Errorf("hub CA exists but is corrupt: %w", err)
		}
	} else {
		pki.logger.Info("[PKI] Generating hub intermediate CA")
		if err := pki.loadCAPrivateKey("root", &pki.rootKey); err != nil {
			return fmt.Errorf("load root CA private key for intermediate generation: %w", err)
		}
		if err := pki.generateIntermediateCA("", hubCertPath, pki.rootCert, pki.rootKey, hubCommonName); err != nil {
			return err
		}
	}

	// Operator Intermediate CA
	operatorCertPath := filepath.Join(pki.pkiDir, "authorities", "operator_ca.crt")
	if fileExists(operatorCertPath) {
		if err := pki.loadCACertificate(operatorCertPath, &pki.operatorCert); err != nil {
			return fmt.Errorf("operator CA exists but is corrupt: %w", err)
		}
	} else {
		pki.logger.Info("[PKI] Generating operator intermediate CA")
		if err := pki.loadCAPrivateKey("root", &pki.rootKey); err != nil {
			return fmt.Errorf("load root CA private key for intermediate generation: %w", err)
		}
		if err := pki.generateIntermediateCA("", operatorCertPath, pki.rootCert, pki.rootKey, operatorCommonName); err != nil {
			return err
		}
	}

	return nil
}

func (pki *PKIAuthority) ensureServiceCert(extraIPs []net.IP) error {
	serviceCertPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-gateway.crt")
	chainPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-gateway.chain.pem")

	needService := !fileExists(serviceCertPath) || !fileExists(chainPath)
	if !needService {
		// Load certificate chain from file
		chainPEM, err := os.ReadFile(chainPath)
		if err != nil {
			pki.logger.Warn("[PKI] Failed to load service cert chain, regenerating", string(constants.ConnectionStateError), err)
			needService = true
		} else {
			// Load private key from keystore
			if pki.secretManager == nil {
				return fmt.Errorf("SecretManager is required for service private key loading")
			}
			keyDER, err := pki.secretManager.GetServicePrivateKey("operator-gateway")
			if err != nil {
				pki.logger.Warn("[PKI] Failed to load service private key from keystore, regenerating", string(constants.ConnectionStateError), err)
				needService = true
			} else {
				// Validate the key can be parsed
				if _, err := x509.ParseECPrivateKey(keyDER); err != nil {
					pki.logger.Warn("[PKI] Failed to parse service private key, regenerating", string(constants.ConnectionStateError), err)
					needService = true
				} else {
					cert, err := tls.X509KeyPair(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
					if err != nil {
						pki.logger.Warn("[PKI] Failed to construct service certificate, regenerating", string(constants.ConnectionStateError), err)
						needService = true
					} else {
						if isExpiringSoon(cert) {
							pki.logger.Info("[PKI] Service certificate expiring soon, renewing")
							needService = true
						} else {
							pki.serviceCert = cert
						}
					}
				}
			}
		}
	}

	if needService {
		pki.logger.Info("[PKI] Generating operator-gateway service certificate")
		// Load hub CA private key on-demand for service cert generation
		if pki.hubKey == nil {
			if err := pki.loadCAPrivateKey("hub", &pki.hubKey); err != nil {
				return fmt.Errorf("load hub CA private key for service cert generation: %w", err)
			}
		}
		if err := pki.generateServiceCert(extraIPs); err != nil {
			return err
		}
		// Load the newly generated certificate and key
		chainPEM, err := os.ReadFile(chainPath)
		if err != nil {
			return fmt.Errorf("failed to load generated service cert chain: %w", err)
		}
		keyDER, err := pki.secretManager.GetServicePrivateKey("operator-gateway")
		if err != nil {
			return fmt.Errorf("failed to load generated service private key from keystore: %w", err)
		}
		tlsCert, err := tls.X509KeyPair(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
		if err != nil {
			return fmt.Errorf("failed to construct service certificate: %w", err)
		}
		pki.serviceCert = tlsCert
	}

	return nil
}

func (pki *PKIAuthority) generateTrustBundles() error {
	// Root bundle (just root CA)
	rootBundlePath := filepath.Join(pki.pkiDir, "trust", "root.pem")
	rootPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "root", "root_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read root CA: %w", err)
	}
	if err := os.WriteFile(rootBundlePath, rootPEM, 0644); err != nil {
		return fmt.Errorf("failed to write root bundle: %w", err)
	}

	// Hub bundle (root + hub intermediate + operator intermediate)
	hubBundlePath := filepath.Join(pki.pkiDir, "trust", "g8e-gw-ca-bundle.pem")
	hubPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "hub_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read hub CA: %w", err)
	}
	operatorPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "operator_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read operator CA: %w", err)
	}
	hubBundle := make([]byte, 0, len(rootPEM)+len(hubPEM)+len(operatorPEM))
	hubBundle = append(hubBundle, rootPEM...)
	hubBundle = append(hubBundle, hubPEM...)
	hubBundle = append(hubBundle, operatorPEM...)
	if err := os.WriteFile(hubBundlePath, hubBundle, 0644); err != nil {
		return fmt.Errorf("failed to write hub bundle: %w", err)
	}

	// Operator bundle (root + operator intermediate)
	operatorBundlePath := filepath.Join(pki.pkiDir, "trust", "operator-bundle.pem")
	operatorBundle := make([]byte, 0, len(rootPEM)+len(operatorPEM))
	operatorBundle = append(operatorBundle, rootPEM...)
	operatorBundle = append(operatorBundle, operatorPEM...)
	if err := os.WriteFile(operatorBundlePath, operatorBundle, 0644); err != nil {
		return fmt.Errorf("failed to write operator bundle: %w", err)
	}

	// Trust domain metadata
	trustDomainData := map[string]string{
		"trust_domain": protocol.TrustDomain,
	}
	trustDomainJSON, _ := json.MarshalIndent(trustDomainData, "", "  ")
	if err := os.WriteFile(filepath.Join(pki.pkiDir, "trust", "trust-domain.json"), trustDomainJSON, 0600); err != nil {
		return fmt.Errorf("failed to write trust-domain.json: %w", err)
	}

	return nil
}

// HubTrustBundle returns the full PEM-encoded hub trust bundle (root + hub intermediate).
func (pki *PKIAuthority) HubTrustBundle() ([]byte, error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()
	return os.ReadFile(filepath.Join(pki.pkiDir, "trust", "g8e-gw-ca-bundle.pem"))
}

// RevokeCertificate adds a certificate serial to the revocation list.
func (pki *PKIAuthority) RevokeCertificate(serial string, reason string) error {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	if pki.db == nil {
		return fmt.Errorf("database not available")
	}

	doc := map[string]interface{}{
		"serial":     serial,
		"reason":     reason,
		"revoked_at": time.Now().UTC(),
	}
	body, _ := json.Marshal(doc)

	return pki.db.DocSet(marshaler.CollectionName(constants.CollectionRevokedCertificates), serial, body)
}

// GenerateRevocationBundle creates a signed JSON bundle of all revoked certificate serials.
func (pki *PKIAuthority) GenerateRevocationBundle() (bundleJSON string, signature string, err error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	if pki.db == nil {
		return "", "", fmt.Errorf("database not available")
	}

	docs, err := pki.db.DocQuery(marshaler.CollectionName(constants.CollectionRevokedCertificates), nil, "revoked_at", 0)
	if err != nil {
		return "", "", err
	}

	revoked := make([]string, 0, len(docs))
	for _, doc := range docs {
		revoked = append(revoked, doc.ID)
	}

	bundle := map[string]interface{}{
		"revoked_serials": revoked,
		"generated_at":    time.Now().UTC(),
		"trust_domain":    protocol.TrustDomain,
	}

	bundleBytes, err := json.Marshal(bundle)
	if err != nil {
		return "", "", err
	}

	// Sign the bundle with the hub intermediate key
	sig, err := pki.signData(bundleBytes, pki.hubKey)
	if err != nil {
		return "", "", err
	}

	return string(bundleBytes), sig, nil
}

// IsRevoked checks if a certificate serial is in the revocation list.
func (pki *PKIAuthority) IsRevoked(serial string) (bool, error) {
	if pki.db == nil {
		return false, fmt.Errorf("database not available")
	}

	doc, err := pki.db.DocGet(marshaler.CollectionName(constants.CollectionRevokedCertificates), serial)
	if err != nil {
		return false, err
	}

	return doc != nil, nil
}

// VerifyCertificate checks if a certificate is valid and not revoked.
func (pki *PKIAuthority) VerifyCertificate(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("no certificate provided")
	}

	revoked, err := pki.IsRevoked(cert.SerialNumber.String())
	if err != nil {
		return fmt.Errorf("failed to check revocation status: %w", err)
	}

	if revoked {
		return fmt.Errorf("certificate is revoked")
	}

	return nil
}

func (pki *PKIAuthority) signData(data []byte, key *ecdsa.PrivateKey) (string, error) {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}

	sig := append(r.Bytes(), s.Bytes()...)
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// SignCSR signs a certificate signing request using the operator intermediate CA.
// leafType should be "operator", "app", or "cli".
// Parameters:
//   - For "operator": organizationID, operatorID, sessionID (operator_session_id)
//   - For "cli": userID, sessionID (cli_session_id)
//   - For "app": operatorID (app identity)
func (pki *PKIAuthority) SignCSR(csrPEM string, leafType string, organizationID, operatorID, userID, sessionID string) (certPEM, chainPEM string, err error) {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	if pki.operatorCert == nil {
		return "", "", fmt.Errorf("operator CA not loaded - call EnsurePKI first")
	}

	// Load operator CA private key on-demand for signing
	if pki.operatorKey == nil {
		if err := pki.loadCAPrivateKey(string(constants.UserRoleOperator), &pki.operatorKey); err != nil {
			return "", "", fmt.Errorf("load operator CA private key for signing: %w", err)
		}
	}

	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", "", fmt.Errorf("invalid CSR PEM")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse CSR: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return "", "", fmt.Errorf("CSR signature check failed: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return "", "", err
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    now.Add(-1 * time.Minute),
		NotAfter:     now.Add(time.Duration(serviceValidityDays) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}

	// Set URI SAN for workload identity
	wid := protocol.NewWorkloadIdentity()
	var uriURL *url.URL
	switch leafType {
	case string(constants.SessionTypeOperator):
		uriURL, _ = wid.OperatorSPIFFEURL(organizationID, operatorID, sessionID)
	case string(constants.SessionTypeCLI):
		uriURL, _ = wid.CLISPIFFEURL(userID, sessionID)
	case "app":
		uriURL, _ = wid.AppSPIFFEURL(operatorID)
	}

	if uriURL != nil {
		template.URIs = []*url.URL{uriURL}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, pki.operatorCert, csr.PublicKey, pki.operatorKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign certificate: %w", err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	// Build chain (leaf + operator intermediate + root)
	opPEM, _ := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "operator_ca.crt"))
	rootPEM, _ := os.ReadFile(filepath.Join(pki.pkiDir, "root", "root_ca.crt"))
	chainPEM = certPEM + string(opPEM) + string(rootPEM)

	return certPEM, chainPEM, nil
}

// ─── private helpers ──────────────────────────────────────────────────────────

func (pki *PKIAuthority) loadCACertificate(certPath string, cert **x509.Certificate) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("invalid cert PEM")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	*cert = parsedCert
	return nil
}

func (pki *PKIAuthority) loadCAPrivateKey(caType string, key **ecdsa.PrivateKey) error {
	if pki.secretManager == nil {
		return fmt.Errorf("SecretManager is required for CA private key loading")
	}

	keyDER, err := pki.secretManager.GetCAPrivateKey(caType)
	if err != nil {
		return fmt.Errorf("load %s CA private key from keystore: %w", caType, err)
	}

	parsedKey, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return fmt.Errorf("parse %s CA private key: %w", caType, err)
	}

	*key = parsedKey
	return nil
}

func (pki *PKIAuthority) generateRootCA(keyPath, certPath string) error {
	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   rootCommonName,
			Organization: []string{"g8e"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(rootValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2, // Root -> Intermediate -> Leaf
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &rootKey.PublicKey, rootKey)
	if err != nil {
		return err
	}

	rootCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	if err := writePEMFile(certPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(rootKey)
	if err != nil {
		return err
	}
	if pki.secretManager == nil {
		return fmt.Errorf("SecretManager is required for PKI private key storage")
	}
	if err := pki.secretManager.StoreCAPrivateKey("root", keyDER); err != nil {
		return fmt.Errorf("store root CA private key in keystore: %w", err)
	}

	pki.rootCert = rootCert
	pki.rootKey = rootKey
	return nil
}

func (pki *PKIAuthority) generateIntermediateCA(keyPath, certPath string, parentCert *x509.Certificate, parentKey *ecdsa.PrivateKey, commonName string) error {
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(intermediateValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1, // Intermediate -> Leaf
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, parentCert, &intermediateKey.PublicKey, parentKey)
	if err != nil {
		return err
	}

	intermediateCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	if err := writePEMFile(certPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(intermediateKey)
	if err != nil {
		return err
	}

	// Determine CA type for keystore storage
	var caType string
	switch commonName {
	case hubCommonName:
		caType = "hub"
	case operatorCommonName:
		caType = string(constants.UserRoleOperator)
	}

	if pki.secretManager == nil {
		return fmt.Errorf("SecretManager is required for PKI private key storage")
	}
	if caType == "" {
		return fmt.Errorf("unknown CA common name: %s", commonName)
	}
	if err := pki.secretManager.StoreCAPrivateKey(caType, keyDER); err != nil {
		return fmt.Errorf("store %s CA private key in keystore: %w", caType, err)
	}

	// Store in the appropriate field based on common name
	switch commonName {
	case hubCommonName:
		pki.hubCert = intermediateCert
		pki.hubKey = intermediateKey
	case operatorCommonName:
		pki.operatorCert = intermediateCert
		pki.operatorKey = intermediateKey
	}

	return nil
}

func (pki *PKIAuthority) ensureAppCerts() error {
	// No first-class app certificates are generated at the protocol level.
	// g8e-compatible agentic ensembles use the SignCSR API for certificate issuance.
	return nil
}

func (pki *PKIAuthority) generateServiceCert(extraIPs []net.IP) error {
	serviceCertPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-gateway.crt")

	if pki.hubCert == nil || pki.hubKey == nil {
		return fmt.Errorf("hub CA not loaded - call EnsurePKI first")
	}

	serviceKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	dnsNames := []string{"localhost", "g8e.local", string(constants.SessionTypeOperator)}
	ipAddresses := append([]net.IP{net.ParseIP("127.0.0.1")}, extraIPs...)

	// Add URI SAN for workload identity
	wid := protocol.NewWorkloadIdentity()
	hubURL, _ := wid.HubSPIFFEURL()

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "operator-gateway",
			Organization: []string{"g8e"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(serviceValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		URIs:                  []*url.URL{hubURL},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, pki.hubCert, &serviceKey.PublicKey, pki.hubKey)
	if err != nil {
		return err
	}

	// Write chain PEM (leaf + hub intermediate + root)
	chainPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	hubPEM, _ := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "hub_ca.crt"))
	rootPEM, _ := os.ReadFile(filepath.Join(pki.pkiDir, "root", "root_ca.crt"))
	chainPEM = append(chainPEM, hubPEM...)
	chainPEM = append(chainPEM, rootPEM...)
	chainPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-gateway.chain.pem")
	if err := os.WriteFile(chainPath, chainPEM, 0600); err != nil {
		return fmt.Errorf("failed to write chain: %w", err)
	}

	if err := writePEMFile(serviceCertPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(serviceKey)
	if err != nil {
		return err
	}
	if pki.secretManager == nil {
		return fmt.Errorf("SecretManager is required for service private key storage")
	}
	if err := pki.secretManager.StoreServicePrivateKey("operator-gateway", keyDER); err != nil {
		return fmt.Errorf("store operator-gateway private key in keystore: %w", err)
	}

	return nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func writePEMFile(path, pemType string, der []byte, mode os.FileMode) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}()
	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: der})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isExpiringSoon(cert tls.Certificate) bool {
	if len(cert.Certificate) == 0 {
		return true
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}
	return time.Until(x509Cert.NotAfter) < 30*24*time.Hour
}

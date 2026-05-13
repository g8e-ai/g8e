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

package listen

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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

const (
	rootValidityDays         = 3650
	intermediateValidityDays = 3650
	serviceValidityDays      = 90
	bootstrapValidityDays    = 1 // Short-lived for enrollment certs

	rootCommonName      = "g8e Root CA"
	hubCommonName       = "g8e Hub Intermediate CA"
	operatorCommonName  = "g8e Operator Intermediate CA"
	bootstrapCommonName = "g8e Bootstrap Intermediate CA"

	trustDomain = "g8e.local"
)

// PKIAuthority manages the full PKI hierarchy for the Operator.
// It generates and stores:
// - Root CA under pki/root/
// - Intermediate CAs under pki/authorities/
// - Service certificates under pki/issued/
// - Trust bundles under pki/trust/
// - Revocation state under pki/revocation/
type PKIAuthority struct {
	mu     sync.RWMutex
	logger *slog.Logger
	db     *ListenDBService

	pkiDir string

	// Root CA
	rootCert *x509.Certificate
	rootKey  *ecdsa.PrivateKey

	// Intermediate CAs
	hubCert       *x509.Certificate
	hubKey        *ecdsa.PrivateKey
	operatorCert  *x509.Certificate
	operatorKey   *ecdsa.PrivateKey
	bootstrapCert *x509.Certificate
	bootstrapKey  *ecdsa.PrivateKey

	// Service certificate for operator-listen
	serviceCert tls.Certificate
}

func newPKIAuthority(dataDir, pkiDir string, db *ListenDBService, logger *slog.Logger) *PKIAuthority {
	if pkiDir == "" {
		pkiDir = filepath.Join(dataDir, "pki")
	}
	return &PKIAuthority{
		pkiDir: pkiDir,
		db:     db,
		logger: logger,
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

	// Generate or load operator-listen service certificate
	if err := pki.ensureServiceCert(extraIPs); err != nil {
		return fmt.Errorf("service certificate setup failed: %w", err)
	}

	// Generate or load certificates for reference apps
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

// TLSConfigPlain returns a TLS config for the bootstrap listener.
// This listener does not require client certificates for unauthenticated routes
// (/.well-known/, /api/auth/device-link/register).
func (pki *PKIAuthority) TLSConfigPlain() *tls.Config {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	return &tls.Config{
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			pki.mu.RLock()
			defer pki.mu.RUnlock()
			c := pki.serviceCert
			return &c, nil
		},
		ClientAuth: tls.NoClientCert,
		MinVersion: tls.VersionTLS13,
	}
}

// TrustBundlePath returns the path to the hub trust bundle.
func (pki *PKIAuthority) TrustBundlePath() string {
	return filepath.Join(pki.pkiDir, "trust", "hub-bundle.pem")
}

// PKIDir returns the path to the pki directory.
func (pki *PKIAuthority) PKIDir() string {
	return pki.pkiDir
}

// ─── PKI hierarchy management ─────────────────────────────────────────────

func (pki *PKIAuthority) ensureRootCA() error {
	rootKeyPath := filepath.Join(pki.pkiDir, "root", "root_ca.key")
	rootCertPath := filepath.Join(pki.pkiDir, "root", "root_ca.crt")

	needRoot := !fileExists(rootKeyPath) || !fileExists(rootCertPath)
	if !needRoot {
		if err := pki.loadCertificatePair(rootCertPath, rootKeyPath, &pki.rootCert, &pki.rootKey); err != nil {
			pki.logger.Warn("[PKI] Failed to load root CA, regenerating", "error", err)
			needRoot = true
		}
	}

	if needRoot {
		pki.logger.Info("[PKI] Generating root CA")
		if err := pki.generateRootCA(rootKeyPath, rootCertPath); err != nil {
			return err
		}
	}

	return nil
}

func (pki *PKIAuthority) ensureIntermediateCAs() error {
	// Hub Intermediate CA
	hubKeyPath := filepath.Join(pki.pkiDir, "authorities", "hub_ca.key")
	hubCertPath := filepath.Join(pki.pkiDir, "authorities", "hub_ca.crt")
	needHub := !fileExists(hubKeyPath) || !fileExists(hubCertPath)
	if !needHub {
		if err := pki.loadCertificatePair(hubCertPath, hubKeyPath, &pki.hubCert, &pki.hubKey); err != nil {
			pki.logger.Warn("[PKI] Failed to load hub CA, regenerating", "error", err)
			needHub = true
		}
	}
	if needHub {
		pki.logger.Info("[PKI] Generating hub intermediate CA")
		if err := pki.generateIntermediateCA(hubKeyPath, hubCertPath, pki.rootCert, pki.rootKey, hubCommonName); err != nil {
			return err
		}
	}

	// Operator Intermediate CA
	operatorKeyPath := filepath.Join(pki.pkiDir, "authorities", "operator_ca.key")
	operatorCertPath := filepath.Join(pki.pkiDir, "authorities", "operator_ca.crt")
	needOperator := !fileExists(operatorKeyPath) || !fileExists(operatorCertPath)
	if !needOperator {
		if err := pki.loadCertificatePair(operatorCertPath, operatorKeyPath, &pki.operatorCert, &pki.operatorKey); err != nil {
			pki.logger.Warn("[PKI] Failed to load operator CA, regenerating", "error", err)
			needOperator = true
		}
	}
	if needOperator {
		pki.logger.Info("[PKI] Generating operator intermediate CA")
		if err := pki.generateIntermediateCA(operatorKeyPath, operatorCertPath, pki.rootCert, pki.rootKey, operatorCommonName); err != nil {
			return err
		}
	}

	// Bootstrap Intermediate CA
	bootstrapKeyPath := filepath.Join(pki.pkiDir, "authorities", "bootstrap_ca.key")
	bootstrapCertPath := filepath.Join(pki.pkiDir, "authorities", "bootstrap_ca.crt")
	needBootstrap := !fileExists(bootstrapKeyPath) || !fileExists(bootstrapCertPath)
	if !needBootstrap {
		if err := pki.loadCertificatePair(bootstrapCertPath, bootstrapKeyPath, &pki.bootstrapCert, &pki.bootstrapKey); err != nil {
			pki.logger.Warn("[PKI] Failed to load bootstrap CA, regenerating", "error", err)
			needBootstrap = true
		}
	}
	if needBootstrap {
		pki.logger.Info("[PKI] Generating bootstrap intermediate CA")
		if err := pki.generateIntermediateCA(bootstrapKeyPath, bootstrapCertPath, pki.rootCert, pki.rootKey, bootstrapCommonName); err != nil {
			return err
		}
	}

	return nil
}

func (pki *PKIAuthority) ensureServiceCert(extraIPs []net.IP) error {
	serviceKeyPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-listen.key")
	serviceCertPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-listen.crt")

	needService := !fileExists(serviceKeyPath) || !fileExists(serviceCertPath)
	if !needService {
		tlsCert, err := tls.LoadX509KeyPair(serviceCertPath, serviceKeyPath)
		if err != nil {
			pki.logger.Warn("[PKI] Failed to load service cert, regenerating", "error", err)
			needService = true
		} else {
			if isExpiringSoon(tlsCert) {
				pki.logger.Info("[PKI] Service certificate expiring soon, renewing")
				needService = true
			} else {
				pki.serviceCert = tlsCert
			}
		}
	}

	if needService {
		pki.logger.Info("[PKI] Generating operator-listen service certificate")
		if err := pki.generateServiceCert(extraIPs); err != nil {
			return err
		}
		tlsCert, err := tls.LoadX509KeyPair(serviceCertPath, serviceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to load generated service cert: %w", err)
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

	// Hub bundle (root + hub intermediate)
	hubBundlePath := filepath.Join(pki.pkiDir, "trust", "hub-bundle.pem")
	hubPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "hub_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read hub CA: %w", err)
	}
	hubBundle := append([]byte(rootPEM), hubPEM...)
	if err := os.WriteFile(hubBundlePath, hubBundle, 0644); err != nil {
		return fmt.Errorf("failed to write hub bundle: %w", err)
	}

	// Operator bundle (root + operator intermediate)
	operatorBundlePath := filepath.Join(pki.pkiDir, "trust", "operator-bundle.pem")
	operatorPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "operator_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read operator CA: %w", err)
	}
	operatorBundle := append([]byte(rootPEM), operatorPEM...)
	if err := os.WriteFile(operatorBundlePath, operatorBundle, 0644); err != nil {
		return fmt.Errorf("failed to write operator bundle: %w", err)
	}

	// Bootstrap bundle (root + bootstrap intermediate)
	bootstrapBundlePath := filepath.Join(pki.pkiDir, "trust", "bootstrap-bundle.pem")
	bootstrapPEM, err := os.ReadFile(filepath.Join(pki.pkiDir, "authorities", "bootstrap_ca.crt"))
	if err != nil {
		return fmt.Errorf("failed to read bootstrap CA: %w", err)
	}
	bootstrapBundle := append([]byte(rootPEM), bootstrapPEM...)
	if err := os.WriteFile(bootstrapBundlePath, bootstrapBundle, 0644); err != nil {
		return fmt.Errorf("failed to write bootstrap bundle: %w", err)
	}

	// Trust domain metadata
	trustDomainData := map[string]string{
		"trust_domain": trustDomain,
	}
	trustDomainJSON, _ := json.MarshalIndent(trustDomainData, "", "  ")
	if err := os.WriteFile(filepath.Join(pki.pkiDir, "trust", "trust-domain.json"), trustDomainJSON, 0644); err != nil {
		return fmt.Errorf("failed to write trust-domain.json: %w", err)
	}

	return nil
}

// HubTrustBundle returns the full PEM-encoded hub trust bundle (root + hub intermediate).
func (pki *PKIAuthority) HubTrustBundle() ([]byte, error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()
	return os.ReadFile(filepath.Join(pki.pkiDir, "trust", "hub-bundle.pem"))
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

	return pki.db.DocSet(string(constants.CollectionRevokedCertificates), serial, body)
}

// GenerateRevocationBundle creates a signed JSON bundle of all revoked certificate serials.
func (pki *PKIAuthority) GenerateRevocationBundle() (bundleJSON string, signature string, err error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	if pki.db == nil {
		return "", "", fmt.Errorf("database not available")
	}

	docs, err := pki.db.DocQuery(string(constants.CollectionRevokedCertificates), nil, "revoked_at", 0)
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
		"trust_domain":    trustDomain,
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

	doc, err := pki.db.DocGet(string(constants.CollectionRevokedCertificates), serial)
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
// leafType should be "operator" or "app".
func (pki *PKIAuthority) SignCSR(csrPEM string, leafType string, organizationID, operatorID, sessionID string) (certPEM, chainPEM string, err error) {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	if pki.operatorCert == nil || pki.operatorKey == nil {
		return "", "", fmt.Errorf("operator CA not loaded — call EnsurePKI first")
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
	var uriSAN string
	if leafType == "operator" {
		uriSAN = fmt.Sprintf("spiffe://%s/operator/%s/%s/%s", trustDomain, organizationID, operatorID, sessionID)
	} else if leafType == "app" {
		uriSAN = fmt.Sprintf("spiffe://%s/app/%s", trustDomain, operatorID)
	}
	if uriSAN != "" {
		parsed, _ := url.Parse(uriSAN)
		template.URIs = []*url.URL{parsed}
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

func (pki *PKIAuthority) loadCertificatePair(certPath, keyPath string, cert **x509.Certificate, key **ecdsa.PrivateKey) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyPath)
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

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("invalid key PEM")
	}
	parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		keyIface, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return fmt.Errorf("parse key: %w (also tried PKCS8: %v)", err, err2)
		}
		var ok bool
		parsedKey, ok = keyIface.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("key is not ECDSA")
		}
	}

	*cert = parsedCert
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
	if err := writePEMFile(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
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
	if err := writePEMFile(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}

	// Store in the appropriate field based on common name
	switch commonName {
	case hubCommonName:
		pki.hubCert = intermediateCert
		pki.hubKey = intermediateKey
	case operatorCommonName:
		pki.operatorCert = intermediateCert
		pki.operatorKey = intermediateKey
	case bootstrapCommonName:
		pki.bootstrapCert = intermediateCert
		pki.bootstrapKey = intermediateKey
	}

	return nil
}

func (pki *PKIAuthority) ensureAppCerts() error {
	apps := []string{constants.Status.ComponentName.G8EE}
	for _, app := range apps {
		keyPath := filepath.Join(pki.pkiDir, "issued", "apps", app+".key")
		certPath := filepath.Join(pki.pkiDir, "issued", "apps", app+".crt")

		if fileExists(keyPath) && fileExists(certPath) {
			continue
		}

		pki.logger.Info("[PKI] Generating certificate for reference app", "app", app)
		// We use a simplified signing flow for bundled reference apps during bootstrap.
		// In a BYO world, they would use the SignCSR API.
		if err := pki.generateAppCert(app, keyPath, certPath); err != nil {
			return fmt.Errorf("failed to generate cert for %s: %w", app, err)
		}
	}
	return nil
}

func (pki *PKIAuthority) generateAppCert(app, keyPath, certPath string) error {
	if pki.hubCert == nil || pki.hubKey == nil {
		return fmt.Errorf("hub CA not loaded")
	}

	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, _ := randomSerial()
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   app,
			Organization: []string{"g8e"},
		},
		NotBefore:   now.Add(-1 * time.Minute),
		NotAfter:    now.Add(time.Duration(serviceValidityDays) * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs: []*url.URL{
			{Scheme: "spiffe", Host: trustDomain, Path: "/app/" + app},
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, pki.hubCert, &priv.PublicKey, pki.hubKey)
	if err != nil {
		return err
	}

	if err := writePEMFile(certPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, _ := x509.MarshalECPrivateKey(priv)
	if err := writePEMFile(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}

	return nil
}

func (pki *PKIAuthority) generateServiceCert(extraIPs []net.IP) error {
	serviceKeyPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-listen.key")
	serviceCertPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-listen.crt")

	if pki.hubCert == nil || pki.hubKey == nil {
		return fmt.Errorf("hub CA not loaded — call EnsurePKI first")
	}

	serviceKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	dnsNames := []string{"localhost", "g8e.local", "operator", constants.Status.ComponentName.G8EE}
	ipAddresses := append([]net.IP{net.ParseIP("127.0.0.1")}, extraIPs...)

	// Add URI SAN for workload identity
	uriSANs := []string{
		fmt.Sprintf("spiffe://%s/hub/operator-listen", trustDomain),
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "operator-listen",
			Organization: []string{"g8e"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(serviceValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		URIs:                  parseURIs(uriSANs),
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
	chainPath := filepath.Join(pki.pkiDir, "issued", "hub", "operator-listen.chain.pem")
	if err := os.WriteFile(chainPath, chainPEM, 0644); err != nil {
		return fmt.Errorf("failed to write chain: %w", err)
	}

	if err := writePEMFile(serviceCertPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(serviceKey)
	if err != nil {
		return err
	}
	if err := writePEMFile(serviceKeyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}

	return nil
}

func parseURIs(uris []string) []*url.URL {
	result := make([]*url.URL, len(uris))
	for i, u := range uris {
		parsed, _ := url.Parse(u)
		result[i] = parsed
	}
	return result
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func writePEMFile(path, pemType string, der []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
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

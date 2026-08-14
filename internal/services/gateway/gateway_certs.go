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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/protocol"
)

const (
	rootValidityDays         = 3650
	intermediateValidityDays = 3650
	servingCertValidityDays  = 90 // 90-day TTL for gateway serving certificate
	leafCertValidityDays     = 7  // 7-day TTL for operator/cli leaf certificates
	peerCertValidityDays     = 90 // 90-day TTL for gateway peer certificates

	hubCommonName         = "g8e Hub Intermediate CA"
	operatorCommonName    = "g8e Operator Intermediate CA"
	gatewayPeerCommonName = "g8e Gateway Peer Intermediate CA"
)

// secretManagerInterface defines the methods PKIAuthority uses from SecretManager.
type secretManagerInterface interface {
	GetCAPrivateKey(caType string) ([]byte, error)
	StoreCAPrivateKey(caType string, keyDER []byte) error
	GetServicePrivateKey(serviceName string) ([]byte, error)
	StoreServicePrivateKey(serviceName string, keyDER []byte) error
}

// revocationDocument represents a certificate revocation record.
type revocationDocument struct {
	Serial    string    `json:"serial"`
	Reason    string    `json:"reason"`
	RevokedAt time.Time `json:"revoked_at"`
}

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
	db     *DocumentStoreService

	fileSvc       fs.RuntimeFileService
	secretManager secretManagerInterface

	// Root CA
	rootCert *x509.Certificate
	rootKey  *ecdsa.PrivateKey

	// Intermediate CAs
	hubCert         *x509.Certificate
	hubKey          *ecdsa.PrivateKey
	operatorCert    *x509.Certificate
	operatorKey     *ecdsa.PrivateKey
	gatewayPeerCert *x509.Certificate
	gatewayPeerKey  *ecdsa.PrivateKey

	// Service certificate for operator-gateway
	serviceCert tls.Certificate
}

func newPKIAuthority(fileSvc fs.RuntimeFileService, docStore *DocumentStoreService, secretManager secretManagerInterface, logger *slog.Logger) *PKIAuthority {
	return &PKIAuthority{
		fileSvc:       fileSvc,
		db:            docStore,
		secretManager: secretManager,
		logger:        logger,
	}
}

// InitializePKI initializes the full PKI hierarchy. Must be called before TLSConfig().
func (pki *PKIAuthority) InitializePKI(extraIPs []net.IP) error {
	return pki.InitializePKIWithNames(extraIPs, nil)
}

// InitializePKIWithNames initializes the full PKI hierarchy with custom DNS names.
// Must be called before TLSConfig().
func (pki *PKIAuthority) InitializePKIWithNames(extraIPs []net.IP, extraDNSNames []string) error {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	// Create directory structure
	dirs := []string{
		filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirGatewayPeer),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirRevocation),
	}
	for _, dir := range dirs {
		if err := pki.fileSvc.MkdirAll(context.Background(), dir, constants.PermDirStandard); err != nil {
			return fmt.Errorf("%s %s: %w", constants.ErrPKICreateDirectory, dir, err)
		}
	}

	// Generate or load Root CA
	if err := pki.loadOrGenerateRootCA(); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadRootCA, err)
	}

	// Generate or load Intermediate CAs
	if err := pki.loadOrGenerateIntermediateCAs(); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
	}

	// Generate or load operator-gateway service certificate
	if err := pki.loadOrGenerateServiceCertWithNames(extraIPs, extraDNSNames); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
	}

	// Generate trust bundles
	if err := pki.generateTrustBundles(); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateTrustBundles, err)
	}

	pki.logger.Info("[PKI] PKI hierarchy initialized", "pki_dir", pki.PKIDir())
	return nil
}

// TLSConfig returns a tls.Config that serves the managed server certificate.
// It also configures the client CA pool to enable mTLS verification.
func (pki *PKIAuthority) TLSConfig() *tls.Config {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	// Create client CA pool from root and Operator intermediate for client verification
	// Hub intermediate is excluded as it only signs the gateway serving certificate
	pool := x509.NewCertPool()
	if pki.rootCert != nil {
		pool.AddCert(pki.rootCert)
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
		ClientAuth:       tls.RequireAndVerifyClientCert,
		ClientCAs:        pool,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
	}
}

// TrustBundlePath returns the path to the gateway trust bundle.
func (pki *PKIAuthority) TrustBundlePath() string {
	return pki.fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
}

// RootCAPath returns the path to the root CA certificate.
func (pki *PKIAuthority) RootCAPath() string {
	return pki.fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
}

// BinariesDir returns the path to the PKI binaries directory.
func (pki *PKIAuthority) BinariesDir() string {
	return pki.fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirBinaries))
}

// PKIDir returns the path to the pki directory.
func (pki *PKIAuthority) PKIDir() string {
	return pki.fileSvc.Resolve(constants.PkiDirname)
}

// RootCAPEM returns the PEM-encoded root CA certificate bytes.
func (pki *PKIAuthority) RootCAPEM() ([]byte, error) {
	return pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
}

// ─── PKI hierarchy management ─────────────────────────────────────────────

func (pki *PKIAuthority) loadOrGenerateRootCA() error {
	rootCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)

	exists, err := pki.fileSvc.FileExists(context.Background(), rootCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadRootCA, err)
	}
	if exists {
		if err := pki.loadCACertificate(rootCARelPath, &pki.rootCert); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadRootCA, err)
		}
		// Verify private key exists in keystore; regenerate if missing
		if _, err := pki.secretManager.GetCAPrivateKey(string(constants.CATypeRoot)); err != nil {
			pki.logger.Info("[PKI] Root CA private key missing from keystore, regenerating")
			if err := pki.generateRootCA(rootCARelPath); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKIGenerateRootCA, err)
			}
			return nil
		}
		return nil
	}

	pki.logger.Info("[PKI] Generating root CA")
	if err := pki.generateRootCA(rootCARelPath); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateRootCA, err)
	}
	return nil
}

func (pki *PKIAuthority) loadOrGenerateIntermediateCAs() error {
	hubCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA)
	operatorCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA)
	gatewayPeerCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileGatewayPeerCA)

	// Hub Intermediate CA
	hubExists, err := pki.fileSvc.FileExists(context.Background(), hubCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
	}
	if hubExists {
		if err := pki.loadCACertificate(hubCARelPath, &pki.hubCert); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
		}
		// Verify private key exists in keystore; regenerate if missing
		if _, err := pki.secretManager.GetCAPrivateKey(string(constants.CATypeHub)); err != nil {
			pki.logger.Info("[PKI] Hub CA private key missing from keystore, regenerating")
			if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
			}
			if err := pki.generateIntermediateCA(hubCARelPath, pki.rootCert, pki.rootKey, hubCommonName); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
			}
		}
	} else {
		pki.logger.Info("[PKI] Generating hub intermediate CA")
		if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
		}
		if err := pki.generateIntermediateCA(hubCARelPath, pki.rootCert, pki.rootKey, hubCommonName); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
		}
	}

	// Operator Intermediate CA
	opExists, err := pki.fileSvc.FileExists(context.Background(), operatorCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
	}
	if opExists {
		if err := pki.loadCACertificate(operatorCARelPath, &pki.operatorCert); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
		}
		// Verify private key exists in keystore; regenerate if missing
		if _, err := pki.secretManager.GetCAPrivateKey(string(constants.CATypeOperator)); err != nil {
			pki.logger.Info("[PKI] Operator CA private key missing from keystore, regenerating")
			if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
			}
			if err := pki.generateIntermediateCA(operatorCARelPath, pki.rootCert, pki.rootKey, operatorCommonName); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
			}
		}
	} else {
		pki.logger.Info("[PKI] Generating Operator intermediate CA")
		if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
		}
		if err := pki.generateIntermediateCA(operatorCARelPath, pki.rootCert, pki.rootKey, operatorCommonName); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
		}
	}

	// Gateway Peer Intermediate CA
	gpExists, err := pki.fileSvc.FileExists(context.Background(), gatewayPeerCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
	}
	if gpExists {
		if err := pki.loadCACertificate(gatewayPeerCARelPath, &pki.gatewayPeerCert); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadIntermediateCA, err)
		}
		// Verify private key exists in keystore; regenerate if missing
		if _, err := pki.secretManager.GetCAPrivateKey(string(constants.CATypeGatewayPeer)); err != nil {
			pki.logger.Info("[PKI] Gateway peer CA private key missing from keystore, regenerating")
			if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
			}
			if err := pki.generateIntermediateCA(gatewayPeerCARelPath, pki.rootCert, pki.rootKey, gatewayPeerCommonName); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
			}
		}
	} else {
		pki.logger.Info("[PKI] Generating gateway peer intermediate CA")
		if err := pki.loadCAPrivateKey(string(constants.CATypeRoot), &pki.rootKey); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
		}
		if err := pki.generateIntermediateCA(gatewayPeerCARelPath, pki.rootCert, pki.rootKey, gatewayPeerCommonName); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
		}
	}

	return nil
}
func (pki *PKIAuthority) loadOrGenerateServiceCertWithNames(extraIPs []net.IP, extraDNSNames []string) error {
	serviceCertRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)
	chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)

	certExists, err := pki.fileSvc.FileExists(context.Background(), serviceCertRelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
	}
	chainExists, err := pki.fileSvc.FileExists(context.Background(), chainRelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
	}
	needService := !certExists || !chainExists
	if !needService {
		// Load certificate chain from file
		chainPEM, err := pki.fileSvc.ReadFile(context.Background(), chainRelPath)
		if err != nil {
			pki.logger.Warn("[PKI] Failed to load service cert chain, regenerating", string(constants.ConnectionStateError), err)
			needService = true
		} else {
			// Load private key from keystore
			if pki.secretManager == nil {
				return fmt.Errorf("SecretManager is required for service private key loading")
			}
			keyDER, err := pki.secretManager.GetServicePrivateKey(string(constants.ServiceNameOperatorGateway))
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
			if err := pki.loadCAPrivateKey(string(constants.CATypeHub), &pki.hubKey); err != nil {
				return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
			}
		}
		if err := pki.generateServiceCertWithNames(extraIPs, extraDNSNames); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKIGenerateServiceCert, err)
		}
		// Load the newly generated certificate and key
		chainPEM, err := pki.fileSvc.ReadFile(context.Background(), chainRelPath)
		if err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
		}
		keyDER, err := pki.secretManager.GetServicePrivateKey(string(constants.ServiceNameOperatorGateway))
		if err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceKey, err)
		}
		tlsCert, err := tls.X509KeyPair(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
		if err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
		}
		pki.serviceCert = tlsCert
	}

	return nil
}

func (pki *PKIAuthority) generateTrustBundles() error {
	rootCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)
	hubCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA)
	operatorCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA)
	gatewayPeerCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileGatewayPeerCA)
	trustBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
	operatorBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileOperatorBundle)
	rootBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileRootBundle)
	trustDomainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileTrustDomainJSON)

	// Gateway bundle (root + hub intermediate + Operator intermediate + gateway peer intermediate)
	rootPEM, err := pki.fileSvc.ReadFile(context.Background(), rootCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadRootCA, err)
	}
	hubPEM, err := pki.fileSvc.ReadFile(context.Background(), hubCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadHubCA, err)
	}
	operatorPEM, err := pki.fileSvc.ReadFile(context.Background(), operatorCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadOperatorCA, err)
	}
	gatewayPeerPEM, err := pki.fileSvc.ReadFile(context.Background(), gatewayPeerCARelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadGatewayPeerCA, err)
	}
	hubBundle := make([]byte, 0, len(rootPEM)+len(hubPEM)+len(operatorPEM)+len(gatewayPeerPEM))
	hubBundle = append(hubBundle, rootPEM...)
	hubBundle = append(hubBundle, hubPEM...)
	hubBundle = append(hubBundle, operatorPEM...)
	hubBundle = append(hubBundle, gatewayPeerPEM...)
	if err := pki.fileSvc.WriteFile(context.Background(), trustBundleRelPath, hubBundle, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWriteGatewayBundle, err)
	}

	// Operator bundle (root + Operator intermediate)
	operatorBundle := make([]byte, 0, len(rootPEM)+len(operatorPEM))
	operatorBundle = append(operatorBundle, rootPEM...)
	operatorBundle = append(operatorBundle, operatorPEM...)
	if err := pki.fileSvc.WriteFile(context.Background(), operatorBundleRelPath, operatorBundle, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWriteOperatorBundle, err)
	}

	// Root CA mirror (for operator clients)
	if err := pki.fileSvc.WriteFile(context.Background(), rootBundleRelPath, rootPEM, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWriteRootBundle, err)
	}

	// Trust domain metadata
	trustDomainData := map[string]string{
		"trust_domain": protocol.TrustDomain,
	}
	trustDomainJSON, err := json.MarshalIndent(trustDomainData, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIMarshalTrustDomain, err)
	}
	trustDomainPEM := pem.EncodeToMemory(&pem.Block{Type: "TRUST DOMAIN", Bytes: trustDomainJSON})
	if err := pki.fileSvc.WriteFile(context.Background(), trustDomainRelPath, trustDomainPEM, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWriteTrustDomain, err)
	}

	return nil
}

// GatewayTrustBundle returns the full PEM-encoded gateway trust bundle (root + hub intermediate + Operator intermediate).
func (pki *PKIAuthority) GatewayTrustBundle() ([]byte, error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()
	trustBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
	if pki.logger != nil {
		pki.logger.Debug("GatewayTrustBundle reading", "path", pki.fileSvc.Resolve(trustBundleRelPath), "pki_dir", pki.PKIDir())
	}
	data, err := pki.fileSvc.ReadFile(context.Background(), trustBundleRelPath)
	if err != nil {
		if pki.logger != nil {
			pki.logger.Error("GatewayTrustBundle failed to read", "error", err, "path", pki.fileSvc.Resolve(trustBundleRelPath), "pki_dir", pki.PKIDir())
		}
		return nil, err
	}
	return data, nil
}

// RevokeCertificate adds a certificate serial to the revocation list.
func (pki *PKIAuthority) RevokeCertificate(serial string, reason string) error {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	if pki.db == nil {
		return constants.ErrPKIDatabaseNotAvailable
	}

	doc := revocationDocument{
		Serial:    serial,
		Reason:    reason,
		RevokedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIRevokeCertificate, err)
	}

	return pki.db.DocSet(marshaler.CollectionName(constants.CollectionRevokedCertificates), serial, body)
}

// GenerateCRL creates a standard X.509 Certificate Revocation List (CRL) signed by the Operator intermediate CA.
// The CRL contains all revoked certificate serials from the database.
func (pki *PKIAuthority) GenerateCRL() (crlDER []byte, err error) {
	pki.mu.RLock()
	defer pki.mu.RUnlock()

	if pki.db == nil {
		return nil, constants.ErrPKIDatabaseNotAvailable
	}

	if pki.operatorCert == nil || pki.operatorKey == nil {
		return nil, constants.ErrPKIOperatorCANotLoaded
	}

	docs, err := pki.db.DocQuery(marshaler.CollectionName(constants.CollectionRevokedCertificates), nil, "revoked_at", 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrPKIGenerateCRL, err)
	}

	// Build revoked certificate list for CRL
	revokedCerts := []pkix.RevokedCertificate{}
	now := time.Now().UTC()

	for _, doc := range docs {
		serial, ok := new(big.Int).SetString(doc.ID, 10)
		if !ok {
			pki.logger.Warn("[PKI] Invalid serial in revocation list", "serial", doc.ID)
			continue
		}

		// Parse revocation time if available, otherwise use current time
		var revokedAt time.Time
		wireData := doc.ForWire()
		if revokedAtRaw, ok := wireData["revoked_at"]; ok {
			var revokedAtStr string
			if err := json.Unmarshal(revokedAtRaw, &revokedAtStr); err == nil {
				revokedAt, _ = time.Parse(time.RFC3339, revokedAtStr)
			}
		}
		if revokedAt.IsZero() {
			revokedAt = now
		}

		revokedCerts = append(revokedCerts, pkix.RevokedCertificate{
			SerialNumber:   serial,
			RevocationTime: revokedAt,
		})
	}

	// Create CRL template
	crlTemplate := &x509.RevocationList{
		SignatureAlgorithm:  x509.ECDSAWithSHA256,
		RevokedCertificates: revokedCerts,
		Number:              big.NewInt(1), // Simple version number
		ThisUpdate:          now,
		NextUpdate:          now.Add(24 * time.Hour), // CRL valid for 24 hours
	}

	// Generate CRL signed by Operator intermediate CA
	crlDER, err = x509.CreateRevocationList(rand.Reader, crlTemplate, pki.operatorCert, pki.operatorKey)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrPKIGenerateCRL, err)
	}

	pki.logger.Info("[PKI] Generated CRL", "revoked_count", len(revokedCerts))
	return crlDER, nil
}

// IsRevoked checks if a certificate serial is in the revocation list.
func (pki *PKIAuthority) IsRevoked(serial string) (bool, error) {
	if pki.db == nil {
		return false, constants.ErrPKIDatabaseNotAvailable
	}

	doc, err := pki.db.DocGet(marshaler.CollectionName(constants.CollectionRevokedCertificates), serial)
	if err != nil {
		return false, fmt.Errorf("%s: %w", constants.ErrPKICheckRevocation, err)
	}

	return doc != nil, nil
}

// VerifyCertificate checks if a certificate is valid and not revoked.
func (pki *PKIAuthority) VerifyCertificate(cert *x509.Certificate) error {
	if cert == nil {
		return constants.ErrPKINoCertificate
	}

	revoked, err := pki.IsRevoked(cert.SerialNumber.String())
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKICheckRevocation, err)
	}

	if revoked {
		return constants.ErrPKICertificateRevoked
	}

	return nil
}

// SignCSR signs a certificate signing request using the appropriate intermediate CA.
// leafType should be "operator", "app", "cli", or "gateway-peer".
// Parameters:
//   - For "operator": organizationID, operatorID, sessionID (operator_session_id)
//   - For "cli": userID, sessionID (cli_session_id)
//   - For "app": operatorID (app identity)
//   - For "gateway-peer": gatewayID (gateway peer identity)
func (pki *PKIAuthority) SignCSR(csrPEM string, leafType string, organizationID, operatorID, userID, sessionID, gatewayID string) (certPEM, chainPEM string, err error) {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	// Determine which CA to use based on leaf type
	var caCert *x509.Certificate
	var caKey *ecdsa.PrivateKey
	var caType constants.CAType
	var certValidityDays int

	switch leafType {
	case "gateway-peer":
		if pki.gatewayPeerCert == nil {
			return "", "", constants.ErrPKIGatewayPeerCANotLoaded
		}
		caCert = pki.gatewayPeerCert
		caType = constants.CATypeGatewayPeer
		certValidityDays = peerCertValidityDays
	default:
		// operator, cli, app use Operator CA
		if pki.operatorCert == nil {
			return "", "", constants.ErrPKIOperatorCANotLoaded
		}
		caCert = pki.operatorCert
		caType = constants.CATypeOperator
		certValidityDays = leafCertValidityDays
	}

	// Load CA private key on-demand for signing
	if caKey == nil {
		if err := pki.loadCAPrivateKey(string(caType), &caKey); err != nil {
			return "", "", fmt.Errorf("%s %s: %w", constants.ErrPKILoadCAPrivateKey, caType, err)
		}
	}

	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", "", constants.ErrPKIInvalidCSR
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIParseCSR, err)
	}

	if err := csr.CheckSignature(); err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKICSRSignatureCheck, err)
	}

	// Enforce P-256 curve policy for all leaf certificates
	if !isCurveP256(csr.PublicKey) {
		return "", "", constants.ErrPKIInvalidCurve
	}

	serial, err := randomSerial()
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    now.Add(-1 * time.Minute),
		NotAfter:     now.Add(time.Duration(certValidityDays) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}

	// Set URI SAN for workload identity
	wid := protocol.NewWorkloadIdentity()
	var uriURL *url.URL
	switch leafType {
	case string(constants.SessionTypeOperator):
		uriURL, err = wid.OperatorSPIFFEURL(organizationID, operatorID, sessionID)
	case string(constants.SessionTypeCLI):
		uriURL, err = wid.CLISPIFFEURL(userID, sessionID)
	case "app":
		uriURL, err = wid.AppSPIFFEURL(operatorID)
	case "gateway-peer":
		uriURL, err = wid.GatewayPeerSPIFFEURL(gatewayID)
	}
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIGenerateSPIFFEURL, err)
	}

	if uriURL != nil {
		template.URIs = []*url.URL{uriURL}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, csr.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKISignCSR, err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	// Build chain based on CA type
	rootPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIReadRootCA, err)
	}
	if leafType == "gateway-peer" {
		// Gateway peer chain: leaf + gateway peer intermediate + root
		caPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileGatewayPeerCA))
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", constants.ErrPKIReadGatewayPeerCA, err)
		}
		chainPEM = certPEM + string(caPEM) + string(rootPEM)
	} else {
		// Operator/cli/app chain: leaf + Operator intermediate + root
		caPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA))
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", constants.ErrPKIReadOperatorCA, err)
		}
		chainPEM = certPEM + string(caPEM) + string(rootPEM)
	}

	return certPEM, chainPEM, nil
}

// SignDelegatedCSR signs a CSR for a delegated credential with dual SANs (app + requestor).
// The certificate is short-lived (1 hour) and binds both the app identity and the requesting user.
func (pki *PKIAuthority) SignDelegatedCSR(csrPEM string, appName, userID string) (certPEM, chainPEM string, err error) {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	// Use Operator CA for delegated credentials
	if pki.operatorCert == nil {
		return "", "", constants.ErrPKIOperatorCANotLoaded
	}

	// Load CA private key on-demand for signing
	var caKey *ecdsa.PrivateKey
	if err := pki.loadCAPrivateKey(string(constants.CATypeOperator), &caKey); err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
	}

	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", "", constants.ErrPKIInvalidCSR
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIParseCSR, err)
	}

	if err := csr.CheckSignature(); err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKICSRSignatureCheck, err)
	}

	// Enforce P-256 curve policy
	if !isCurveP256(csr.PublicKey) {
		return "", "", constants.ErrPKIInvalidCurve
	}

	serial, err := randomSerial()
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    now.Add(-1 * time.Minute),
		NotAfter:     now.Add(1 * time.Hour), // Short-lived: 1 hour
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}

	// Set dual URI SANs: app identity + requestor user identity
	wid := protocol.NewWorkloadIdentity()
	appURI, err := wid.AppSPIFFEURL(appName)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIGenerateSPIFFEURL, err)
	}
	userURI, err := wid.UserSPIFFEURL(userID)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIGenerateSPIFFEURL, err)
	}
	template.URIs = []*url.URL{appURI, userURI}

	certDER, err := x509.CreateCertificate(rand.Reader, template, pki.operatorCert, csr.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKISignCSR, err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	// Build chain: leaf + Operator intermediate + root
	rootPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIReadRootCA, err)
	}
	caPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", constants.ErrPKIReadOperatorCA, err)
	}
	chainPEM = certPEM + string(caPEM) + string(rootPEM)

	return certPEM, chainPEM, nil
}

// ─── private helpers ──────────────────────────────────────────────────────────

func (pki *PKIAuthority) loadCACertificate(certRelPath string, cert **x509.Certificate) error {
	certPEM, err := pki.fileSvc.ReadFile(context.Background(), certRelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadCACertificate, err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return constants.ErrPKIInvalidCertPEM
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIParseCertificate, err)
	}

	if parsedCert.SignatureAlgorithm == x509.PureEd25519 {
		return fmt.Errorf("%w: %s", constants.ErrPKIEd25519CertRejected, parsedCert.Subject.CommonName)
	}

	*cert = parsedCert
	return nil
}

func (pki *PKIAuthority) loadCAPrivateKey(caType string, key **ecdsa.PrivateKey) error {
	if pki.secretManager == nil {
		return constants.ErrPKIPrivateKeyRequired
	}

	keyDER, err := pki.secretManager.GetCAPrivateKey(caType)
	if err != nil {
		return fmt.Errorf("%s %s: %w", constants.ErrPKIPrivateKeyNotFound, caType, err)
	}

	parsedKey, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return fmt.Errorf("%s %s: %w", constants.ErrPKIPrivateKeyParse, caType, err)
	}

	*key = parsedKey
	return nil
}

func (pki *PKIAuthority) generateRootCA(certPath string) error {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateRootCA, err)
	}

	serial, err := randomSerial()
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   constants.RootCACommonName,
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
		return fmt.Errorf("%s: %w", constants.ErrPKICreateCertificate, err)
	}

	rootCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIParseCertificate, err)
	}

	if err := pki.fileSvc.WriteFile(context.Background(), certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWritePEMFile, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(rootKey)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIMarshalPrivateKey, err)
	}
	if pki.secretManager == nil {
		return constants.ErrPKIPrivateKeyRequired
	}
	if err := pki.secretManager.StoreCAPrivateKey(string(constants.CATypeRoot), keyDER); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIStorePrivateKey, err)
	}

	pki.rootCert = rootCert
	pki.rootKey = rootKey
	return nil
}

func (pki *PKIAuthority) generateIntermediateCA(certPath string, parentCert *x509.Certificate, parentKey *ecdsa.PrivateKey, commonName string) error {
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateIntermediateCA, err)
	}

	serial, err := randomSerial()
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
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
		return fmt.Errorf("%s: %w", constants.ErrPKICreateCertificate, err)
	}

	intermediateCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIParseCertificate, err)
	}

	if err := pki.fileSvc.WriteFile(context.Background(), certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWritePEMFile, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(intermediateKey)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIMarshalPrivateKey, err)
	}

	// Determine CA type for keystore storage
	var caType constants.CAType
	switch commonName {
	case hubCommonName:
		caType = constants.CATypeHub
	case operatorCommonName:
		caType = constants.CATypeOperator
	case gatewayPeerCommonName:
		caType = constants.CATypeGatewayPeer
	}

	if pki.secretManager == nil {
		return constants.ErrPKIPrivateKeyRequired
	}
	if caType == "" {
		return fmt.Errorf("%s: %s", constants.ErrPKIUnknownCACommonName, commonName)
	}
	if err := pki.secretManager.StoreCAPrivateKey(string(caType), keyDER); err != nil {
		return fmt.Errorf("%s %s: %w", constants.ErrPKIStorePrivateKey, caType, err)
	}

	// Store in the appropriate field based on common name
	switch commonName {
	case hubCommonName:
		pki.hubCert = intermediateCert
		pki.hubKey = intermediateKey
	case operatorCommonName:
		pki.operatorCert = intermediateCert
		pki.operatorKey = intermediateKey
	case gatewayPeerCommonName:
		pki.gatewayPeerCert = intermediateCert
		pki.gatewayPeerKey = intermediateKey
	}

	return nil
}
func (pki *PKIAuthority) generateServiceCertWithNames(extraIPs []net.IP, extraDNSNames []string) error {
	serviceCertRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)
	chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)

	if pki.hubCert == nil || pki.hubKey == nil {
		return constants.ErrPKIHubCANotLoaded
	}

	serviceKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateServiceCert, err)
	}

	serial, err := randomSerial()
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
	}

	dnsNames := []string{"localhost", "g8e.local", string(constants.SessionTypeOperator)}
	// Add extra DNS names from network identity detection
	if extraDNSNames != nil {
		dnsNames = append(dnsNames, extraDNSNames...)
	}
	ipAddresses := append([]net.IP{net.ParseIP("127.0.0.1")}, extraIPs...)

	// Add URI SAN for workload identity
	wid := protocol.NewWorkloadIdentity()
	hubURL, err := wid.HubSPIFFEURL()
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIGenerateSPIFFEURL, err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   string(constants.ServiceNameOperatorGateway),
			Organization: []string{"g8e"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(servingCertValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		URIs:                  []*url.URL{hubURL},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, pki.hubCert, &serviceKey.PublicKey, pki.hubKey)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKICreateCertificate, err)
	}

	// Write chain PEM (leaf + hub intermediate + root)
	chainPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	hubPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA))
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadHubCA, err)
	}
	rootPEM, err := pki.fileSvc.ReadFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIReadRootCA, err)
	}
	chainPEM = append(chainPEM, hubPEM...)
	chainPEM = append(chainPEM, rootPEM...)
	// Write chain PEM directly without re-encoding (chainPEM is already concatenated PEM blocks)
	if err := pki.fileSvc.WriteFile(context.Background(), chainRelPath, chainPEM, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWritePEMFile, err)
	}

	if err := pki.fileSvc.WriteFile(context.Background(), serviceCertRelPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), constants.PermFilePublic); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIWritePEMFile, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(serviceKey)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIMarshalPrivateKey, err)
	}
	if pki.secretManager == nil {
		return constants.ErrPKIPrivateKeyRequired
	}
	if err := pki.secretManager.StoreServicePrivateKey(string(constants.ServiceNameOperatorGateway), keyDER); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIStoreServiceKey, err)
	}

	return nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrPKIGenerateSerial, err)
	}
	return serial, nil
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

// isCurveP256 checks if a public key uses the P-256 elliptic curve.
func isCurveP256(pubKey interface{}) bool {
	if pk, ok := pubKey.(*ecdsa.PublicKey); ok {
		return pk.Curve == elliptic.P256()
	}
	return false
}

// RenewServiceCert renews the operator-gateway service certificate if it is expiring soon.
// This is called by the background renewal loop in the gateway service.
func (pki *PKIAuthority) RenewServiceCert(extraIPs []net.IP) error {
	return pki.RenewServiceCertWithNames(extraIPs, nil)
}

// RenewServiceCertWithNames renews the operator-gateway service certificate with custom DNS names.
// This is called by the background renewal loop in the gateway service.
func (pki *PKIAuthority) RenewServiceCertWithNames(extraIPs []net.IP, extraDNSNames []string) error {
	pki.mu.Lock()
	defer pki.mu.Unlock()

	// Check if current cert is expiring soon
	if !isExpiringSoon(pki.serviceCert) {
		return nil
	}

	pki.logger.Info("[PKI] Service certificate expiring soon, renewing")

	// Load hub CA private key on-demand for service cert generation
	if pki.hubKey == nil {
		if err := pki.loadCAPrivateKey(string(constants.CATypeHub), &pki.hubKey); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrPKILoadCAPrivateKey, err)
		}
	}

	// Generate new service certificate
	if err := pki.generateServiceCertWithNames(extraIPs, extraDNSNames); err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKIRenewServiceCert, err)
	}

	// Load the newly generated certificate and key
	chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)
	chainPEM, err := pki.fileSvc.ReadFile(context.Background(), chainRelPath)
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
	}
	keyDER, err := pki.secretManager.GetServicePrivateKey(string(constants.ServiceNameOperatorGateway))
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceKey, err)
	}
	tlsCert, err := tls.X509KeyPair(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return fmt.Errorf("%s: %w", constants.ErrPKILoadServiceCert, err)
	}

	// Atomically swap the service certificate
	pki.serviceCert = tlsCert

	pki.logger.Info("[PKI] Service certificate renewed successfully")
	return nil
}

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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
)

const (
	caValidityDays     = 3650
	serverValidityDays = 90
	caCommonName       = "g8e Operator CA"
	serverCommonName   = "g8e.local"
)

// CertStore manages the CA and server certificates for listen mode.
// On first start it generates a self-signed CA and a server certificate
// signed by that CA, persisting both to dataDir/ssl/. On subsequent starts
// it loads them from disk. The server certificate is renewed automatically
// when it expires within renewThresholdDays.
type CertStore struct {
	mu     sync.RWMutex
	logger *slog.Logger

	sslDir string

	caCert     *x509.Certificate
	caKey      *ecdsa.PrivateKey
	serverCert tls.Certificate
}

func newCertStore(dataDir, sslDir string, logger *slog.Logger) *CertStore {
	if sslDir == "" {
		sslDir = filepath.Join(dataDir, "ssl")
	}
	return &CertStore{
		sslDir: sslDir,
		logger: logger,
	}
}

// EnsureCerts loads existing certificates or generates new ones. Must be
// called before TLSConfig() or CACertPEM().
func (cs *CertStore) EnsureCerts(extraIPs []net.IP) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if err := os.MkdirAll(filepath.Join(cs.sslDir, "ca"), 0755); err != nil {
		return fmt.Errorf("failed to create ssl dir: %w", err)
	}

	caKeyPath := filepath.Join(cs.sslDir, "ca", "ca.key")
	caCertPath := filepath.Join(cs.sslDir, "ca", "ca.crt")
	caCertRootPath := filepath.Join(cs.sslDir, "ca.crt")
	serverKeyPath := filepath.Join(cs.sslDir, "server.key")
	serverCertPath := filepath.Join(cs.sslDir, "server.crt")

	needCA := !fileExists(caKeyPath) || !fileExists(caCertPath)
	if !needCA {
		if err := cs.loadCA(caCertPath, caKeyPath); err != nil {
			cs.logger.Warn("[CERTS] Failed to load existing CA, regenerating", "error", err)
			needCA = true
		}
	}

	if needCA {
		cs.logger.Info("[CERTS] Generating new CA certificate")
		if err := cs.generateCA(caKeyPath, caCertPath); err != nil {
			return fmt.Errorf("CA generation failed: %w", err)
		}
		// Mirror ca.crt to ssl root for g8ee/VSOD consumers
		caCertPEM, err := os.ReadFile(caCertPath)
		if err != nil {
			return fmt.Errorf("failed to read generated CA cert: %w", err)
		}
		if err := os.WriteFile(caCertRootPath, caCertPEM, 0644); err != nil {
			return fmt.Errorf("failed to mirror CA cert to ssl root: %w", err)
		}
	} else {
		// Ensure mirror exists even on subsequent starts
		caCertPEM, _ := os.ReadFile(caCertPath)
		if len(caCertPEM) > 0 && !fileExists(caCertRootPath) {
			_ = os.WriteFile(caCertRootPath, caCertPEM, 0644)
		}
	}

	needServer := !fileExists(serverKeyPath) || !fileExists(serverCertPath) || needCA
	if !needServer {
		tlsCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			cs.logger.Warn("[CERTS] Failed to load existing server cert, regenerating", "error", err)
			needServer = true
		} else {
			if isExpiringSoon(tlsCert) {
				cs.logger.Info("[CERTS] Server certificate expiring soon, renewing")
				needServer = true
			} else {
				cs.serverCert = tlsCert
			}
		}
	}

	if needServer {
		cs.logger.Info("[CERTS] Generating new server certificate")
		if err := cs.generateServerCert(serverKeyPath, serverCertPath, extraIPs); err != nil {
			return fmt.Errorf("server cert generation failed: %w", err)
		}
		tlsCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			return fmt.Errorf("failed to load generated server cert: %w", err)
		}
		cs.serverCert = tlsCert
	}

	cs.logger.Info("[CERTS] Certificates ready", "ssl_dir", cs.sslDir)
	return nil
}

// TLSConfig returns a tls.Config that serves the managed server certificate.
func (cs *CertStore) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cs.mu.RLock()
			defer cs.mu.RUnlock()
			c := cs.serverCert
			return &c, nil
		},
		MinVersion: tls.VersionTLS12,
	}
}

// CACertPEM returns the PEM-encoded CA certificate bytes.
func (cs *CertStore) CACertPEM() ([]byte, error) {
	path := filepath.Join(cs.sslDir, "ca.crt")
	return os.ReadFile(path)
}

// SSLDir returns the path to the ssl directory within the data dir.
func (cs *CertStore) SSLDir() string {
	return cs.sslDir
}

// ─── private helpers ──────────────────────────────────────────────────────────

func (cs *CertStore) loadCA(certPath, keyPath string) error {
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
		return fmt.Errorf("invalid CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("invalid CA key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		keyIface, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return fmt.Errorf("parse CA key: %w (also tried PKCS8: %v)", err, err2)
		}
		var ok bool
		caKey, ok = keyIface.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("CA key is not ECDSA")
		}
	}

	cs.caCert = caCert
	cs.caKey = caKey
	return nil
}

func (cs *CertStore) generateCA(keyPath, certPath string) error {
	caKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
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
			CommonName:   caCommonName,
			Organization: []string{"g8e.local"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(caValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	caCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	if err := writePEMFile(certPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return err
	}
	if err := writePEMFile(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}

	cs.caCert = caCert
	cs.caKey = caKey
	return nil
}

func (cs *CertStore) generateServerCert(keyPath, certPath string, extraIPs []net.IP) error {
	if cs.caCert == nil || cs.caKey == nil {
		return fmt.Errorf("CA not loaded — call EnsureCerts first")
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	dnsNames := []string{"g8e.local", "localhost", "vsodb", constants.Status.ComponentName.G8EE, constants.Status.ComponentName.VSOD}
	ipAddresses := append([]net.IP{net.ParseIP("127.0.0.1")}, extraIPs...)

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   serverCommonName,
			Organization: []string{"g8e.local"},
			Country:      []string{"US"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(time.Duration(serverValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, cs.caCert, &serverKey.PublicKey, cs.caKey)
	if err != nil {
		return err
	}

	if err := writePEMFile(certPath, "CERTIFICATE", certDER, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return err
	}
	if err := writePEMFile(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}

	return nil
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

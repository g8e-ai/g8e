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
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// PeerConnectionManager manages outbound-only peer connections to a seed gateway.
// It implements the outbound-only invariant: it never opens an inbound listener.
type PeerConnectionManager struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *CanonicalDBService
	pki    *PKIAuthority

	seedURL      string
	gatewayID    string
	peerCert     tls.Certificate
	peerKey      *ecdsa.PrivateKey
	peerCertPEM  string
	peerChainPEM string

	mu            sync.Mutex
	client        *http.Client
	connected     bool
	lastConnectAt time.Time
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewPeerConnectionManager creates a new peer connection manager.
// If seedURL is empty, the manager operates in standalone mode (no federation).
func NewPeerConnectionManager(cfg *config.Config, logger *slog.Logger, db *CanonicalDBService, pki *PKIAuthority) *PeerConnectionManager {
	return &PeerConnectionManager{
		cfg:    cfg,
		logger: logger,
		db:     db,
		pki:    pki,
	}
}

// Start initializes the peer connection manager and begins the connection loop.
// If no seed URL is configured, this is a no-op (standalone mode).
func (pcm *PeerConnectionManager) Start(ctx context.Context) error {
	pcm.seedURL = pcm.cfg.Gateway.FederationSeedURL
	if pcm.seedURL == "" {
		pcm.logger.Info("[Federation] No seed URL configured, running in standalone mode")
		return nil
	}

	// Validate seed URL
	parsedURL, err := url.Parse(pcm.seedURL)
	if err != nil {
		return fmt.Errorf("gateway: invalid seed URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("gateway: seed URL must use HTTPS scheme")
	}

	// Load or generate gateway ID
	pcm.gatewayID, err = pcm.loadGatewayID()
	if err != nil {
		pcm.gatewayID, err = pcm.generateAndStoreGatewayID()
		if err != nil {
			return fmt.Errorf("gateway: initialize gateway ID: %w", err)
		}
	}

	// Load peer certificate, or enroll if not available/expired
	err = pcm.loadPeerCert()
	if err != nil {
		pcm.logger.Info("[Federation] Peer certificate not available or expired, enrolling new certificate")
		if err := pcm.enrollPeerCert(); err != nil {
			return fmt.Errorf("gateway: initialize peer certificate: %w", err)
		}
	}

	// Create context for connection loop
	ctx, cancel := context.WithCancel(ctx)
	pcm.cancel = cancel

	// Start connection loop
	pcm.wg.Add(1)
	go pcm.connectionLoop(ctx)

	pcm.logger.Info("[Federation] Peer connection manager started", "seed_url", pcm.seedURL, "gateway_id", pcm.gatewayID)
	return nil
}

// Stop gracefully shuts down the peer connection manager.
func (pcm *PeerConnectionManager) Stop() {
	if pcm.cancel != nil {
		pcm.cancel()
	}
	pcm.wg.Wait()
	pcm.logger.Info("[Federation] Peer connection manager stopped")
}

// IsConnected returns whether the peer is currently connected to the seed.
func (pcm *PeerConnectionManager) IsConnected() bool {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()
	return pcm.connected
}

// loadGatewayID loads the gateway ID from disk.
func (pcm *PeerConnectionManager) loadGatewayID() (string, error) {
	gatewayIDPath := constants.GatewayIDPath

	data, err := os.ReadFile(gatewayIDPath)
	if err != nil {
		return "", fmt.Errorf("gateway: load gateway ID: %w", err)
	}

	id := string(data)
	if id == "" {
		return "", fmt.Errorf("gateway: gateway ID file is empty")
	}

	pcm.logger.Debug("[Federation] Loaded existing gateway ID", "gateway_id", id)
	return id, nil
}

// generateAndStoreGatewayID generates a new gateway ID and stores it to disk.
func (pcm *PeerConnectionManager) generateAndStoreGatewayID() (string, error) {
	gatewayIDPath := constants.GatewayIDPath

	id, err := generateGatewayID()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(gatewayIDPath, []byte(id), 0600); err != nil {
		return "", fmt.Errorf("gateway: write gateway ID: %w", err)
	}

	pcm.logger.Info("[Federation] Generated new gateway ID", "gateway_id", id)
	return id, nil
}

// loadPeerCert loads the peer certificate from disk.
func (pcm *PeerConnectionManager) loadPeerCert() error {
	peerCertPath := constants.PeerCertPath
	peerKeyPath := constants.PeerKeyPath
	peerChainPath := constants.PeerChainPath

	certPEM, err := os.ReadFile(peerCertPath)
	if err != nil {
		return fmt.Errorf("gateway: read peer certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(peerKeyPath)
	if err != nil {
		return fmt.Errorf("gateway: read peer key: %w", err)
	}

	chainPEM, err := os.ReadFile(peerChainPath)
	if err != nil {
		return fmt.Errorf("gateway: read peer chain: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("gateway: parse peer certificate/key pair: %w", err)
	}

	if isExpiringSoon(cert) {
		return fmt.Errorf("gateway: peer certificate is expiring soon")
	}

	pcm.peerCert = cert
	pcm.peerCertPEM = string(certPEM)
	pcm.peerChainPEM = string(chainPEM)
	pcm.logger.Debug("[Federation] Loaded existing peer certificate")
	return nil
}

// enrollPeerCert generates a new keypair and enrolls for a peer certificate from the seed.
func (pcm *PeerConnectionManager) enrollPeerCert() error {
	// Generate P-256 keypair
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("gateway: generate peer keypair: %w", err)
	}

	// Create CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "g8e-gateway-peer",
			Organization: []string{"g8e"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, key)
	if err != nil {
		return fmt.Errorf("gateway: create CSR: %w", err)
	}
	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	// Submit CSR to seed for signing
	certPEM, chainPEM, err := pcm.submitCSRToSeed(csrPEM)
	if err != nil {
		return fmt.Errorf("gateway: submit CSR to seed: %w", err)
	}

	// Store certificate and key
	peerDir := filepath.Dir(constants.PeerCertPath)
	if err := os.MkdirAll(peerDir, 0755); err != nil {
		return fmt.Errorf("gateway: create peer directory: %w", err)
	}

	peerCertPath := constants.PeerCertPath
	peerKeyPath := constants.PeerKeyPath
	peerChainPath := constants.PeerChainPath

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("gateway: marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := writePEMFile(peerCertPath, "CERTIFICATE", []byte(certPEM), 0600); err != nil {
		return fmt.Errorf("gateway: write peer certificate: %w", err)
	}
	if err := writePEMFile(peerKeyPath, "EC PRIVATE KEY", keyPEM, 0600); err != nil {
		return fmt.Errorf("gateway: write peer key: %w", err)
	}
	if err := writePEMFile(peerChainPath, "", []byte(chainPEM), 0600); err != nil {
		return fmt.Errorf("gateway: write peer chain: %w", err)
	}

	// Load into memory
	cert, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return fmt.Errorf("gateway: load certificate/key pair: %w", err)
	}

	pcm.peerCert = cert
	pcm.peerKey = key
	pcm.peerCertPEM = certPEM
	pcm.peerChainPEM = chainPEM

	pcm.logger.Info("[Federation] Enrolled new peer certificate from seed")
	return nil
}

// submitCSRToSeed submits a CSR to the seed gateway for signing.
func (pcm *PeerConnectionManager) submitCSRToSeed(csrPEM string) (certPEM string, chainPEM string, err error) {
	certPEM, chainPEM, err = pcm.pki.SignCSR(csrPEM, "gateway-peer", "", "", "", "", pcm.gatewayID)
	if err != nil {
		return "", "", fmt.Errorf("gateway: submit CSR to seed: %w", err)
	}
	return certPEM, chainPEM, nil
}

// connectionLoop maintains the connection to the seed with reconnect/backoff.
func (pcm *PeerConnectionManager) connectionLoop(ctx context.Context) {
	defer pcm.wg.Done()

	backoff := time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := pcm.connect(); err != nil {
			pcm.logger.Warn("[Federation] Failed to connect to seed", "error", err, "backoff", backoff)
			pcm.mu.Lock()
			pcm.connected = false
			pcm.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = calculateBackoff(backoff, maxBackoff)
			}
			continue
		}

		// Connection successful, reset backoff
		backoff = time.Second
		pcm.mu.Lock()
		pcm.connected = true
		pcm.lastConnectAt = time.Now()
		pcm.mu.Unlock()

		pcm.logger.Info("[Federation] Connected to seed gateway")

		// Maintain connection with periodic health checks
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

	healthCheckLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := pcm.healthCheck(); err != nil {
					pcm.logger.Warn("[Federation] Health check failed, reconnecting", "error", err)
					pcm.mu.Lock()
					pcm.connected = false
					pcm.mu.Unlock()
					break healthCheckLoop
				}
			}
		}
	}
}

// connect establishes a connection to the seed gateway.
func (pcm *PeerConnectionManager) connect() error {
	// Build TLS config with peer certificate
	peerCertPool := x509.NewCertPool()
	peerCertPool.AppendCertsFromPEM([]byte(pcm.peerChainPEM))

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{pcm.peerCert},
		RootCAs:      peerCertPool,
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	pcm.mu.Lock()
	pcm.client = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	pcm.mu.Unlock()

	// Perform a simple health check to verify connection
	if err := pcm.healthCheck(); err != nil {
		return fmt.Errorf("gateway: connect: %w", err)
	}
	return nil
}

// healthCheck performs a health check against the seed gateway.
func (pcm *PeerConnectionManager) healthCheck() error {
	pcm.mu.Lock()
	client := pcm.client
	pcm.mu.Unlock()

	if client == nil {
		return fmt.Errorf("gateway: health check: client not initialized")
	}

	healthURL := pcm.seedURL + "/.well-known/g8e/federation/health"
	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("gateway: health check: create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway: health check: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway: health check: unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// generateGatewayID generates a unique gateway ID.
func generateGatewayID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gateway: generate gateway ID: %w", err)
	}
	return fmt.Sprintf("gw-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:16]), nil
}

// calculateBackoff calculates exponential backoff with a maximum limit.
func calculateBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next < max {
		return next
	}
	return max
}

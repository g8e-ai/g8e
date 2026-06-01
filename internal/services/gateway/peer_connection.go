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
)

// PeerConnectionManager manages outbound-only peer connections to a seed gateway.
// It implements the outbound-only invariant: it never opens an inbound listener.
type PeerConnectionManager struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *GatewayDBService
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
func NewPeerConnectionManager(cfg *config.Config, logger *slog.Logger, db *GatewayDBService, pki *PKIAuthority) *PeerConnectionManager {
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
		return fmt.Errorf("invalid seed URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("seed URL must use HTTPS scheme")
	}

	// Generate or load gateway ID
	pcm.gatewayID, err = pcm.loadOrGenerateGatewayID()
	if err != nil {
		return fmt.Errorf("failed to initialize gateway ID: %w", err)
	}

	// Load or generate peer certificate
	if err := pcm.loadOrGeneratePeerCert(); err != nil {
		return fmt.Errorf("failed to initialize peer certificate: %w", err)
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

// loadOrGenerateGatewayID loads the gateway ID from disk or generates a new one.
func (pcm *PeerConnectionManager) loadOrGenerateGatewayID() (string, error) {
	gatewayIDPath := filepath.Join(pcm.cfg.Gateway.DataDir, "gateway-id")

	// Try to load existing ID
	if data, err := os.ReadFile(gatewayIDPath); err == nil {
		id := string(data)
		if id != "" {
			pcm.logger.Debug("[Federation] Loaded existing gateway ID", "gateway_id", id)
			return id, nil
		}
	}

	// Generate new ID
	id := generateGatewayID()
	if err := os.WriteFile(gatewayIDPath, []byte(id), 0600); err != nil {
		return "", fmt.Errorf("failed to write gateway ID: %w", err)
	}

	pcm.logger.Info("[Federation] Generated new gateway ID", "gateway_id", id)
	return id, nil
}

// loadOrGeneratePeerCert loads the peer certificate from disk or enrolls for a new one.
func (pcm *PeerConnectionManager) loadOrGeneratePeerCert() error {
	peerCertPath := filepath.Join(pcm.cfg.Gateway.PKIDir, "peer", "peer.crt")
	peerKeyPath := filepath.Join(pcm.cfg.Gateway.PKIDir, "peer", "peer.key")
	peerChainPath := filepath.Join(pcm.cfg.Gateway.PKIDir, "peer", "peer.chain.pem")

	// Try to load existing certificate
	if fileExists(peerCertPath) && fileExists(peerKeyPath) {
		certPEM, err := os.ReadFile(peerCertPath)
		if err != nil {
			return fmt.Errorf("failed to read peer certificate: %w", err)
		}

		keyPEM, err := os.ReadFile(peerKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read peer key: %w", err)
		}

		chainPEM, err := os.ReadFile(peerChainPath)
		if err != nil {
			return fmt.Errorf("failed to read peer chain: %w", err)
		}

		// Parse certificate and key
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("failed to parse peer certificate/key pair: %w", err)
		}

		// Check if certificate is expiring soon
		if !isExpiringSoon(cert) {
			pcm.peerCert = cert
			pcm.peerCertPEM = string(certPEM)
			pcm.peerChainPEM = string(chainPEM)
			pcm.logger.Debug("[Federation] Loaded existing peer certificate")
			return nil
		}

		pcm.logger.Info("[Federation] Peer certificate expiring soon, will renew")
	}

	// Generate new keypair and enroll
	return pcm.enrollPeerCert()
}

// enrollPeerCert generates a new keypair and enrolls for a peer certificate from the seed.
func (pcm *PeerConnectionManager) enrollPeerCert() error {
	// Generate P-256 keypair
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate peer keypair: %w", err)
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
		return fmt.Errorf("failed to create CSR: %w", err)
	}
	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	// Submit CSR to seed for signing
	certPEM, chainPEM, err := pcm.submitCSRToSeed(csrPEM)
	if err != nil {
		return fmt.Errorf("failed to submit CSR to seed: %w", err)
	}

	// Store certificate and key
	peerDir := filepath.Join(pcm.cfg.Gateway.PKIDir, "peer")
	if err := os.MkdirAll(peerDir, 0755); err != nil {
		return fmt.Errorf("failed to create peer directory: %w", err)
	}

	peerCertPath := filepath.Join(peerDir, "peer.crt")
	peerKeyPath := filepath.Join(peerDir, "peer.key")
	peerChainPath := filepath.Join(peerDir, "peer.chain.pem")

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(peerCertPath, []byte(certPEM), 0600); err != nil {
		return fmt.Errorf("failed to write peer certificate: %w", err)
	}
	if err := os.WriteFile(peerKeyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write peer key: %w", err)
	}
	if err := os.WriteFile(peerChainPath, []byte(chainPEM), 0600); err != nil {
		return fmt.Errorf("failed to write peer chain: %w", err)
	}

	// Load into memory
	cert, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return fmt.Errorf("failed to load certificate/key pair: %w", err)
	}

	pcm.peerCert = cert
	pcm.peerKey = key
	pcm.peerCertPEM = certPEM
	pcm.peerChainPEM = chainPEM

	pcm.logger.Info("[Federation] Enrolled new peer certificate from seed")
	return nil
}

// submitCSRToSeed submits a CSR to the seed gateway for signing.
// This is a placeholder - the actual implementation will depend on the seed's enrollment API.
func (pcm *PeerConnectionManager) submitCSRToSeed(csrPEM string) (certPEM string, chainPEM string, err error) {
	// TODO: Implement actual seed enrollment API call
	// For now, use local PKI for testing (this will be replaced with seed API)
	certPEM, chainPEM, err = pcm.pki.SignCSR(csrPEM, "gateway-peer", "", "", "", "", pcm.gatewayID)
	if err != nil {
		return "", "", fmt.Errorf("local PKI signing failed (will be replaced with seed API): %w", err)
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
				backoff = min(backoff*2, maxBackoff)
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
				if !pcm.healthCheck() {
					pcm.logger.Warn("[Federation] Health check failed, reconnecting")
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
	if !pcm.healthCheck() {
		return fmt.Errorf("health check failed")
	}
	return nil
}

// healthCheck performs a health check against the seed gateway.
func (pcm *PeerConnectionManager) healthCheck() bool {
	pcm.mu.Lock()
	client := pcm.client
	pcm.mu.Unlock()

	if client == nil {
		return false
	}

	// TODO: Replace with actual seed health check endpoint
	// For now, just verify we can make a request
	healthURL := pcm.seedURL + "/.well-known/g8e/federation/health"
	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// generateGatewayID generates a unique gateway ID.
func generateGatewayID() string {
	// Simple UUID-like generation
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("gw-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:16])
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

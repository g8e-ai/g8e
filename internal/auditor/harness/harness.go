// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package harness provides test infrastructure for starting self-contained
// g8e gateway and operator instances for auditor tests. Zero external dependencies.
package harness

import (
	"bytes"
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
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestHarness manages a self-contained gateway instance in gateway mode.
type TestHarness struct {
	mu            sync.Mutex
	tempDir       string
	gatewayCmd    *exec.Cmd
	ctx           context.Context
	cancel        context.CancelFunc
	ready         bool
	GatewayPort   int
	BootstrapPort int
	PublicPort    int
	DataDir       string
	PKIDir        string
	SecretsDir    string
	Binary        string
}

// Config holds configuration for the test harness.
type Config struct {
	GatewayPort   int
	BootstrapPort int
	PublicPort    int
	Binary        string
	Posture       string // "doctrine", "consensus", or "notary"
}

// DefaultConfig returns a default test harness configuration.
func DefaultConfig() Config {
	return Config{
		GatewayPort:   18440, // Non-standard port to avoid conflicts
		BootstrapPort: 18441,
		PublicPort:    18442,
		Binary:        "./bin/g8e",
		Posture:       "doctrine", // L1 enforced, L2/L3 audited (fastest for tests)
	}
}

// New creates a new test harness with temporary PKI material.
func New(cfg Config) (*TestHarness, error) {
	// Create temp directory in current working directory instead of /tmp
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	// Generate unique suffix using timestamp and random bytes
	randomBytes := make([]byte, 2)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	randomNum := int(randomBytes[0])<<8 | int(randomBytes[1])
	uniqueSuffix := fmt.Sprintf("%d-%d", time.Now().UnixNano(), randomNum%10000)
	tempDir := filepath.Join(wd, fmt.Sprintf(".g8e-harness-%s", uniqueSuffix))

	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	dataDir := filepath.Join(tempDir, "data")
	pkiDir := filepath.Join(tempDir, "pki")
	secretsDir := filepath.Join(tempDir, "secrets")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(pkiDir, 0o755); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("create pki dir: %w", err)
	}
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("create secrets dir: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TestHarness{
		tempDir:       tempDir,
		ctx:           ctx,
		cancel:        cancel,
		GatewayPort:   cfg.GatewayPort,
		BootstrapPort: cfg.BootstrapPort,
		PublicPort:    cfg.PublicPort,
		DataDir:       dataDir,
		PKIDir:        pkiDir,
		SecretsDir:    secretsDir,
		Binary:        cfg.Binary,
	}, nil
}

// Start launches the gateway in gateway mode (operator + gateway in one process).
func (h *TestHarness) Start(posture string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ready {
		return fmt.Errorf("harness already started")
	}

	// Start gateway in gateway mode (operator + gateway combined)
	gatewayArgs := []string{
		"--" + posture, // --doctrine, --consensus, or --notary
		"--http-listen-port", fmt.Sprintf("%d", h.GatewayPort),
		"--bootstrap-listen-port", fmt.Sprintf("%d", h.BootstrapPort),
		"--public-listen-port", fmt.Sprintf("%d", h.PublicPort),
		"--data-dir", h.DataDir,
		"--pki-dir", h.PKIDir,
		"--secrets-dir", h.SecretsDir,
		"--log", "error",
	}

	h.gatewayCmd = exec.CommandContext(h.ctx, h.Binary, gatewayArgs...)
	if os.Getenv("G8E_TEST_VERBOSE") == "1" {
		h.gatewayCmd.Stdout = os.Stdout
		h.gatewayCmd.Stderr = os.Stderr
	} else {
		h.gatewayCmd.Stdout = nil
		h.gatewayCmd.Stderr = nil
	}

	if err := h.gatewayCmd.Start(); err != nil {
		return fmt.Errorf("start gateway: %w", err)
	}

	// Wait for gateway to be ready
	if err := h.waitForPort(h.GatewayPort, 15*time.Second); err != nil {
		h.Stop()
		return fmt.Errorf("gateway not ready: %w", err)
	}

	// Wait for public port to be ready (PKI endpoint is here)
	if err := h.waitForPort(h.PublicPort, 15*time.Second); err != nil {
		h.Stop()
		return fmt.Errorf("public port not ready: %w", err)
	}

	h.ready = true
	log.Printf("Test harness ready: gateway on %d, public on %d", h.GatewayPort, h.PublicPort)
	return nil
}

// Stop terminates the gateway subprocess and cleans up temp files.
func (h *TestHarness) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
	}

	if h.gatewayCmd != nil {
		if err := h.gatewayCmd.Process.Kill(); err != nil {
			log.Printf("warning: failed to kill gateway: %v", err)
		}
		_ = h.gatewayCmd.Wait()
	}

	if h.tempDir != "" {
		os.RemoveAll(h.tempDir)
	}

	h.ready = false
	log.Println("Test harness stopped and cleaned up")
}

// GatewayURL returns the gateway mTLS URL.
func (h *TestHarness) GatewayURL() string {
	return fmt.Sprintf("https://localhost:%d", h.GatewayPort)
}

// PublicURL returns the gateway public URL.
func (h *TestHarness) PublicURL() string {
	return fmt.Sprintf("https://localhost:%d", h.PublicPort)
}

func (h *TestHarness) waitForTrustBundle(timeout time.Duration) (string, error) {
	trustBundlePath := filepath.Join(h.PKIDir, "trust", "g8e-gw-ca-bundle.pem")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(trustBundlePath); err == nil {
			return trustBundlePath, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("trust bundle not found at %s after %v", trustBundlePath, timeout)
}

func (h *TestHarness) waitForPKIEndpoint(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/.well-known/g8e/pki/ca-bundle", h.PublicPort))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("PKI endpoint not ready after %v", timeout)
}

// waitForPort blocks until a port is accepting connections.
func (h *TestHarness) waitForPort(port int, timeout time.Duration) error {
	// Use the gateway's trust bundle for CA verification
	trustBundlePath, err := h.waitForTrustBundle(timeout)
	if err != nil {
		return fmt.Errorf("trust bundle not ready: %w", err)
	}

	caCert, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return fmt.Errorf("read trust bundle: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to parse trust bundle")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := tls.Dial(string(constants.NetworkProtocolTCP), fmt.Sprintf("localhost:%d", port), &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS13,
		})
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %d not ready after %v", port, timeout)
}

// EnrollTestClient generates a CSR and enrolls it with the gateway's real PKI system.
// Returns paths to the client cert, key, and CA bundle.
func (h *TestHarness) EnrollTestClient(userID, sessionID string) (certPath, keyPath, caBundlePath string, err error) {
	// Wait for trust bundle file to exist (PKI authority must finish initialization)
	trustBundlePath, err := h.waitForTrustBundle(15 * time.Second)
	if err != nil {
		return "", "", "", fmt.Errorf("trust bundle not ready: %w", err)
	}

	// Then wait for PKI HTTP endpoint to be responsive
	if err := h.waitForPKIEndpoint(10 * time.Second); err != nil {
		return "", "", "", fmt.Errorf("PKI endpoint not ready: %w", err)
	}

	// Generate client key and CSR
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("generate client key: %w", err)
	}

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"g8e Test Harness"},
			CommonName:   "test-auditor-client",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, priv)
	if err != nil {
		return "", "", "", fmt.Errorf("create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	// Submit CSR to gateway's real PKI signing endpoint
	signURL := fmt.Sprintf("https://localhost:%d/api/v1/pki/csr/sign", h.GatewayPort)
	signReq := map[string]string{
		"csr_pem":             string(csrPEM),
		"leaf_type":           "cli",
		"user_id":             userID,
		"workload_session_id": sessionID,
	}
	reqBody, _ := json.Marshal(signReq)

	// Use the trust bundle for TLS verification
	caCert, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return "", "", "", fmt.Errorf("read trust bundle: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return "", "", "", fmt.Errorf("failed to parse trust bundle")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caCertPool,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS13,
			},
		},
		Timeout: 30 * time.Second,
	}

	maxRetries := 5
	baseDelay := 100 * time.Millisecond
	var resp *http.Response
	var reqErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, reqErr = client.Post(signURL, "application/json", bytes.NewReader(reqBody))
		if reqErr == nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return "", "", "", fmt.Errorf("CSR signing failed: status %d, body: %s", resp.StatusCode, string(body))
			}
			resp.Body.Close()
		}

		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<uint(attempt))
			time.Sleep(delay)
		}
	}
	if reqErr != nil {
		return "", "", "", fmt.Errorf("submit CSR to gateway: %w", reqErr)
	}
	if resp == nil {
		return "", "", "", fmt.Errorf("submit CSR to gateway: response was nil")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", fmt.Errorf("CSR signing failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var signResp struct {
		CertificatePEM      string `json:"certificate_pem"`
		CertificateChainPEM string `json:"certificate_chain_pem"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signResp); err != nil {
		return "", "", "", fmt.Errorf("decode sign response: %w", err)
	}

	// Save client cert and key
	certPath = filepath.Join(h.tempDir, "test-client.crt")
	keyPath = filepath.Join(h.tempDir, "test-client.key")

	if err := os.WriteFile(certPath, []byte(signResp.CertificatePEM), 0600); err != nil {
		return "", "", "", fmt.Errorf("write client cert: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return "", "", "", fmt.Errorf("write client key: %w", err)
	}

	return certPath, keyPath, trustBundlePath, nil
}

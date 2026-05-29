// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package harness provides test infrastructure for starting self-contained
// g8e gateway and operator instances for auditor tests. Zero external dependencies.
package harness

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
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// TestHarness manages self-contained gateway and operator instances.
type TestHarness struct {
	mu             sync.Mutex
	tempDir        string
	gatewayCmd     *exec.Cmd
	operatorCmd    *exec.Cmd
	ctx            context.Context
	cancel         context.CancelFunc
	ready          bool
	GatewayPort    int
	OperatorPort   int
	PublicPort     int
	DataDir        string
	PKIDir         string
	SecretsDir     string
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
	GatewayBinary  string
	OperatorBinary string
}

// Config holds configuration for the test harness.
type Config struct {
	GatewayPort    int
	OperatorPort   int
	PublicPort     int
	GatewayBinary  string
	OperatorBinary string
	Posture        string // "doctrine", "consensus", or "notary"
}

// DefaultConfig returns a default test harness configuration.
func DefaultConfig() Config {
	return Config{
		GatewayPort:    18440, // Non-standard port to avoid conflicts
		OperatorPort:   18441,
		PublicPort:     18442,
		GatewayBinary:  "./bin/g8e",
		OperatorBinary: "./bin/g8e",
		Posture:        "doctrine", // L1 enforced, L2/L3 audited (fastest for tests)
	}
}

// New creates a new test harness with temporary PKI material.
func New(cfg Config) (*TestHarness, error) {
	tempDir, err := os.MkdirTemp("", "g8ea-harness-*")
	if err != nil {
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

	// Generate test PKI
	caCertPath, caKeyPath, err := generateCA(pkiDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// Generate server certs for gateway and operator (using same hostname for localhost testing)
	_, _, err = generateServerCert(pkiDir, caCertPath, caKeyPath, "localhost")
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("generate server cert: %w", err)
	}

	clientCertPath, clientKeyPath, err := generateClientCert(pkiDir, caCertPath, caKeyPath, "auditor-client")
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("generate client cert: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TestHarness{
		tempDir:        tempDir,
		ctx:            ctx,
		cancel:         cancel,
		GatewayPort:    cfg.GatewayPort,
		OperatorPort:   cfg.OperatorPort,
		PublicPort:     cfg.PublicPort,
		DataDir:        dataDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CACertPath:     caCertPath,
		ClientCertPath: clientCertPath,
		ClientKeyPath:  clientKeyPath,
		GatewayBinary:  cfg.GatewayBinary,
		OperatorBinary: cfg.OperatorBinary,
	}, nil
}

// Start launches both gateway and operator subprocesses.
func (h *TestHarness) Start(posture string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ready {
		return fmt.Errorf("harness already started")
	}

	// Start operator first (gateway needs to connect to it)
	operatorArgs := []string{
		"--http-port", fmt.Sprintf("%d", h.OperatorPort),
		"--working-dir", h.tempDir,
		"--data-dir", h.DataDir,
		"--pki-dir", h.PKIDir,
		"--secrets-dir", h.SecretsDir,
		"--log", "error", // Minimal logging for tests
	}

	h.operatorCmd = exec.CommandContext(h.ctx, h.OperatorBinary, operatorArgs...)
	h.operatorCmd.Stdout = nil
	h.operatorCmd.Stderr = nil

	if err := h.operatorCmd.Start(); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}

	// Wait for operator to be ready
	if err := h.waitForPort(h.OperatorPort, 10*time.Second); err != nil {
		h.Stop()
		return fmt.Errorf("operator not ready: %w", err)
	}

	// Start gateway
	gatewayArgs := []string{
		"--" + posture, // --doctrine, --consensus, or --notary
		"--http-listen-port", fmt.Sprintf("%d", h.GatewayPort),
		"--bootstrap-listen-port", fmt.Sprintf("%d", h.OperatorPort),
		"--public-listen-port", fmt.Sprintf("%d", h.PublicPort),
		"--data-dir", h.DataDir,
		"--pki-dir", h.PKIDir,
		"--secrets-dir", h.SecretsDir,
		"--log", "error",
	}

	h.gatewayCmd = exec.CommandContext(h.ctx, h.GatewayBinary, gatewayArgs...)
	h.gatewayCmd.Stdout = nil
	h.gatewayCmd.Stderr = nil

	if err := h.gatewayCmd.Start(); err != nil {
		h.Stop()
		return fmt.Errorf("start gateway: %w", err)
	}

	// Wait for gateway to be ready
	if err := h.waitForPort(h.GatewayPort, 15*time.Second); err != nil {
		h.Stop()
		return fmt.Errorf("gateway not ready: %w", err)
	}

	h.ready = true
	log.Printf("Test harness ready: gateway on %d, operator on %d", h.GatewayPort, h.OperatorPort)
	return nil
}

// Stop terminates both subprocesses and cleans up temp files.
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
	if h.operatorCmd != nil {
		if err := h.operatorCmd.Process.Kill(); err != nil {
			log.Printf("warning: failed to kill operator: %v", err)
		}
		_ = h.operatorCmd.Wait()
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

// waitForPort blocks until a port is accepting connections.
func (h *TestHarness) waitForPort(port int, timeout time.Duration) error {
	caCert, err := os.ReadFile(h.CACertPath)
	if err != nil {
		return fmt.Errorf("read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to parse CA cert")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := tls.Dial("tcp", fmt.Sprintf("localhost:%d", port), &tls.Config{
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

// generateCA creates a test CA certificate and key.
func generateCA(dir string) (certPath, keyPath string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"g8e Test Harness"},
			CommonName:   "g8e Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("create CA cert: %w", err)
	}

	certPath = filepath.Join(dir, "ca.crt")
	keyPath = filepath.Join(dir, "ca.key")

	if err := writePEM(certPath, "CERTIFICATE", derBytes); err != nil {
		return "", "", err
	}
	if err := writeKeyPEM(keyPath, priv); err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

// generateServerCert creates a server certificate signed by the CA.
func generateServerCert(dir, caCertPath, caKeyPath, hostname string) (certPath, keyPath string, err error) {
	caKey, err := readKeyPEM(caKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("read CA key: %w", err)
	}

	caCert, err := readPEM(caCertPath)
	if err != nil {
		return "", "", fmt.Errorf("read CA cert: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate server key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"g8e Test Harness"},
			CommonName:   hostname,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{hostname},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create server cert: %w", err)
	}

	certPath = filepath.Join(dir, hostname+".crt")
	keyPath = filepath.Join(dir, hostname+".key")

	if err := writePEM(certPath, "CERTIFICATE", derBytes); err != nil {
		return "", "", err
	}
	if err := writeKeyPEM(keyPath, priv); err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

// generateClientCert creates a client certificate signed by the CA.
func generateClientCert(dir, caCertPath, caKeyPath, cn string) (certPath, keyPath string, err error) {
	caKey, err := readKeyPEM(caKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("read CA key: %w", err)
	}

	caCert, err := readPEM(caCertPath)
	if err != nil {
		return "", "", fmt.Errorf("read CA cert: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate client key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"g8e Test Harness"},
			CommonName:   cn,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create client cert: %w", err)
	}

	certPath = filepath.Join(dir, cn+".crt")
	keyPath = filepath.Join(dir, cn+".key")

	if err := writePEM(certPath, "CERTIFICATE", derBytes); err != nil {
		return "", "", err
	}
	if err := writeKeyPEM(keyPath, priv); err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

func writePEM(path, typ string, derBytes []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: typ, Bytes: derBytes})
}

func writeKeyPEM(path string, key *ecdsa.PrivateKey) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	privBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
}

func readPEM(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func readKeyPEM(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key.(*ecdsa.PrivateKey), nil
}

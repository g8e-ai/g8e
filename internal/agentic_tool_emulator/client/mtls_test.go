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

package client

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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/agentic_tool_emulator/config"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestGenerateCA(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	if caCert == nil {
		t.Fatal("CA certificate should not be nil")
	}

	if caPriv == nil {
		t.Fatal("CA private key should not be nil")
	}

	if !caCert.IsCA {
		t.Error("Generated certificate should have IsCA flag set")
	}

	if caCert.Subject.Organization[0] != "g8e Trusted Authority" {
		t.Errorf("Expected organization 'g8e Trusted Authority', got '%s'", caCert.Subject.Organization[0])
	}

	if time.Now().After(caCert.NotAfter) {
		t.Error("CA certificate should not be expired")
	}
}

func TestGenerateCert(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	cert, priv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert failed: %v", err)
	}

	if cert == nil {
		t.Fatal("Certificate should not be nil")
	}

	if priv == nil {
		t.Fatal("Private key should not be nil")
	}

	if cert.Subject.CommonName != "Test Server" {
		t.Errorf("Expected CommonName 'Test Server', got '%s'", cert.Subject.CommonName)
	}

	if cert.IsCA {
		t.Error("Generated server certificate should not have IsCA flag set")
	}

	if len(cert.DNSNames) == 0 || cert.DNSNames[0] != "localhost" {
		t.Error("Certificate should include localhost DNS name")
	}

	if len(cert.IPAddresses) == 0 {
		t.Error("Certificate should include IP addresses")
	}
}

func TestGenerateCert_MultipleCerts(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	if serverCert.Subject.CommonName == clientCert.Subject.CommonName {
		t.Error("Different certificates should have different CommonNames")
	}

	if serverCert.SerialNumber == clientCert.SerialNumber {
		t.Error("Different certificates should have different serial numbers")
	}

	if serverPriv == clientPriv {
		t.Error("Different certificates should have different private keys")
	}
}

func TestMTLSServerClient_Handshake(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Test Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	receivedEnvelope := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var env governance.GovernanceEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("Failed to decode envelope: %v", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if env.ActionType != "TEST_ACTION" {
			t.Errorf("Expected ActionType 'TEST_ACTION', got '%s'", env.ActionType)
		}

		receivedEnvelope = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	clientTLSConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{clientCert.Raw},
			PrivateKey:  clientPriv,
		}},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
		Timeout: 5 * time.Second,
	}

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "test-operator",
		Timestamp:       timestamppb.Now(),
		ActionType:      "TEST_ACTION",
		TargetResource:  "localhost",
		Payload:         []byte("test payload"),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				KeyId: "test-key",
			},
		},
	}

	id, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("Failed to generate MessageID: %v", err)
	}
	env.Id = id

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	resp, err := client.Post(server.URL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("Client POST failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	if !receivedEnvelope {
		t.Error("Server did not receive the envelope")
	}
}

func TestMTLSServerClient_InvalidClientCert(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	clientTLSConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    caPool,
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Error("Expected TLS handshake error with no client certificate")
	}
}

func TestGovernanceEnvelope_Integration(t *testing.T) {
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "test-operator",
		Timestamp:       timestamppb.Now(),
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				KeyId: "test-key",
			},
		},
	}

	id, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("Failed to generate MessageID: %v", err)
	}

	if id == "" {
		t.Fatal("MessageID should not be empty")
	}

	env.Id = id

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	var decoded governance.GovernanceEnvelope
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal envelope: %v", err)
	}

	if decoded.Id != id {
		t.Errorf("Expected ID %s, got %s", id, decoded.Id)
	}

	if decoded.ActionType != "EXECUTE_BASH" {
		t.Errorf("Expected ActionType EXECUTE_BASH, got %s", decoded.ActionType)
	}

	if string(decoded.Payload) != "echo test" {
		t.Errorf("Expected payload 'echo test', got '%s'", string(decoded.Payload))
	}
}

// Helper functions for mTLS test fixtures

func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"g8e Trusted Authority"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certBytes)
	return cert, priv, err
}

func generateCert(caCert *x509.Certificate, caPriv *ecdsa.PrivateKey, commonName string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(1 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caPriv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certBytes)
	return cert, priv, err
}

func TestGetReceipt_Success(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Test Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	transactionID := "test-tx-123"
	expectedReceipt := map[string]interface{}{
		"transaction_id":    transactionID,
		"transaction_hash":  "hash-abc123",
		"action_type":       "EXECUTE_BASH",
		"target_resource":   "localhost",
		"status":            "completed",
		"state_root_before": "root-before",
		"state_root_after":  "root-after",
		"signature":         "sig-def456",
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Query().Get("tx_id") != transactionID {
			http.Error(w, "Transaction ID mismatch", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedReceipt)
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	tempDir := t.TempDir()
	clientCertPath := filepath.Join(tempDir, "client.crt")
	clientKeyPath := filepath.Join(tempDir, "client.key")
	caBundlePath := filepath.Join(tempDir, "ca.pem")

	if err := os.WriteFile(clientCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}
	clientKeyBytes, err := x509.MarshalPKCS8PrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	if err := os.WriteFile(clientKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyBytes}), 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}
	if err := os.WriteFile(caBundlePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write CA bundle: %v", err)
	}

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth: config.Auth{
			ClientCert: clientCertPath,
			ClientKey:  clientKeyPath,
			CABundle:   caBundlePath,
			Insecure:   false,
		},
	}

	testClient, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	receipt, body, err := testClient.GetReceipt(ctx, transactionID)

	if err != nil {
		t.Fatalf("GetReceipt failed: %v", err)
	}

	if receipt == nil {
		t.Fatal("Expected non-nil receipt")
	}

	if receipt.TransactionID != transactionID {
		t.Errorf("Expected TransactionID %s, got %s", transactionID, receipt.TransactionID)
	}

	if receipt.TransactionHash != "hash-abc123" {
		t.Errorf("Expected TransactionHash hash-abc123, got %s", receipt.TransactionHash)
	}

	if receipt.ActionType != "EXECUTE_BASH" {
		t.Errorf("Expected ActionType EXECUTE_BASH, got %s", receipt.ActionType)
	}

	if receipt.Status != "completed" {
		t.Errorf("Expected status completed, got %s", receipt.Status)
	}

	if receipt.Signature != "sig-def456" {
		t.Errorf("Expected signature sig-def456, got %s", receipt.Signature)
	}

	if len(body) == 0 {
		t.Error("Expected non-empty response body")
	}
}

func TestGetReceipt_NotFound(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Test Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	tempDir := t.TempDir()
	clientCertPath := filepath.Join(tempDir, "client.crt")
	clientKeyPath := filepath.Join(tempDir, "client.key")
	caBundlePath := filepath.Join(tempDir, "ca.pem")

	if err := os.WriteFile(clientCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}
	clientKeyBytes, err := x509.MarshalPKCS8PrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	if err := os.WriteFile(clientKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyBytes}), 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}
	if err := os.WriteFile(caBundlePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write CA bundle: %v", err)
	}

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth: config.Auth{
			ClientCert: clientCertPath,
			ClientKey:  clientKeyPath,
			CABundle:   caBundlePath,
			Insecure:   false,
		},
	}

	testClient, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	receipt, _, err := testClient.GetReceipt(ctx, "nonexistent-tx")

	if err != nil {
		t.Fatalf("GetReceipt should not return error for 404: %v", err)
	}

	if receipt != nil {
		t.Error("Expected nil receipt for 404")
	}
}

func TestGetReceipt_ServerError(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Test Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	tempDir := t.TempDir()
	clientCertPath := filepath.Join(tempDir, "client.crt")
	clientKeyPath := filepath.Join(tempDir, "client.key")
	caBundlePath := filepath.Join(tempDir, "ca.pem")

	if err := os.WriteFile(clientCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}
	clientKeyBytes, err := x509.MarshalPKCS8PrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	if err := os.WriteFile(clientKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyBytes}), 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}
	if err := os.WriteFile(caBundlePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write CA bundle: %v", err)
	}

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth: config.Auth{
			ClientCert: clientCertPath,
			ClientKey:  clientKeyPath,
			CABundle:   caBundlePath,
			Insecure:   false,
		},
	}

	testClient, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	receipt, body, err := testClient.GetReceipt(ctx, "test-tx")

	if err == nil {
		t.Fatal("Expected error for server error")
	}

	if receipt != nil {
		t.Error("Expected nil receipt for server error")
	}

	if string(body) != "internal server error" {
		t.Errorf("Expected error body 'internal server error', got %s", string(body))
	}
}

func TestGetReceipt_InvalidJSON(t *testing.T) {
	caCert, caPriv, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Test Server")
	if err != nil {
		t.Fatalf("generateCert for server failed: %v", err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Test Client")
	if err != nil {
		t.Fatalf("generateCert for client failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {{{"))
	}))

	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			PrivateKey:  serverPriv,
		}},
	}

	server.StartTLS()
	defer server.Close()

	tempDir := t.TempDir()
	clientCertPath := filepath.Join(tempDir, "client.crt")
	clientKeyPath := filepath.Join(tempDir, "client.key")
	caBundlePath := filepath.Join(tempDir, "ca.pem")

	if err := os.WriteFile(clientCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}
	clientKeyBytes, err := x509.MarshalPKCS8PrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	if err := os.WriteFile(clientKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyBytes}), 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}
	if err := os.WriteFile(caBundlePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0600); err != nil {
		t.Fatalf("failed to write CA bundle: %v", err)
	}

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth: config.Auth{
			ClientCert: clientCertPath,
			ClientKey:  clientKeyPath,
			CABundle:   caBundlePath,
			Insecure:   false,
		},
	}

	testClient, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	receipt, body, err := testClient.GetReceipt(ctx, "test-tx")

	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if receipt != nil {
		t.Error("Expected nil receipt for invalid JSON")
	}

	if len(body) == 0 {
		t.Error("Expected non-empty response body for invalid JSON")
	}
}

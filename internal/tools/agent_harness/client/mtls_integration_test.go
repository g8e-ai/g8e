// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

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

		var env govpkg.GovernanceEnvelope
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

	env := &govpkg.GovernanceEnvelope{
		ProtocolVersion: govpkg.GovernanceProtocolVersionV1,
		OperatorId:      "test-operator",
		Timestamp:       timestamppb.Now(),
		ActionType:      "TEST_ACTION",
		TargetResource:  "localhost",
		Payload:         []byte("test payload"),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				Votes: []*commonv1.L2Vote{
					{
						SignerKeyId: "test-key",
					},
				},
			},
		},
	}

	id, err := govpkg.GenerateMessageID(env)
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

	tempDir := testutil.TempDir(t)
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

	tempDir := testutil.TempDir(t)
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

	tempDir := testutil.TempDir(t)
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

	tempDir := testutil.TempDir(t)
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

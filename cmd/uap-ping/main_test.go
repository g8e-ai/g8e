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

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/g8e-ai/g8e/pkg/uap"
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

		var env uap.UAPEnvelope
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

	env := &uap.UAPEnvelope{
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

	id, err := uap.GenerateMessageID(env)
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

	_, err = client.Get(server.URL)
	if err == nil {
		t.Error("Expected TLS handshake error with no client certificate")
	}
}

func TestUAPEnvelope_Integration(t *testing.T) {
	env := &uap.UAPEnvelope{
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

	id, err := uap.GenerateMessageID(env)
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

	var decoded uap.UAPEnvelope
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

func TestConstants_Ports(t *testing.T) {
	if constants.Ports.G8eeHttps <= 0 {
		t.Error("G8eeHttps port should be positive")
	}

	if constants.Ports.G8eeHttps > 65535 {
		t.Error("G8eeHttps port should be valid (< 65536)")
	}
}

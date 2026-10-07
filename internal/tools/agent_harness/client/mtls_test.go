// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
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

func TestGovernanceEnvelope_Integration(t *testing.T) {
	env := &govpkg.GovernanceEnvelope{
		ProtocolVersion: govpkg.GovernanceProtocolVersionV2,
		OperatorId:      "test-operator",
		Timestamp:       timestamppb.Now(),
		EventType:       string(constants.EventOperatorCommandRequested),
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
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

	if id == "" {
		t.Fatal("MessageID should not be empty")
	}

	env.Id = id

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	var decoded govpkg.GovernanceEnvelope
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

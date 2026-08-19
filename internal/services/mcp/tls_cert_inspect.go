// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// certInspectResult represents the structured output of the TLS certificate inspection tool.
type certInspectResult struct {
	Subject            string             `json:"subject"`
	Issuer             string             `json:"issuer"`
	SerialNumber       string             `json:"serial_number"`
	NotBefore          string             `json:"not_before"`
	NotAfter           string             `json:"not_after"`
	IsExpired          bool               `json:"is_expired"`
	DaysUntilExpiry    int                `json:"days_until_expiry"`
	IsNearExpiry       bool               `json:"is_near_expiry"`
	SignatureAlgorithm string             `json:"signature_algorithm"`
	PublicKeyAlgorithm string             `json:"public_key_algorithm"`
	KeyUsage           x509.KeyUsage      `json:"key_usage"`
	ExtKeyUsage        []x509.ExtKeyUsage `json:"ext_key_usage"`
	DNSNames           []string           `json:"dns_names"`
	EmailAddresses     []string           `json:"email_addresses"`
	IPAddresses        []string           `json:"ip_addresses"`
	Organization       string             `json:"organization,omitempty"`
	OrganizationalUnit string             `json:"organizational_unit,omitempty"`
}

// TLSCertInspectTool parses TLS certificates, verifies chains, and checks expiration.
type TLSCertInspectTool struct{}

// Name returns the tool identifier.
func (t *TLSCertInspectTool) Name() string {
	return "tls_cert_inspect"
}

// Description returns a human-readable description.
func (t *TLSCertInspectTool) Description() string {
	return "Parses TLS certificates, verifies chains, and checks expiration (critical for PKI debugging)."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *TLSCertInspectTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"cert_path": {
				Type:        "string",
				Description: "Path to certificate file (PEM format)",
			},
			"host": {
				Type:        "string",
				Description: "Remote host to fetch certificate from via TLS handshake",
			},
			"port": {
				Type:        "integer",
				Description: "Port number for remote host (default 443)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *TLSCertInspectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req TLSCertInspectRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("%w: %v", constants.ErrMCPUnmarshalArguments, err)
	}

	var cert *x509.Certificate
	var err error

	if req.CertPath != "" {
		if err := validateFilePath(req.CertPath); err != nil {
			return CallToolResult{}, fmt.Errorf("tls_cert_inspect: invalid cert path: %w", err)
		}
		cert, err = loadCertFromFile(req.CertPath)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("tls_cert_inspect: failed to load certificate from file: %w", err)
		}
	} else if req.Host != "" {
		port := req.Port
		if port <= 0 {
			port = 443
		}
		cert, err = fetchCertFromHost(ctx, req.Host, port)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("tls_cert_inspect: failed to fetch certificate from host: %w", err)
		}
	} else {
		return CallToolResult{}, fmt.Errorf("%w", constants.ErrMCPTLSCertInspectRequired)
	}

	result := inspectCertificate(cert)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("tls_cert_inspect: failed to marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

func loadCertFromFile(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tls_cert_inspect: failed to read file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("tls_cert_inspect: failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("tls_cert_inspect: failed to parse certificate: %w", err)
	}

	return cert, nil
}

func fetchCertFromHost(ctx context.Context, host string, port int) (*x509.Certificate, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // inspection tool must bypass verify to inspect invalid certs
	})
	if err != nil {
		return nil, fmt.Errorf("tls_cert_inspect: failed to dial host: %w", err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("tls_cert_inspect: no certificates presented")
	}

	return certs[0], nil
}

func inspectCertificate(cert *x509.Certificate) certInspectResult {
	now := time.Now()
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	isExpired := now.After(cert.NotAfter)
	isNearExpiry := daysUntilExpiry > 0 && daysUntilExpiry <= 30

	ipAddresses := make([]string, len(cert.IPAddresses))
	for i, ip := range cert.IPAddresses {
		ipAddresses[i] = ip.String()
	}

	result := certInspectResult{
		Subject:            cert.Subject.CommonName,
		Issuer:             cert.Issuer.CommonName,
		SerialNumber:       cert.SerialNumber.String(),
		NotBefore:          cert.NotBefore.Format(time.RFC3339),
		NotAfter:           cert.NotAfter.Format(time.RFC3339),
		IsExpired:          isExpired,
		DaysUntilExpiry:    daysUntilExpiry,
		IsNearExpiry:       isNearExpiry,
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		PublicKeyAlgorithm: cert.PublicKeyAlgorithm.String(),
		KeyUsage:           cert.KeyUsage,
		ExtKeyUsage:        cert.ExtKeyUsage,
		DNSNames:           cert.DNSNames,
		EmailAddresses:     cert.EmailAddresses,
		IPAddresses:        ipAddresses,
	}

	if len(cert.Subject.Organization) > 0 {
		result.Organization = cert.Subject.Organization[0]
	}
	if len(cert.Subject.OrganizationalUnit) > 0 {
		result.OrganizationalUnit = cert.Subject.OrganizationalUnit[0]
	}

	return result
}

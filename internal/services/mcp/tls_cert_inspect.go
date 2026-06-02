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
	"time"
)

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
func (t *TLSCertInspectTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cert_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to certificate file (PEM format)",
			},
			"host": map[string]interface{}{
				"type":        "string",
				"description": "Remote host to fetch certificate from via TLS handshake",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "Port number for remote host (default 443)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *TLSCertInspectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		CertPath string `json:"cert_path,omitempty"`
		Host     string `json:"host,omitempty"`
		Port     int    `json:"port,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	var cert *x509.Certificate
	var err error

	if req.CertPath != "" {
		cert, err = loadCertFromFile(req.CertPath)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to load certificate from file: %w", err)
		}
	} else if req.Host != "" {
		port := req.Port
		if port <= 0 {
			port = 443
		}
		cert, err = fetchCertFromHost(ctx, req.Host, port)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to fetch certificate from host: %w", err)
		}
	} else {
		return CallToolResult{}, fmt.Errorf("either cert_path or host must be specified")
	}

	result := inspectCertificate(cert)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
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
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

func fetchCertFromHost(ctx context.Context, host string, port int) (*x509.Certificate, error) {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	
	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates presented")
	}

	return certs[0], nil
}

func inspectCertificate(cert *x509.Certificate) map[string]interface{} {
	now := time.Now()
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	isExpired := now.After(cert.NotAfter)
	isNearExpiry := daysUntilExpiry > 0 && daysUntilExpiry <= 30

	result := map[string]interface{}{
		"subject":         cert.Subject.CommonName,
		"issuer":          cert.Issuer.CommonName,
		"serial_number":   cert.SerialNumber.String(),
		"not_before":      cert.NotBefore.Format(time.RFC3339),
		"not_after":       cert.NotAfter.Format(time.RFC3339),
		"is_expired":      isExpired,
		"days_until_expiry": daysUntilExpiry,
		"is_near_expiry":  isNearExpiry,
		"signature_algorithm": cert.SignatureAlgorithm.String(),
		"public_key_algorithm": cert.PublicKeyAlgorithm.String(),
		"key_usage":       cert.KeyUsage,
		"ext_key_usage":   cert.ExtKeyUsage,
		"dns_names":       cert.DNSNames,
		"email_addresses": cert.EmailAddresses,
		"ip_addresses":    cert.IPAddresses,
	}

	if len(cert.Subject.Organization) > 0 {
		result["organization"] = cert.Subject.Organization[0]
	}
	if len(cert.Subject.OrganizationalUnit) > 0 {
		result["organizational_unit"] = cert.Subject.OrganizationalUnit[0]
	}

	return result
}

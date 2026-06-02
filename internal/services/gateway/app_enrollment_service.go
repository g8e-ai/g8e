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
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/g8e-ai/g8e/protocol"
)

// AppEnrollmentService handles external app enrollment.
// Enrollment is identity-only by default: apps receive mTLS certificates but no L2 consensus power.
// L2 signers must be explicitly registered by an admin via POST /api/admin/app-policies/{app_id}/signer.
type AppEnrollmentService struct {
	db     *GatewayDBService
	pki    *PKIAuthority
	logger *slog.Logger
}

// NewAppEnrollmentService creates a new AppEnrollmentService.
func NewAppEnrollmentService(db *GatewayDBService, pki *PKIAuthority, logger *slog.Logger) *AppEnrollmentService {
	return &AppEnrollmentService{
		db:     db,
		pki:    pki,
		logger: logger,
	}
}

// AppEnrollRequest represents the request body for external app enrollment.
type AppEnrollRequest struct {
	CSR            string `json:"csr_pem"`
	AppName        string `json:"app_name"`
	AppType        string `json:"app_type"`        // mcp-client, a2a-gateway, custom
	OrganizationID string `json:"organization_id"` // optional, for multi-tenant
}

// AppEnrollResponse represents the response for external app enrollment.
type AppEnrollResponse struct {
	Success     bool   `json:"success"`
	AppCert     string `json:"app_cert"`
	CertChain   string `json:"cert_chain"`
	TrustBundle string `json:"trust_bundle"`
	AppID       string `json:"app_id"`
	ExpiresAt   string `json:"expires_at"`
	Error       string `json:"error,omitempty"`
}

// EnrollApp handles external app enrollment.
// Returns an mTLS identity certificate only (identity-only enrollment).
// L2 consensus power requires explicit admin registration via POST /api/admin/app-policies/{app_id}/signer.
func (s *AppEnrollmentService) EnrollApp(req AppEnrollRequest) (*AppEnrollResponse, error) {
	// Validate request
	if req.CSR == "" {
		return &AppEnrollResponse{Success: false, Error: "csr_pem is required"}, nil
	}
	if req.AppName == "" {
		return &AppEnrollResponse{Success: false, Error: "app_name is required"}, nil
	}
	if req.AppType == "" {
		return &AppEnrollResponse{Success: false, Error: "app_type is required"}, nil
	}

	// Validate app type
	validAppTypes := map[string]bool{
		"mcp-client":  true,
		"a2a-gateway": true,
		"custom":      true,
	}
	if !validAppTypes[req.AppType] {
		return &AppEnrollResponse{Success: false, Error: "invalid app_type (must be mcp-client, a2a-gateway, or custom)"}, nil
	}

	// Sanitize app name (alphanumeric, hyphens, underscores only)
	sanitizedName := strings.Trim(req.AppName, " ")
	if !isValidAppName(sanitizedName) {
		return &AppEnrollResponse{Success: false, Error: "app_name must contain only alphanumeric characters, hyphens, and underscores"}, nil
	}

	appID := s.generateAppID(sanitizedName)

	// Validate CSR format
	block, _ := pem.Decode([]byte(req.CSR))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return &AppEnrollResponse{Success: false, Error: "invalid CSR PEM format"}, nil
	}

	// Parse CSR to extract public key for L2 signer generation
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: "failed to parse CSR"}, nil
	}

	if err := csr.CheckSignature(); err != nil {
		return &AppEnrollResponse{Success: false, Error: "CSR signature check failed"}, nil
	}

	// Sign the CSR with the Operator intermediate CA
	// Use appID as the operatorID parameter for AppSPIFFEID generation
	certPEM, chainPEM, err := s.pki.SignCSR(req.CSR, "app", req.OrganizationID, appID, "", "", "")
	if err != nil {
		s.logger.Error("Failed to sign app CSR", "app_id", appID, "error", err)
		return &AppEnrollResponse{Success: false, Error: "failed to sign certificate"}, nil
	}

	// Fetch trust bundle
	trustBundle, err := s.pki.GatewayTrustBundle()
	if err != nil {
		s.logger.Error("Failed to fetch trust bundle", "error", err)
		// Non-fatal: continue without trust bundle
		trustBundle = []byte{}
	}

	// Calculate expiry time from certificate
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return &AppEnrollResponse{Success: false, Error: "failed to parse issued certificate"}, nil
	}
	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: "failed to parse issued certificate"}, nil
	}

	s.logger.Info("[APP_ENROLLMENT] External app enrolled (identity only)",
		"app_id", appID,
		"app_name", sanitizedName,
		"app_type", req.AppType)

	return &AppEnrollResponse{
		Success:     true,
		AppCert:     certPEM,
		CertChain:   chainPEM,
		TrustBundle: string(trustBundle),
		AppID:       appID,
		ExpiresAt:   parsedCert.NotAfter.UTC().Format(time.RFC3339),
		// L2SignerID is deliberately omitted as enrollment is identity-only by default.
		// App policies and signers must be explicitly configured by an admin.
	}, nil
}

// generateAppID generates a SPIFFE ID for the app.
func (s *AppEnrollmentService) generateAppID(appName string) string {
	wid := protocol.NewWorkloadIdentity()
	appURL, _ := wid.AppSPIFFEURL(appName)
	return appURL.String()
}

// isValidAppName validates that the app name contains only allowed characters.
func isValidAppName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

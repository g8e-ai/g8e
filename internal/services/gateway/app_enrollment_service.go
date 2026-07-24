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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// AppEnrollmentService handles external app enrollment.
// Enrollment is identity-only by default: apps receive mTLS certificates but no L2 consensus power.
// L2 signers must be explicitly registered by an admin via POST /api/admin/app-policies/{app_id}/signer.
type AppEnrollmentService struct {
	db     *DocumentStoreService
	pki    *PKIAuthority
	logger *slog.Logger
}

// NewAppEnrollmentService creates a new AppEnrollmentService.
func NewAppEnrollmentService(docStore *DocumentStoreService, pki *PKIAuthority, logger *slog.Logger) *AppEnrollmentService {
	return &AppEnrollmentService{
		db:     docStore,
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
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollCSRRequired.Error()}, nil
	}
	if req.AppName == "" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollAppNameRequired.Error()}, nil
	}
	if req.AppType == "" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollAppTypeRequired.Error()}, nil
	}

	// Validate app type
	validAppTypes := map[string]bool{
		"mcp-client":      true,
		"a2a-gateway":     true,
		"custom":          true,
		"tribunal-member": true,
	}
	if !validAppTypes[req.AppType] {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollInvalidAppType.Error()}, nil
	}

	// Sanitize app name (alphanumeric, hyphens, underscores only)
	sanitizedName := strings.Trim(req.AppName, " ")
	if !isValidAppName(sanitizedName) {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollInvalidAppName.Error()}, nil
	}

	// Validate CSR format
	block, _ := pem.Decode([]byte(req.CSR))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollInvalidCSRPEM.Error()}, nil
	}

	// Parse CSR to extract public key for L2 signer generation
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollParseCSR.Error()}, nil
	}

	if err := csr.CheckSignature(); err != nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollCSRSignatureCheck.Error()}, nil
	}

	// Sign the CSR with the Operator intermediate CA
	// Pass the app name (not the full SPIFFE ID) - SignCSR will generate the SPIFFE ID
	certPEM, chainPEM, err := s.pki.SignCSR(req.CSR, "app", req.OrganizationID, sanitizedName, "", "", "")
	if err != nil {
		s.logger.Error("Failed to sign app CSR", "app_name", sanitizedName, "error", err)
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollSignCertificate.Error()}, nil
	}

	// Extract the actual appID from the signed certificate's URI SAN
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollParseIssuedCert.Error()}, nil
	}
	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: fmt.Sprintf("%s: %v", constants.ErrAppEnrollParseIssuedCert.Error(), err)}, nil
	}
	if len(parsedCert.URIs) == 0 {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollNoURISAN.Error()}, nil
	}
	appID := parsedCert.URIs[0].String()

	if err := s.persistAppPolicy(appID); err != nil {
		return &AppEnrollResponse{Success: false, Error: err.Error()}, nil
	}

	// Fetch trust bundle
	trustBundle, err := s.pki.GatewayTrustBundle()
	if err != nil {
		s.logger.Error("Failed to fetch trust bundle", "error", err)
		// Non-fatal: continue without trust bundle
		trustBundle = []byte{}
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
	}, nil
}

// EnrollDelegatedApp handles delegated app enrollment with dual SANs (app + requestor).
// The certificate is short-lived (1 hour) and binds both the app identity and the requesting user.
// Like EnrollApp, it persists a default AppPolicy so handleAppAuth accepts the app certificate.
func (s *AppEnrollmentService) EnrollDelegatedApp(req AppEnrollRequest, userID string) (*AppEnrollResponse, error) {
	if req.CSR == "" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollCSRRequired.Error()}, nil
	}
	if req.AppName == "" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollAppNameRequired.Error()}, nil
	}

	sanitizedName := strings.Trim(req.AppName, " ")
	if !isValidAppName(sanitizedName) {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollInvalidAppName.Error()}, nil
	}

	block, _ := pem.Decode([]byte(req.CSR))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollInvalidCSRPEM.Error()}, nil
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollParseCSR.Error()}, nil
	}

	if err := csr.CheckSignature(); err != nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollCSRSignatureCheck.Error()}, nil
	}

	certPEM, chainPEM, err := s.pki.SignDelegatedCSR(req.CSR, sanitizedName, userID)
	if err != nil {
		s.logger.Error("Failed to sign delegated CSR", "app_name", sanitizedName, "user_id", userID, "error", err)
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollSignCertificate.Error()}, nil
	}

	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollParseIssuedCert.Error()}, nil
	}
	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return &AppEnrollResponse{Success: false, Error: fmt.Sprintf("%s: %v", constants.ErrAppEnrollParseIssuedCert.Error(), err)}, nil
	}
	if len(parsedCert.URIs) == 0 {
		return &AppEnrollResponse{Success: false, Error: constants.ErrAppEnrollNoURISAN.Error()}, nil
	}
	appID := parsedCert.URIs[0].String()

	if err := s.persistAppPolicy(appID); err != nil {
		return &AppEnrollResponse{Success: false, Error: err.Error()}, nil
	}

	trustBundle, err := s.pki.GatewayTrustBundle()
	if err != nil {
		s.logger.Error("Failed to fetch trust bundle", "error", err)
		trustBundle = []byte{}
	}

	s.logger.Info("[DELEGATED_CREDENTIAL] Minted delegated credential",
		"app_id", appID,
		"app_name", sanitizedName,
		"user_id", userID,
		"expires_at", parsedCert.NotAfter.UTC().Format(time.RFC3339))

	return &AppEnrollResponse{
		Success:     true,
		AppCert:     certPEM,
		CertChain:   chainPEM,
		TrustBundle: string(trustBundle),
		AppID:       appID,
		ExpiresAt:   parsedCert.NotAfter.UTC().Format(time.RFC3339),
	}, nil
}

// persistAppPolicy writes a default AppPolicy for the given appID to the doc store.
// This is required for handleAppAuth to accept the app certificate.
func (s *AppEnrollmentService) persistAppPolicy(appID string) error {
	policy := models.AppPolicy{
		AppID:              appID,
		AllowedCollections: nil,
		RateLimitRPS:       0,
		MaxPayloadBytes:    0,
		RequireL3Approval:  false,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	data, err := json.Marshal(policy)
	if err != nil {
		s.logger.Error("Failed to marshal app policy", "app_id", appID, "error", err)
		return constants.ErrAppEnrollMarshalAppPolicy
	}
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, data); err != nil {
		s.logger.Error("Failed to persist app policy", "app_id", appID, "error", err)
		return constants.ErrAppEnrollPersistAppPolicy
	}
	return nil
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

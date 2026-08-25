// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
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

// EnrollDelegatedApp handles delegated app enrollment with dual SANs (app + requestor).
// The certificate is short-lived (1 hour) and binds both the app identity and the requesting user.
// It persists a default AppPolicy so handleAppAuth accepts the app certificate.
// Reserved platform component names (g8ed, g8ee, g8eo) are rejected; those identities
// require owner-approved enrollment via the platform enrollment protocol.
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
	if isReservedPlatformName(sanitizedName) {
		return &AppEnrollResponse{Success: false, Error: constants.ErrPlatformEnrollmentReservedIdentity.Error()}, nil
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

// isReservedPlatformName reports whether the given app name collides with a
// canonical platform component identity. Those identities are issued only
// through the owner-approved platform enrollment protocol.
func isReservedPlatformName(name string) bool {
	switch name {
	case models.PlatformDashboardName, models.PlatformEnsembleName, models.PlatformOperatorName:
		return true
	default:
		return false
	}
}

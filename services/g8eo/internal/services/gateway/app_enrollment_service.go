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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/protocol"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
)

// AppEnrollmentService handles external app enrollment with automatic L2 signer provisioning.
// This enables third-party applications to authenticate via mTLS and participate in Consensus Mode.
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
	L2SignerID  string `json:"l2_signer_id"` // The key ID for L2 signatures
	Error       string `json:"error,omitempty"`
}

// EnrollApp handles external app enrollment with automatic L2 signer provisioning.
// This implements Option A from the plan: auto-provision L2 signer on enrollment.
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

	// Check if app name already exists (uniqueness validation)
	appID := s.generateAppID(sanitizedName)
	existingSigner, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
	if err != nil {
		s.logger.Error("Failed to check for existing app signer", "app_id", appID, "error", err)
		return &AppEnrollResponse{Success: false, Error: "failed to validate app name uniqueness"}, nil
	}
	if existingSigner != nil {
		return &AppEnrollResponse{Success: false, Error: "app_name already registered"}, nil
	}

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

	// Generate Ed25519 key pair for L2 signing (Option A: auto-provision)
	// Note: ed25519.GenerateKey returns (PublicKey, PrivateKey, error)
	l2Pub, l2Priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		s.logger.Error("Failed to generate L2 signing key", "app_name", sanitizedName, "error", err)
		return &AppEnrollResponse{Success: false, Error: "failed to generate L2 signing key"}, nil
	}

	// Store L2 private key in SecretManager
	// The app ID is used as the service name for key storage
	if s.pki.secretManager == nil {
		return &AppEnrollResponse{Success: false, Error: "secret manager not available"}, nil
	}
	l2PrivDER := l2Priv.Seed()
	if err := s.pki.secretManager.StoreServicePrivateKey(appID, l2PrivDER); err != nil {
		s.logger.Error("Failed to store L2 private key", "app_id", appID, "error", err)
		return &AppEnrollResponse{Success: false, Error: "failed to store L2 signing key"}, nil
	}

	// Add public key to TrustedSigner collection
	trustedSigner := models.TrustedSigner{
		ID:        appID,
		PublicKey: hex.EncodeToString(l2Pub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signerBytes, err := json.Marshal(trustedSigner)
	if err != nil {
		// Rollback: delete private key
		_ = s.pki.secretManager.DeleteServicePrivateKey(appID)
		return &AppEnrollResponse{Success: false, Error: "failed to marshal signer data"}, nil
	}
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID, signerBytes); err != nil {
		s.logger.Error("Failed to store L2 signer", "app_id", appID, "error", err)
		// Rollback: delete private key
		_ = s.pki.secretManager.DeleteServicePrivateKey(appID)
		return &AppEnrollResponse{Success: false, Error: "failed to register L2 signer"}, nil
	}

	// Sign the CSR with the operator intermediate CA
	// Use appID as the operatorID parameter for AppSPIFFEID generation
	certPEM, chainPEM, err := s.pki.SignCSR(req.CSR, "app", req.OrganizationID, appID, "", "")
	if err != nil {
		s.logger.Error("Failed to sign app CSR", "app_id", appID, "error", err)
		// Rollback: delete signer and private key
		_, _ = s.db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		_ = s.pki.secretManager.DeleteServicePrivateKey(appID)
		return &AppEnrollResponse{Success: false, Error: "failed to sign certificate"}, nil
	}

	// Fetch trust bundle
	trustBundle, err := s.pki.HubTrustBundle()
	if err != nil {
		s.logger.Error("Failed to fetch trust bundle", "error", err)
		// Non-fatal: continue without trust bundle
		trustBundle = []byte{}
	}

	// Calculate expiry time from certificate
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		// Rollback on error
		_, _ = s.db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		_ = s.pki.secretManager.DeleteServicePrivateKey(appID)
		return &AppEnrollResponse{Success: false, Error: "failed to parse issued certificate"}, nil
	}
	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		// Rollback on error
		_, _ = s.db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		_ = s.pki.secretManager.DeleteServicePrivateKey(appID)
		return &AppEnrollResponse{Success: false, Error: "failed to parse issued certificate"}, nil
	}

	s.logger.Info("[APP_ENROLLMENT] External app enrolled",
		"app_id", appID,
		"app_name", sanitizedName,
		"app_type", req.AppType,
		"l2_signer_id", appID)

	return &AppEnrollResponse{
		Success:     true,
		AppCert:     certPEM,
		CertChain:   chainPEM,
		TrustBundle: string(trustBundle),
		AppID:       appID,
		ExpiresAt:   parsedCert.NotAfter.UTC().Format(time.RFC3339),
		L2SignerID:  appID,
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
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

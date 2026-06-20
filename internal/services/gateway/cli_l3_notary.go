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
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/protocol"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// CLIL3Notary provides L3 verification for CLI sessions using mTLS certificates.
// CLI sessions authenticate via mTLS certificates with SPIFFE URI SANs, and this
// notary leverages that transport-layer authentication as the L3 proof.
type CLIL3Notary struct {
	db            *CanonicalDBService
	pki           *PKIAuthority
	logger        *slog.Logger
	userSvc       *UserService
	cliSessionSvc *CLISessionService
}

// NewCLIL3Notary creates a new CLI L3 notary.
func NewCLIL3Notary(db *CanonicalDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, cliSessionSvc *CLISessionService) *CLIL3Notary {
	return &CLIL3Notary{
		db:            db,
		pki:           pki,
		logger:        logger,
		userSvc:       userSvc,
		cliSessionSvc: cliSessionSvc,
	}
}

// VerifyL3Proof verifies an L3 proof for CLI sessions using mTLS certificate validation.
// For CLI sessions, the L3 proof is the fingerprint of the mTLS certificate used for
// authentication. The notary checks that:
// 1. The certificate fingerprint matches the provided proof
// 2. The certificate is valid (not revoked, not expired)
// 3. The certificate's SPIFFE URI SAN matches the expected CLI session
// 4. The user associated with the CLI session is active
// 5. The CLI session ID in the envelope matches the session with the certificate fingerprint
//
// This method is called during transaction verification. The actual mTLS certificate
// is passed via request context in production, but for envelope verification we need
// to reconstruct the validation from the stored fingerprint.
func (v *CLIL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if userID == "" {
		return false, constants.ErrUserIDRequired
	}
	if transactionHash == "" {
		return false, constants.ErrCLIL3TransactionHashRequired
	}
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}
	if proof.MtlsCertFingerprint == "" {
		return false, constants.ErrCLIL3CertFingerprintRequired
	}

	// Verify the fingerprint is a valid SHA256 hex string
	if _, err := hex.DecodeString(proof.MtlsCertFingerprint); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3InvalidFingerprintFormat, err)
	}

	// Check if the user is active
	if v.userSvc != nil {
		user, err := v.userSvc.GetByID(userID)
		if err != nil {
			v.logger.Error("Failed to load user for CLI L3 verification", "user_id", userID, "error", err)
			return false, fmt.Errorf("%w: %w", constants.ErrUserNotFound, err)
		}
		if user == nil {
			return false, constants.ErrUserNotFound
		}
		if !user.IsActive() {
			v.logger.Warn("CLI L3 verification failed: user is not active", "user_id", userID)
			return false, constants.ErrCLIL3UserInactive
		}
	}

	// Load the specific CLI session by ID to enforce session-specific authorization
	if v.db == nil {
		return false, constants.ErrGatewayDatabaseServiceNotConfigured
	}

	if cliSessionID == "" {
		return false, constants.ErrCLIL3SessionIDRequired
	}

	doc, err := v.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	if err != nil {
		v.logger.Error("Failed to load CLI session for L3 verification", "cli_session_id", cliSessionID, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3SessionLoadFailed, err)
	}
	if doc == nil {
		v.logger.Warn("CLI L3 verification failed: CLI session not found", "cli_session_id", cliSessionID)
		return false, constants.ErrCLIL3SessionNotFound
	}

	sessionBytes, err := json.Marshal(doc.ForWire())
	if err != nil {
		v.logger.Warn("Failed to marshal CLI session", "cli_session_id", doc.ID, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3SessionMarshalFailed, err)
	}

	var session models.CLISession
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		v.logger.Warn("Failed to unmarshal CLI session", "cli_session_id", doc.ID, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3SessionUnmarshalFailed, err)
	}

	// Verify the session belongs to the user
	if session.UserID != userID {
		v.logger.Warn("CLI L3 verification failed: session user mismatch", "cli_session_id", cliSessionID, "session_user_id", session.UserID, "envelope_user_id", userID)
		return false, constants.ErrCLIL3SessionUserMismatch
	}

	// Verify the certificate fingerprint matches the session's stored fingerprint
	if session.CertFingerprint != proof.MtlsCertFingerprint {
		expectedPrefix := session.CertFingerprint
		providedPrefix := proof.MtlsCertFingerprint
		if len(session.CertFingerprint) > 16 {
			expectedPrefix = session.CertFingerprint[:16]
		}
		if len(proof.MtlsCertFingerprint) > 16 {
			providedPrefix = proof.MtlsCertFingerprint[:16]
		}
		v.logger.Warn("CLI L3 verification failed: certificate fingerprint mismatch", "cli_session_id", cliSessionID, "expected", expectedPrefix, "provided", providedPrefix)
		return false, constants.ErrCLIL3FingerprintMismatch
	}

	// Verify the CLI session is active
	if !session.IsActive {
		v.logger.Warn("CLI L3 verification failed: CLI session is not active", "cli_session_id", cliSessionID)
		return false, constants.ErrCLIL3SessionInactive
	}

	// Verify the CLI session is not expired
	if time.Now().UTC().After(session.ExpiresAt) {
		v.logger.Warn("CLI L3 verification failed: CLI session expired", "user_id", userID, "cli_session_id", cliSessionID)
		return false, constants.ErrCLIL3SessionExpired
	}

	// Verify the certificate is not revoked via PKI authority
	if v.pki != nil && session.CertSerial != "" {
		revoked, err := v.pki.IsRevoked(session.CertSerial)
		if err != nil {
			v.logger.Error("Failed to check certificate revocation status", "user_id", userID, "cli_session_id", cliSessionID, "cert_serial", session.CertSerial, "error", err)
			return false, fmt.Errorf("%w: %w", constants.ErrCLIL3CertRevocationCheckFailed, err)
		}
		if revoked {
			v.logger.Warn("CLI L3 verification failed: certificate is revoked", "user_id", userID, "cli_session_id", cliSessionID, "cert_serial", session.CertSerial)
			return false, nil
		}
	}

	hashPrefix := transactionHash
	if len(transactionHash) > 8 {
		hashPrefix = transactionHash[:8]
	}
	v.logger.Debug("CLI L3 verification passed", "user_id", userID, "transaction_hash", hashPrefix, "cli_session_id", cliSessionID)
	return true, nil
}

// CertFingerprint computes the SHA-256 fingerprint of a certificate.
func CertFingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// VerifyCLICertificate verifies that a CLI certificate is valid for the given CLI session.
// This is used during request authentication to validate the mTLS certificate.
func (v *CLIL3Notary) VerifyCLICertificate(cert *x509.Certificate, cliSessionID, userID string) error {
	if cert == nil {
		return constants.ErrCLIL3CertNil
	}

	// Check certificate expiry
	if time.Now().After(cert.NotAfter) {
		return constants.ErrCLIL3CertExpired
	}
	if time.Now().Before(cert.NotBefore) {
		return constants.ErrCLIL3CertNotYetValid
	}

	// Verify certificate validity if PKI authority is available
	if v.pki != nil {
		if err := v.pki.VerifyCertificate(cert); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCLIL3CertVerificationFailed, err)
		}
	}

	// Verify SPIFFE URI SAN matches the expected CLI session
	wid := protocol.NewWorkloadIdentity()
	match := false
	for _, uri := range cert.URIs {
		if wid.MatchesCLI(uri.String(), userID, cliSessionID) {
			match = true
			break
		}
	}
	if !match {
		return constants.ErrCLIL3SPIFFESANMismatch
	}

	return nil
}

// ExtractCLISessionFromCert extracts the CLI session ID from a certificate's SPIFFE URI SAN.
func ExtractCLISessionFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", constants.ErrCLIL3CertNil
	}

	wid := protocol.NewWorkloadIdentity()
	for _, uri := range cert.URIs {
		if sessionID, ok := wid.ExtractCLISessionID(uri.String()); ok {
			return sessionID, nil
		}
	}

	return "", constants.ErrCLIL3NoSessionIDInCert
}

// ExtractUserIDFromCert extracts the user ID from a certificate's SPIFFE URI SAN.
func ExtractUserIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", constants.ErrCLIL3CertNil
	}

	wid := protocol.NewWorkloadIdentity()
	for _, uri := range cert.URIs {
		if userID, ok := wid.ExtractUserID(uri.String()); ok {
			return userID, nil
		}
	}

	return "", constants.ErrCLIL3NoUserIDInCert
}

// VerifyCertificate verifies a single certificate using the PKI authority.
func (v *CLIL3Notary) VerifyCertificate(cert *x509.Certificate) error {
	if v.pki == nil {
		return constants.ErrCLIL3PKINotConfigured
	}
	return v.pki.VerifyCertificate(cert)
}

// CreateL3ProofFromCert creates an L3 proof from an mTLS certificate.
// This is used by CLI clients to attach the certificate fingerprint to envelopes.
func CreateL3ProofFromCert(cert *x509.Certificate) *commonv1.L3Proof {
	if cert == nil {
		return nil
	}
	return &commonv1.L3Proof{
		MtlsCertFingerprint: CertFingerprint(cert),
	}
}

// CreateL3ProofFromTLSState creates an L3 proof from a TLS connection state.
// This is used by the server to create L3 proofs from incoming mTLS connections.
func CreateL3ProofFromTLSState(tlsState *tls.ConnectionState) *commonv1.L3Proof {
	if tlsState == nil || len(tlsState.PeerCertificates) == 0 {
		return nil
	}
	return CreateL3ProofFromCert(tlsState.PeerCertificates[0])
}

// ParseSPIFFEURIFromCert parses the SPIFFE URI from a certificate.
func ParseSPIFFEURIFromCert(cert *x509.Certificate) (*url.URL, error) {
	if cert == nil {
		return nil, constants.ErrCLIL3CertNil
	}

	for _, uri := range cert.URIs {
		if uri.Scheme == "spiffe" {
			return uri, nil
		}
	}

	return nil, constants.ErrCLIL3NoSPIFFEURI
}

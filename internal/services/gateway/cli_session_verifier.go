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
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
)

// cliSessionVerifier implements governance.CLISessionVerifier using gateway-specific
// services (UserService, CLISessionService, PKIAuthority, DocumentStoreService).
type cliSessionVerifier struct {
	db            *DocumentStoreService
	pki           *PKIAuthority
	logger        *slog.Logger
	userSvc       *UserService
	cliSessionSvc *CLISessionService
}

// NewCLISessionVerifier creates a governance.CLISessionVerifier that validates
// user active status, CLI session validity, and certificate revocation.
func NewCLISessionVerifier(docStore *DocumentStoreService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, cliSessionSvc *CLISessionService) governance.CLISessionVerifier {
	return &cliSessionVerifier{
		db:            docStore,
		pki:           pki,
		logger:        logger,
		userSvc:       userSvc,
		cliSessionSvc: cliSessionSvc,
	}
}

// VerifyCLISession performs CLI session-specific verification: user active status,
// session ownership/fingerprint/active/expiry, and certificate revocation.
// Returns governance.ErrCLISessionDenied for denials (revoked certs) and other
// errors for system failures.
func (v *cliSessionVerifier) VerifyCLISession(userID, cliSessionID, certFingerprint string) error {
	if err := v.verifyUserActive(userID); err != nil {
		return err
	}

	session, err := v.verifyCLISession(userID, cliSessionID, certFingerprint)
	if err != nil {
		return err
	}

	if err := v.verifyCertNotRevoked(userID, cliSessionID, session); err != nil {
		return err
	}

	return nil
}

// verifyUserActive loads the user and verifies they are active.
func (v *cliSessionVerifier) verifyUserActive(userID string) error {
	if v.userSvc == nil {
		return constants.ErrCLIL3UserServiceNotConfigured
	}
	user, err := v.userSvc.GetByID(userID)
	if err != nil {
		v.logger.Error("Failed to load user for CLI L3 verification", "user_id", userID, "error", err)
		return fmt.Errorf("cli session verifier: load user: %w", err)
	}
	if user == nil {
		return constants.ErrUserNotFound
	}
	if !user.IsActive() {
		v.logger.Warn("CLI L3 verification failed: user is not active", "user_id", userID)
		return constants.ErrCLIL3UserInactive
	}
	return nil
}

// verifyCLISession loads the CLI session by ID and verifies session ownership,
// certificate fingerprint match, active status, and expiry.
func (v *cliSessionVerifier) verifyCLISession(userID, cliSessionID, certFingerprint string) (*models.CLISession, error) {
	if v.db == nil {
		return nil, constants.ErrGatewayDatabaseServiceNotConfigured
	}
	if cliSessionID == "" {
		return nil, constants.ErrCLIL3SessionIDRequired
	}

	doc, err := v.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	if err != nil {
		v.logger.Error("Failed to load CLI session for L3 verification", "cli_session_id", cliSessionID, "error", err)
		return nil, fmt.Errorf("cli session verifier: load cli session: %w", err)
	}
	if doc == nil {
		v.logger.Warn("CLI L3 verification failed: CLI session not found", "cli_session_id", cliSessionID)
		return nil, constants.ErrCLIL3SessionNotFound
	}

	sessionBytes, err := json.Marshal(doc.ForWire())
	if err != nil {
		v.logger.Warn("Failed to marshal CLI session", "cli_session_id", doc.ID, "error", err)
		return nil, fmt.Errorf("cli session verifier: marshal cli session: %w", err)
	}

	var session models.CLISession
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		v.logger.Warn("Failed to unmarshal CLI session", "cli_session_id", doc.ID, "error", err)
		return nil, fmt.Errorf("cli session verifier: unmarshal cli session: %w", err)
	}

	if session.UserID != userID {
		v.logger.Warn("CLI L3 verification failed: session user mismatch", "cli_session_id", cliSessionID, "session_user_id", session.UserID, "envelope_user_id", userID)
		return nil, constants.ErrCLIL3SessionUserMismatch
	}

	if subtle.ConstantTimeCompare([]byte(session.CertFingerprint), []byte(certFingerprint)) != 1 {
		expectedPrefix := session.CertFingerprint
		providedPrefix := certFingerprint
		if len(session.CertFingerprint) > 16 {
			expectedPrefix = session.CertFingerprint[:16]
		}
		if len(certFingerprint) > 16 {
			providedPrefix = certFingerprint[:16]
		}
		v.logger.Warn("CLI L3 verification failed: certificate fingerprint mismatch", "cli_session_id", cliSessionID, "expected", expectedPrefix, "provided", providedPrefix)
		return nil, constants.ErrCLIL3FingerprintMismatch
	}

	if !session.IsActive {
		v.logger.Warn("CLI L3 verification failed: CLI session is not active", "cli_session_id", cliSessionID)
		return nil, constants.ErrCLIL3SessionInactive
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		v.logger.Warn("CLI L3 verification failed: CLI session expired", "user_id", userID, "cli_session_id", cliSessionID)
		return nil, constants.ErrCLIL3SessionExpired
	}

	return &session, nil
}

// verifyCertNotRevoked checks that the session's certificate has not been revoked
// via the PKI authority. Returns governance.ErrCLISessionDenied if the certificate is revoked.
func (v *cliSessionVerifier) verifyCertNotRevoked(userID, cliSessionID string, session *models.CLISession) error {
	if v.pki == nil {
		return constants.ErrCLIL3PKINotConfigured
	}
	if session.CertSerial != "" {
		revoked, err := v.pki.IsRevoked(session.CertSerial)
		if err != nil {
			v.logger.Error("Failed to check certificate revocation status", "user_id", userID, "cli_session_id", cliSessionID, "cert_serial", session.CertSerial, "error", err)
			return fmt.Errorf("cli session verifier: check cert revocation: %w", err)
		}
		if revoked {
			v.logger.Warn("CLI L3 verification failed: certificate is revoked", "user_id", userID, "cli_session_id", cliSessionID, "cert_serial", session.CertSerial)
			return governance.ErrCLISessionDenied
		}
	}
	return nil
}

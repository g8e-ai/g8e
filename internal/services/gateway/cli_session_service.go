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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/uuid"
)

// CLISessionService handles CLI session persistence and management.
// CLI sessions are strictly disjoint from operator sessions - they represent
// the routing namespace for BYO/CLI clients to receive SessionEvents (SSE)
// and embed in outbound request bodies.
type CLISessionService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewCLISessionService creates a new CLISessionService instance.
func NewCLISessionService(docStore *DocumentStoreService, logger *slog.Logger) *CLISessionService {
	return &CLISessionService{
		db:     docStore,
		logger: logger,
	}
}

// PersistCLISession creates and persists a CLI session document.
// The operatorSessionID binds this CLI session to an operator session for authorization.
func (s *CLISessionService) PersistCLISession(cliSessionID, operatorSessionID, userID, systemFingerprint, certFingerprint, certSerial, loginMethod string) error {
	cliExpiry := time.Now().UTC().Add(1 * time.Hour)
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		SystemFingerprint: systemFingerprint,
		CertFingerprint:   certFingerprint,
		CertSerial:        certSerial,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         cliExpiry,
		AbsoluteExpiresAt: cliExpiry,
		IdleExpiresAt:     cliExpiry,
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       loginMethod,
	}
	cliSessionBytes, err := json.Marshal(cliSession)
	if err != nil {
		return fmt.Errorf("failed to marshal CLI session: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes); err != nil {
		s.logger.Error("Failed to persist CLI session", string(constants.ConnectionStateError), err)
		return fmt.Errorf("failed to persist CLI session: %w", err)
	}

	return nil
}

// CLISessionFields carries the identity-binding fields for a new CLI session
// created by ReplaceCLISession. It is a typed subset of models.CLISession so
// callers (rotation, recovery completion) cannot accidentally swap the user
// or operator binding when replacing a session.
type CLISessionFields struct {
	OperatorSessionID string
	UserID            string
	SystemFingerprint string
	CertFingerprint   string
	CertSerial        string
	LoginMethod       string
}

// ErrCLISessionNotFound is returned by ReplaceCLISession/DeactivateCLISession
// when the referenced old session does not exist.
var ErrCLISessionNotFound = errors.New("CLI session not found")

// ErrCLISessionAlreadyDeactivated is returned by DeactivateCLISession (and
// ReplaceCLISession) when the target session is already inactive. This is a
// distinct signal from "not found" so callers can distinguish a concurrent
// replacement from a missing document.
var ErrCLISessionAlreadyDeactivated = errors.New("CLI session already deactivated")

// DeactivateCLISession atomically marks a CLI session inactive by setting
// is_active=false via DocConditionalUpdate. Only an active session can be
// deactivated; an already-deactivated session returns
// ErrCLISessionAlreadyDeactivated and a missing session returns
// ErrCLISessionNotFound. The caller is responsible for any PKI revocation
// side-effect; this method only mutates CLI-session state.
func (s *CLISessionService) DeactivateCLISession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("deactivate CLI session: %w", constants.ErrCLISessionInvalid)
	}

	// Read first to distinguish not-found from already-deactivated.
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), sessionID)
	if err != nil {
		return fmt.Errorf("deactivate CLI session: load: %w", err)
	}
	if doc == nil {
		return ErrCLISessionNotFound
	}
	existing, err := decodeCLISession(doc)
	if err != nil {
		return fmt.Errorf("deactivate CLI session: decode: %w", err)
	}
	if !existing.IsActive {
		return ErrCLISessionAlreadyDeactivated
	}

	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionCLISessions),
		sessionID,
		map[string]interface{}{"is_active": false},
		"is_active", true,
	)
	if err != nil {
		return fmt.Errorf("deactivate CLI session: conditional update: %w", err)
	}
	if !applied {
		// Another caller deactivated or replaced the session between our read
		// and the conditional update. Re-read to give a precise error.
		current, err := s.loadCLISession(sessionID)
		if err != nil {
			return err
		}
		if !current.IsActive {
			return ErrCLISessionAlreadyDeactivated
		}
		// Should not happen — the conditional update matched on is_active=true
		// and the only competing writer also flips it to false.
		return fmt.Errorf("deactivate CLI session: %w", constants.ErrCLISessionInvalid)
	}
	s.logger.Info("CLI session deactivated", "cli_session_id_prefix", safeTruncateID(sessionID, 8))
	return nil
}

// ReplaceCLISession transactionally replaces an active CLI session with a new
// one bound to the supplied identity fields. It is the single session-
// replacement path used by both CLI rotation (5d) and recovery completion
// (5c) so PKI revocation state and CLI-session state cannot silently diverge.
//
// Order of operations (documented for partial-failure recovery):
//  1. Read the old session; reject if missing or already deactivated.
//  2. Persist the new CLI session document (DocSet).
//  3. Atomically deactivate the old session via DocConditionalUpdate on
//     is_active=true. If this fails because a concurrent caller already
//     deactivated it, the freshly-written new session is left in place but
//     the caller receives ErrCLISessionAlreadyDeactivated so it can decide
//     whether to revoke the new cert it just signed.
//
// PKI revocation of the OLD certificate is the caller's responsibility and
// MUST happen after this method returns nil. If the caller revokes first and
// this method then fails, the old session is still active and the user is
// locked out; if this method succeeds first and revocation then fails, the
// old session is inactive but the old cert may still pass a CRL-skipping
// verifier — the typed error lets the caller retry revocation idempotently.
// The new certificate is signed by the caller BEFORE this method is invoked
// so a signing failure never leaves a stale deactivated session.
func (s *CLISessionService) ReplaceCLISession(oldSessionID string, newCertFingerprint, newCertSerial string, newFields CLISessionFields) (newSession *models.CLISession, err error) {
	if oldSessionID == "" {
		return nil, fmt.Errorf("replace CLI session: %w", constants.ErrCLISessionInvalid)
	}
	if newFields.UserID == "" || newFields.OperatorSessionID == "" {
		return nil, fmt.Errorf("replace CLI session: missing user or operator session binding")
	}

	// 1. Verify the old session exists and is still active.
	oldDoc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), oldSessionID)
	if err != nil {
		return nil, fmt.Errorf("replace CLI session: load old: %w", err)
	}
	if oldDoc == nil {
		return nil, ErrCLISessionNotFound
	}
	oldSession, err := decodeCLISession(oldDoc)
	if err != nil {
		return nil, fmt.Errorf("replace CLI session: decode old: %w", err)
	}
	if !oldSession.IsActive {
		return nil, ErrCLISessionAlreadyDeactivated
	}

	// 2. Persist the new session first so a deactivation failure does not
	//    leave the user without any active session.
	newSessionID := uuid.NewString()
	cliExpiry := time.Now().UTC().Add(1 * time.Hour)
	created := models.CLISession{
		ID:                newSessionID,
		UserID:            newFields.UserID,
		OperatorSessionID: newFields.OperatorSessionID,
		SystemFingerprint: newFields.SystemFingerprint,
		CertFingerprint:   newCertFingerprint,
		CertSerial:        newCertSerial,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         cliExpiry,
		AbsoluteExpiresAt: cliExpiry,
		IdleExpiresAt:     cliExpiry,
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       newFields.LoginMethod,
	}
	createdBytes, err := json.Marshal(created)
	if err != nil {
		return nil, fmt.Errorf("replace CLI session: marshal new: %w", err)
	}
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), newSessionID, createdBytes); err != nil {
		return nil, fmt.Errorf("replace CLI session: persist new: %w", err)
	}

	// 3. Atomically deactivate the old session. Competing replacements or
	//    explicit DeactivateCLISession calls flip is_active to false; our
	//    conditional update only succeeds while it is still true.
	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionCLISessions),
		oldSessionID,
		map[string]interface{}{"is_active": false},
		"is_active", true,
	)
	if err != nil {
		return &created, fmt.Errorf("replace CLI session: deactivate old: %w", err)
	}
	if !applied {
		// A concurrent caller already deactivated the old session. The new
		// session is persisted and active; surface the typed error so the
		// caller can decide whether to revoke the cert it just signed.
		s.logger.Warn("ReplaceCLISession: old session already deactivated by concurrent caller",
			"old_session_id_prefix", safeTruncateID(oldSessionID, 8),
			"new_session_id_prefix", safeTruncateID(newSessionID, 8),
		)
		return &created, ErrCLISessionAlreadyDeactivated
	}

	s.logger.Info("CLI session replaced",
		"old_session_id_prefix", safeTruncateID(oldSessionID, 8),
		"new_session_id_prefix", safeTruncateID(newSessionID, 8),
		"user_id", newFields.UserID,
	)
	return &created, nil
}

// loadCLISession fetches a CLI session by ID and returns typed errors.
func (s *CLISessionService) loadCLISession(sessionID string) (*models.CLISession, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), sessionID)
	if err != nil {
		return nil, fmt.Errorf("load CLI session: %w", err)
	}
	if doc == nil {
		return nil, ErrCLISessionNotFound
	}
	return decodeCLISession(doc)
}

// decodeCLISession deserializes a Document into a CLISession.
func decodeCLISession(doc *models.Document) (*models.CLISession, error) {
	dataBytes, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal document data: %w", err)
	}
	var session models.CLISession
	if err := json.Unmarshal(dataBytes, &session); err != nil {
		return nil, fmt.Errorf("unmarshal CLI session: %w", err)
	}
	session.ID = doc.ID
	return &session, nil
}

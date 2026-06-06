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
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// SessionsService centralizes the logic for creating and binding Operator and CLI sessions.
type SessionsService struct {
	db     *GatewayDBService
	logger *slog.Logger
}

// NewSessionService creates a new SessionsService instance.
func NewSessionService(db *GatewayDBService, logger *slog.Logger) *SessionsService {
	return &SessionsService{
		db:     db,
		logger: logger,
	}
}

// PersistSessions binds an Operator session and a CLI session and persists both documents.
// If cliSessionID is empty (operator-only enrollment), only the operator session is persisted.
func (s *SessionsService) PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod string) error {
	// CLI session id is a first-class session type, strictly disjoint from
	// operator_session_id. The operator_session_id authenticates the host
	// agent (mTLS URI SAN); the cli_session_id is the routing namespace
	// the BYO/CLI client uses to receive SessionEvents (SSE) and embed in
	// outbound request bodies. Conflating the two would let an operator
	// session drain another client's event stream - and vice versa.

	// Only persist CLI session if cliSessionID is provided (not operator-only enrollment)
	if cliSessionID != "" {
		// Store the binding between operator_session_id and cli_session_id in a first-class
		// collection to support metadata, expiry, and revocation. Without this binding,
		// any authenticated Operator could drain any cli_session_id's event buffer.
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
	}

	// Write an operator_sessions document so g8e-compatible agentic ensembles can look up the
	// session by ID via GET /db/operator_sessions/{operator_session_id}.
	// Field names match the canonical Operator session document schema.
	sessionExpiry := time.Now().UTC().Add(1 * time.Hour)
	operatorSessionDoc := map[string]interface{}{
		"id":                  operatorSessionID,
		"session_type":        string(constants.SessionTypeCLI),
		"user_id":             userID,
		"organization_id":     orgID,
		"operator_id":         operatorID,
		"is_active":           true,
		"created_at":          time.Now().UTC().Format(time.RFC3339),
		"absolute_expires_at": sessionExpiry.Format(time.RFC3339),
		"idle_expires_at":     sessionExpiry.Format(time.RFC3339),
		"last_activity":       time.Now().UTC().Format(time.RFC3339),
		"login_method":        loginMethod,
	}
	operatorSessionBytes, err := json.Marshal(operatorSessionDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal Operator session document: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID, operatorSessionBytes); err != nil {
		s.logger.Error("Failed to persist Operator session document", string(constants.ConnectionStateError), err)
		return fmt.Errorf("failed to persist Operator session document: %w", err)
	}

	return nil
}

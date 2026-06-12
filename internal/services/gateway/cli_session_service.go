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

// CLISessionService handles CLI session persistence and management.
// CLI sessions are strictly disjoint from operator sessions - they represent
// the routing namespace for BYO/CLI clients to receive SessionEvents (SSE)
// and embed in outbound request bodies.
type CLISessionService struct {
	db     *CanonicalDBService
	logger *slog.Logger
}

// NewCLISessionService creates a new CLISessionService instance.
func NewCLISessionService(db *CanonicalDBService, logger *slog.Logger) *CLISessionService {
	return &CLISessionService{
		db:     db,
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

	if err := s.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes); err != nil {
		s.logger.Error("Failed to persist CLI session", string(constants.ConnectionStateError), err)
		return fmt.Errorf("failed to persist CLI session: %w", err)
	}

	return nil
}

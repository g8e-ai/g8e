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

// OperatorSessionService handles operator session persistence and management.
// Operator sessions authenticate the host agent via mTLS URI SAN and are used
// by g8e-compatible agentic ensembles to look up sessions by ID.
type OperatorSessionService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewOperatorSessionService creates a new OperatorSessionService instance.
func NewOperatorSessionService(docStore *DocumentStoreService, logger *slog.Logger) *OperatorSessionService {
	return &OperatorSessionService{
		db:     docStore,
		logger: logger,
	}
}

// PersistOperatorSession creates and persists an operator session document.
// Field names match the canonical Operator session document schema.
func (s *OperatorSessionService) PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod string) error {
	sessionExpiry := time.Now().UTC().Add(1 * time.Hour)
	now := time.Now().UTC()

	operatorSessionDoc := models.OperatorSession{
		ID:                operatorSessionID,
		SessionType:       string(constants.SessionTypeOperator),
		UserID:            userID,
		OrganizationID:    orgID,
		OperatorID:        operatorID,
		IsActive:          true,
		CreatedAt:         now.Format(time.RFC3339),
		AbsoluteExpiresAt: sessionExpiry.Format(time.RFC3339),
		IdleExpiresAt:     sessionExpiry.Format(time.RFC3339),
		LastActivity:      now.Format(time.RFC3339),
		LoginMethod:       loginMethod,
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

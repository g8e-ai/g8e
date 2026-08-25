// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

// GetActiveSessionForUser returns the first active operator session for the
// given user ID, or nil if none exists. Used by the CLI refresh controller
// to inherit an operator binding when the old CLI session is missing (e.g.,
// after a gateway volume reset that wiped CLI sessions but left operator
// sessions intact).
func (s *OperatorSessionService) GetActiveSessionForUser(userID string) (*models.OperatorSession, error) {
	userIDVal, err := json.Marshal(userID)
	if err != nil {
		return nil, fmt.Errorf("marshal user_id filter: %w", err)
	}
	activeVal, err := json.Marshal(true)
	if err != nil {
		return nil, fmt.Errorf("marshal is_active filter: %w", err)
	}
	docs, err := s.db.DocQuery(
		marshaler.CollectionName(constants.CollectionOperatorSessions),
		[]models.DocFilter{
			{Field: "user_id", Op: "==", Value: userIDVal},
			{Field: "is_active", Op: "==", Value: activeVal},
		},
		"created_at DESC",
		1,
	)
	if err != nil {
		return nil, fmt.Errorf("query active operator sessions for user %s: %w", userID, err)
	}
	if len(docs) == 0 {
		return nil, nil
	}
	dataBytes, err := json.Marshal(docs[0].Data)
	if err != nil {
		return nil, fmt.Errorf("marshal operator session document: %w", err)
	}
	var session models.OperatorSession
	if err := json.Unmarshal(dataBytes, &session); err != nil {
		return nil, fmt.Errorf("unmarshal operator session: %w", err)
	}
	session.ID = docs[0].ID
	return &session, nil
}

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
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

func (s *OperatorSessionService) DeactivateOperatorSession(operatorSessionID string) error {
	if operatorSessionID == "" {
		return constants.ErrGatewayOperatorSessionIDRequired
	}
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
	if err != nil {
		return fmt.Errorf("deactivate operator session: load: %w", err)
	}
	if doc == nil {
		return constants.ErrGatewayOperatorSessionInvalid
	}
	update, err := json.Marshal(map[string]any{"is_active": false})
	if err != nil {
		return fmt.Errorf("deactivate operator session: marshal: %w", err)
	}
	if _, err := s.db.DocUpdate(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID, update); err != nil {
		return fmt.Errorf("deactivate operator session: update: %w", err)
	}
	return nil
}

// GetActiveSessionForUser returns the active operator session the CLI
// should bind to for the given user ID, or nil if none exists. Used by the
// CLI refresh controller to inherit an operator binding when the old CLI
// session is missing (e.g., after a gateway volume reset that wiped CLI
// sessions but left operator sessions intact).
//
// When multiple active sessions exist, the session bound to the gateway's
// embedded operator is preferred — the embedded operator is the canonical
// local binding — otherwise the newest session wins.
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
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("query active operator sessions for user %s: %w", userID, err)
	}
	if len(docs) == 0 {
		return nil, nil
	}

	embedded := string(constants.DocIDEmbeddedOperator)
	selected := docs[0] // newest by created_at DESC
	for _, doc := range docs {
		dataBytes, err := json.Marshal(doc.Data)
		if err != nil {
			return nil, fmt.Errorf("marshal operator session document: %w", err)
		}
		var session models.OperatorSession
		if err := json.Unmarshal(dataBytes, &session); err != nil {
			return nil, fmt.Errorf("unmarshal operator session: %w", err)
		}
		if session.OperatorID == embedded {
			selected = doc
			break
		}
	}

	dataBytes, err := json.Marshal(selected.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal operator session document: %w", err)
	}
	var session models.OperatorSession
	if err := json.Unmarshal(dataBytes, &session); err != nil {
		return nil, fmt.Errorf("unmarshal operator session: %w", err)
	}
	session.ID = selected.ID
	return &session, nil
}

// GetActiveDataOperatorSessionForUser returns the operator session binding for
// the user's active governed tool Operator as recorded in the operator
// registry. Registry state wins over stale operator_sessions rows after stack
// rebuilds or remote Operator re-enrollment.
func (s *OperatorSessionService) GetActiveDataOperatorSessionForUser(userID string) (*models.OperatorSession, error) {
	if userID == "" {
		return nil, nil
	}
	userIDVal, err := json.Marshal(userID)
	if err != nil {
		return nil, fmt.Errorf("marshal user_id filter: %w", err)
	}
	activeStatus, err := json.Marshal(string(constants.OperatorStatusActive))
	if err != nil {
		return nil, fmt.Errorf("marshal status filter: %w", err)
	}
	docs, err := s.db.DocQuery(
		marshaler.CollectionName(constants.CollectionOperators),
		[]models.DocFilter{
			{Field: "user_id", Op: "==", Value: userIDVal},
			{Field: "status", Op: "==", Value: activeStatus},
		},
		"created_at DESC",
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("query active operators for user %s: %w", userID, err)
	}
	for _, doc := range docs {
		dataBytes, err := json.Marshal(doc.Data)
		if err != nil {
			return nil, fmt.Errorf("marshal operator document: %w", err)
		}
		var operator models.OperatorDocumentGo
		if err := json.Unmarshal(dataBytes, &operator); err != nil {
			return nil, fmt.Errorf("unmarshal operator document: %w", err)
		}
		if operator.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if operator.RuntimeConfig != nil && operator.RuntimeConfig.InferenceEnabled {
			continue
		}
		if operator.OperatorSessionID == "" {
			continue
		}
		return &models.OperatorSession{
			ID:         operator.OperatorSessionID,
			UserID:     operator.UserID,
			OperatorID: operator.ID,
			IsActive:   true,
		}, nil
	}
	return nil, nil
}

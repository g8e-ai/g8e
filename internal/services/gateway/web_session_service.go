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

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// WebSessionService handles web session persistence and management.
// Web sessions are used for browser-based authentication via WebAuthn/passkeys.
type WebSessionService struct {
	db     *CanonicalDBService
	logger *slog.Logger
}

// NewWebSessionService creates a new WebSessionService instance.
func NewWebSessionService(db *CanonicalDBService, logger *slog.Logger) *WebSessionService {
	return &WebSessionService{
		db:     db,
		logger: logger,
	}
}

// CreateWebSession creates a new web session after successful authentication.
func (s *WebSessionService) CreateWebSession(userID string) (*models.WebSession, error) {
	webSessionID := uuid.New().String()
	now := time.Now()

	webSession := &models.WebSession{
		ID:              webSessionID,
		UserID:          userID,
		CreatedAtUnixMs: now.UnixMilli(),
		ExpiresAtUnixMs: now.Add(constants.WebSessionTTL).UnixMilli(),
	}

	data, err := json.Marshal(webSession)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal web session: %w", err)
	}
	if err := s.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID, data); err != nil {
		s.logger.Error("Failed to create web session", "error", err, "userID", userID)
		return nil, fmt.Errorf("gateway: create web session: %w", err)
	}

	s.logger.Info("Web session created", "userID", userID, "webSessionID", webSessionID[:8])
	return webSession, nil
}

// ValidateWebSession validates a web session by ID and returns the session if valid.
func (s *WebSessionService) ValidateWebSession(webSessionID string) (*models.WebSession, error) {
	doc, err := s.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID)
	if err != nil {
		return nil, fmt.Errorf("gateway: validate web session: %w", err)
	}
	if doc == nil {
		return nil, constants.ErrNotFound
	}

	dataBytes, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal web session data: %w", err)
	}
	var webSession models.WebSession
	if err := json.Unmarshal(dataBytes, &webSession); err != nil {
		return nil, fmt.Errorf("gateway: unmarshal web session: %w", err)
	}

	webSession.ID = webSessionID

	if time.Now().UnixMilli() > webSession.ExpiresAtUnixMs {
		return nil, constants.ErrExpired
	}

	return &webSession, nil
}

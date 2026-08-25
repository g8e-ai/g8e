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
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// WebSessionService handles web session persistence and management.
// Web sessions are used for browser-based authentication via WebAuthn/passkeys.
type WebSessionService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewWebSessionService creates a new WebSessionService instance.
func NewWebSessionService(docStore *DocumentStoreService, logger *slog.Logger) *WebSessionService {
	return &WebSessionService{
		db:     docStore,
		logger: logger,
	}
}

// CreateWebSession creates a new web session after successful authentication.
func (s *WebSessionService) CreateWebSession(userID string) (*models.WebSession, error) {
	webSessionID := uuid.NewString()
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
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID, data); err != nil {
		s.logger.Error("Failed to create web session", "error", err, "userID", userID)
		return nil, fmt.Errorf("gateway: create web session: %w", err)
	}

	s.logger.Info("Web session created", "userID", userID, "webSessionID", webSessionID[:8])
	return webSession, nil
}

// ValidateWebSession validates a web session by ID and returns the session if valid.
func (s *WebSessionService) ValidateWebSession(webSessionID string) (*models.WebSession, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID)
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

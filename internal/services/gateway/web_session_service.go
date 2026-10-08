// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
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
func (s *WebSessionService) CreateWebSession(ctx context.Context, userID string) (*models.WebSession, error) {
	webSessionID, err := uuid.NewString()
	if err != nil {
		return nil, err
	}
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
	if err := s.db.DocSet(ctx, marshaler.CollectionName(constants.CollectionWebSessions), webSessionID, data); err != nil {
		s.logger.Error("Failed to create web session", "error", err, "userID", userID)
		return nil, fmt.Errorf("gateway: create web session: %w", err)
	}

	s.logger.Info("Web session created", "userID", userID, "webSessionID", webSessionID[:8])
	return webSession, nil
}

// ValidateWebSession validates a web session by ID and returns the session if valid.
func (s *WebSessionService) ValidateWebSession(ctx context.Context, webSessionID string) (*models.WebSession, error) {
	doc, err := s.db.DocGet(ctx, marshaler.CollectionName(constants.CollectionWebSessions), webSessionID)
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

// TerminateUserWebSessions deletes every web session document owned by
// userID, expired or not, and returns how many of them were still live. A
// deleted document can no longer authenticate a browser, which is what the
// single-session logout endpoint does for its own cookie.
func (s *WebSessionService) TerminateUserWebSessions(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, fmt.Errorf("gateway: terminate user web sessions: %w", constants.ErrRegistrationUserIDRequired)
	}
	docs, err := s.db.DocQuery(ctx, marshaler.CollectionName(constants.CollectionWebSessions), []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
	}, "", 0)
	if err != nil {
		return 0, fmt.Errorf("gateway: terminate user web sessions: query: %w", err)
	}

	nowMs := time.Now().UnixMilli()
	live := 0
	var errs []error
	for _, doc := range docs {
		wire, err := json.Marshal(doc.ForWire())
		if err != nil {
			errs = append(errs, fmt.Errorf("marshal web session %s: %w", doc.ID, err))
			continue
		}
		var session models.WebSession
		if err := json.Unmarshal(wire, &session); err != nil {
			errs = append(errs, fmt.Errorf("decode web session %s: %w", doc.ID, err))
			continue
		}
		deleted, err := s.db.DocDeleteWithResult(ctx, marshaler.CollectionName(constants.CollectionWebSessions), doc.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("delete web session %s: %w", doc.ID, err))
			continue
		}
		if deleted && nowMs <= session.ExpiresAtUnixMs {
			live++
		}
	}
	if len(errs) > 0 {
		return live, fmt.Errorf("gateway: terminate user web sessions: %w", errors.Join(errs...))
	}
	return live, nil
}

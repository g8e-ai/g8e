// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

const (
	// enrollmentTokenTTL is the lifetime of an enrollment token (5 minutes)
	enrollmentTokenTTL = 5 * time.Minute
	// enrollmentTokenBytes is the number of random bytes to generate for the token
	enrollmentTokenBytes = 32
)

// EnrollmentTokenService manages one-time enrollment tokens for secure passkey registration.
type EnrollmentTokenService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewEnrollmentTokenService creates a new EnrollmentTokenService.
func NewEnrollmentTokenService(docStore *DocumentStoreService, logger *slog.Logger) *EnrollmentTokenService {
	return &EnrollmentTokenService{
		db:     docStore,
		logger: logger,
	}
}

// GenerateToken creates a new one-time enrollment token for the given user and CLI session.
// The token expires after enrollmentTokenTTL and can only be used once.
func (s *EnrollmentTokenService) GenerateToken(userID, cliSessionID string) (*models.EnrollmentToken, error) {
	// Generate random 32-byte token
	tokenBytes := make([]byte, enrollmentTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("Failed to generate enrollment token", "error", err, "user_id", userID)
		return nil, constants.ErrEnrollmentTokenGenerationFailed
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	enrollmentToken := &models.EnrollmentToken{
		Token:        token,
		UserID:       userID,
		CLISessionID: cliSessionID,
		CreatedAt:    now,
		ExpiresAt:    now.Add(enrollmentTokenTTL),
		Consumed:     false,
	}

	// Persist the token
	tokenData, err := json.Marshal(enrollmentToken)
	if err != nil {
		s.logger.Error("Failed to marshal enrollment token", "error", err, "user_id", userID)
		return nil, constants.ErrEnrollmentTokenPersistenceFailed
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), token, tokenData); err != nil {
		s.logger.Error("Failed to persist enrollment token", "error", err, "user_id", userID)
		return nil, constants.ErrEnrollmentTokenPersistenceFailed
	}

	cliSessionIDPrefix := cliSessionID
	if len(cliSessionIDPrefix) > 8 {
		cliSessionIDPrefix = cliSessionIDPrefix[:8]
	}
	s.logger.Info("Generated enrollment token", "user_id", userID, "cli_session_id_prefix", cliSessionIDPrefix, "token_prefix", token[:8])
	return enrollmentToken, nil
}

// ValidateToken checks if a token is valid, not expired, and not already
// consumed, WITHOUT consuming it. It returns the associated user_id and
// cli_session_id. Use this for the challenge step of a two-phase ceremony
// (challenge + verify) so the token is only consumed on the verifying step.
// A consumed token is still rejected (it is no longer valid), so retrying a
// challenge after a successful verify is correctly blocked.
func (s *EnrollmentTokenService) ValidateToken(token string) (*models.EnrollmentToken, error) {
	enrollmentToken, err := s.loadAndCheckToken(token)
	if err != nil {
		return nil, err
	}
	return enrollmentToken, nil
}

// ValidateAndConsumeToken checks if a token is valid, not expired, and not already consumed.
// If valid, it atomically marks the token as consumed and returns the associated user_id and cli_session_id.
// The atomic conditional UPDATE prevents TOCTOU races where concurrent callers could both
// read consumed=false before either writes consumed=true.
func (s *EnrollmentTokenService) ValidateAndConsumeToken(token string) (*models.EnrollmentToken, error) {
	enrollmentToken, err := s.loadAndCheckToken(token)
	if err != nil {
		return nil, err
	}

	// Atomically mark token as consumed: only updates if consumed is still false.
	// This prevents TOCTOU races where concurrent callers both read consumed=false.
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionEnrollmentTokens),
		token,
		map[string]interface{}{
			"consumed":    true,
			"consumed_at": nowStr,
		},
		"consumed", 0,
	)
	if err != nil {
		tokenPrefix := token
		if len(tokenPrefix) > 8 {
			tokenPrefix = tokenPrefix[:8]
		}
		s.logger.Error("Failed to atomically consume enrollment token", "error", err, "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenPersistenceFailed
	}
	if !applied {
		// Another goroutine consumed it between our read and the conditional update.
		tokenPrefix := token
		if len(tokenPrefix) > 8 {
			tokenPrefix = tokenPrefix[:8]
		}
		s.logger.Warn("Enrollment token consumed by concurrent caller", "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenConsumed
	}

	enrollmentToken.Consumed = true
	enrollmentToken.ConsumedAt = &now

	cliSessionIDPrefix := enrollmentToken.CLISessionID
	if len(cliSessionIDPrefix) > 8 {
		cliSessionIDPrefix = cliSessionIDPrefix[:8]
	}
	tokenPrefix := token
	if len(tokenPrefix) > 8 {
		tokenPrefix = tokenPrefix[:8]
	}
	s.logger.Info("Enrollment token consumed", "user_id", enrollmentToken.UserID, "cli_session_id_prefix", cliSessionIDPrefix, "token_prefix", tokenPrefix)
	return enrollmentToken, nil
}

// loadAndCheckToken retrieves an enrollment token by its opaque value and
// validates that it exists, is not expired, and has not already been consumed.
// It does NOT mutate the persisted token. Callers that need to consume the
// token must follow up with a conditional update (see ValidateAndConsumeToken).
func (s *EnrollmentTokenService) loadAndCheckToken(token string) (*models.EnrollmentToken, error) {
	tokenPrefix := token
	if len(tokenPrefix) > 8 {
		tokenPrefix = tokenPrefix[:8]
	}

	// Retrieve the token
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), token)
	if err != nil {
		s.logger.Error("Failed to look up enrollment token", "error", err, "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenInvalid
	}
	if doc == nil {
		s.logger.Warn("Enrollment token not found", "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenInvalid
	}

	dataBytes, err := json.Marshal(doc.Data)
	if err != nil {
		s.logger.Error("Failed to marshal enrollment token document", "error", err, "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenInvalid
	}

	var enrollmentToken models.EnrollmentToken
	if err := json.Unmarshal(dataBytes, &enrollmentToken); err != nil {
		s.logger.Error("Failed to unmarshal enrollment token", "error", err, "token_prefix", tokenPrefix)
		return nil, constants.ErrEnrollmentTokenInvalid
	}

	// Check if token is expired
	if time.Now().UTC().After(enrollmentToken.ExpiresAt) {
		s.logger.Warn("Enrollment token expired", "token_prefix", tokenPrefix, "expired_at", enrollmentToken.ExpiresAt)
		return nil, constants.ErrEnrollmentTokenExpired
	}

	// Check if token is already consumed
	if enrollmentToken.Consumed {
		s.logger.Warn("Enrollment token already consumed", "token_prefix", tokenPrefix, "consumed_at", enrollmentToken.ConsumedAt)
		return nil, constants.ErrEnrollmentTokenConsumed
	}

	return &enrollmentToken, nil
}

// CleanupExpiredTokens removes tokens that have expired from the database.
// This should be called periodically to prevent unbounded growth of the
// enrollment_tokens collection.
//
// Note: This relies on lexicographic string comparison of RFC3339 timestamps,
// which works correctly for UTC values (lexicographic order matches chronological
// order). This assumes expires_at is always stored as fixed-microsecond UTC.
func (s *EnrollmentTokenService) CleanupExpiredTokens() error {
	now := timesvc.NowTimestamp()
	filters := []models.DocFilter{
		{Field: "expires_at", Op: "<", Value: json.RawMessage(`"` + now + `"`)},
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionEnrollmentTokens), filters, "", 0)
	if err != nil {
		return fmt.Errorf("enrollment token: cleanup: query: %w", err)
	}

	var deleted int
	for _, doc := range docs {
		err := s.db.DocDelete(marshaler.CollectionName(constants.CollectionEnrollmentTokens), doc.ID)
		if err != nil {
			s.logger.Warn("Failed to delete expired enrollment token", "token_id", doc.ID, "error", err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		s.logger.Info("Cleaned up expired enrollment tokens", "count", deleted)
	}
	return nil
}

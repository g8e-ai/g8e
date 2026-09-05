// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newTestEnrollmentTokenService(t *testing.T) *EnrollmentTokenService {
	t.Helper()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	return NewEnrollmentTokenService(db.GetDocStore(), logger)
}

func TestEnrollmentToken_GenerateAndValidateRoundTrip(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	token, err := svc.GenerateToken("user-123", "cli-session-456")
	require.NoError(t, err)
	assert.NotEmpty(t, token.Token)
	assert.Equal(t, "user-123", token.UserID)
	assert.Equal(t, "cli-session-456", token.CLISessionID)
	assert.False(t, token.Consumed)

	consumed, err := svc.ValidateAndConsumeToken(token.Token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", consumed.UserID)
	assert.Equal(t, "cli-session-456", consumed.CLISessionID)
	assert.True(t, consumed.Consumed)
	assert.NotNil(t, consumed.ConsumedAt)
}

func TestEnrollmentToken_ValidateUnknownTokenReturnsInvalid(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	_, err := svc.ValidateAndConsumeToken("nonexistenttoken1234567890abcdef")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))
}

func TestEnrollmentToken_ValidateExpiredTokenReturnsExpired(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	expiredToken := &models.EnrollmentToken{
		Token:        "expiredtoken1234567890abcdef1234567890ab",
		UserID:       "user-exp",
		CLISessionID: "cli-exp",
		CreatedAt:    time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:    time.Now().UTC().Add(-5 * time.Minute),
		Consumed:     false,
	}
	data, err := json.Marshal(expiredToken)
	require.NoError(t, err)
	err = svc.db.DocSet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), expiredToken.Token, data)
	require.NoError(t, err)

	_, err = svc.ValidateAndConsumeToken(expiredToken.Token)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenExpired))
}

func TestEnrollmentToken_ConsumedTokenCannotBeReused(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	token, err := svc.GenerateToken("user-reuse", "cli-reuse")
	require.NoError(t, err)

	first, err := svc.ValidateAndConsumeToken(token.Token)
	require.NoError(t, err)
	assert.True(t, first.Consumed)

	_, err = svc.ValidateAndConsumeToken(token.Token)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenConsumed))
}

func TestEnrollmentToken_ShortStringDoesNotPanic(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	_, err := svc.ValidateAndConsumeToken("")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))

	_, err = svc.ValidateAndConsumeToken("x")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))
}

func TestEnrollmentToken_GenerateTokenShortCLISessionIDDoesNotPanic(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	// cliSessionID shorter than 8 characters should not panic on prefix slicing
	token, err := svc.GenerateToken("user-short", "abc")
	require.NoError(t, err)
	assert.NotEmpty(t, token.Token)
	assert.Equal(t, "abc", token.CLISessionID)

	// Empty cliSessionID should also not panic
	token2, err := svc.GenerateToken("user-empty", "")
	require.NoError(t, err)
	assert.NotEmpty(t, token2.Token)
	assert.Equal(t, "", token2.CLISessionID)
}

func TestEnrollmentToken_ValidateTokenDoesNotConsume(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	token, err := svc.GenerateToken("user-validate", "cli-validate")
	require.NoError(t, err)

	// ValidateToken must succeed and NOT mark the token consumed.
	got, err := svc.ValidateToken(token.Token)
	require.NoError(t, err)
	assert.Equal(t, "user-validate", got.UserID)
	assert.Equal(t, "cli-validate", got.CLISessionID)
	assert.False(t, got.Consumed)

	// A second ValidateToken on the same token still succeeds — the
	// token remains unconsumed and reusable for the verify step.
	got2, err := svc.ValidateToken(token.Token)
	require.NoError(t, err)
	assert.False(t, got2.Consumed)

	// ValidateAndConsumeToken must still work afterwards (the
	// non-consuming validate did not consume it).
	consumed, err := svc.ValidateAndConsumeToken(token.Token)
	require.NoError(t, err)
	assert.True(t, consumed.Consumed)

	// Now ValidateToken must reject it as consumed.
	_, err = svc.ValidateToken(token.Token)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenConsumed))
}

func TestEnrollmentToken_CleanupExpiredTokens(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	// Insert an expired token via direct DocSet
	expiredToken := &models.EnrollmentToken{
		Token:        "expiredtoken1234567890abcdef1234567890ab",
		UserID:       "user-expired",
		CLISessionID: "cli-expired",
		CreatedAt:    time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:    time.Now().UTC().Add(-5 * time.Minute),
		Consumed:     false,
	}
	expiredData, err := json.Marshal(expiredToken)
	require.NoError(t, err)
	err = svc.db.DocSet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), expiredToken.Token, expiredData)
	require.NoError(t, err)

	// Insert a valid (non-expired) token via GenerateToken
	validToken, err := svc.GenerateToken("user-valid", "cli-valid")
	require.NoError(t, err)

	// Call CleanupExpiredTokens
	err = svc.CleanupExpiredTokens()
	require.NoError(t, err)

	// Verify the expired token is deleted
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), expiredToken.Token)
	require.NoError(t, err)
	assert.Nil(t, doc, "expired token should be deleted")

	// Verify the valid token remains
	doc, err = svc.db.DocGet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), validToken.Token)
	require.NoError(t, err)
	assert.NotNil(t, doc, "valid token should remain")
}

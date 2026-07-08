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

//go:build integration

package gateway

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func newTestEnrollmentTokenService(t *testing.T) *EnrollmentTokenService {
	t.Helper()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	return NewEnrollmentTokenService(db, logger)
}

func TestEnrollmentToken_GenerateAndValidateRoundTrip(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	svc := newTestEnrollmentTokenService(t)

	_, err := svc.ValidateAndConsumeToken("nonexistenttoken1234567890abcdef")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))
}

func TestEnrollmentToken_ValidateExpiredTokenReturnsExpired(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	svc := newTestEnrollmentTokenService(t)

	_, err := svc.ValidateAndConsumeToken("")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))

	_, err = svc.ValidateAndConsumeToken("x")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid))
}

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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// TestEnrollmentToken_ConcurrentValidateAndConsume verifies that concurrent
// calls to ValidateAndConsumeToken on the same token are safe — exactly one
// goroutine succeeds in consuming the token, and the rest receive
// ErrEnrollmentTokenConsumed. This guards against TOCTOU race conditions
// in the consume path.
func TestEnrollmentToken_ConcurrentValidateAndConsume(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	token, err := svc.GenerateToken("user-concurrent", "cli-concurrent")
	require.NoError(t, err)

	const numConcurrent = 10
	var wg sync.WaitGroup
	errs := make(chan error, numConcurrent)
	successes := make(chan bool, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumed, err := svc.ValidateAndConsumeToken(token.Token)
			if err != nil {
				errs <- err
			} else {
				successes <- consumed != nil
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(successes)

	successCount := 0
	for range successes {
		successCount++
	}

	consumedCount := 0
	for err := range errs {
		if errors.Is(err, constants.ErrEnrollmentTokenConsumed) {
			consumedCount++
		}
	}

	assert.Equal(t, 1, successCount, "exactly one goroutine should succeed")
	assert.Equal(t, numConcurrent-1, consumedCount, "remaining goroutines should get ErrEnrollmentTokenConsumed")
}

// TestEnrollmentToken_PersistenceFailureDuringGenerate verifies that when
// the database is unavailable during DocSet in GenerateToken, the service
// returns ErrEnrollmentTokenPersistenceFailed rather than panicking or
// returning a nil token.
func TestEnrollmentToken_PersistenceFailureDuringGenerate(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewEnrollmentTokenService(db, logger)

	// Close the DB to simulate a persistence failure.
	db.Close()

	_, err := svc.GenerateToken("user-persist-fail", "cli-persist-fail")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenPersistenceFailed),
		"expected ErrEnrollmentTokenPersistenceFailed, got %v", err)
}

// TestEnrollmentToken_MalformedTokenInStorage verifies that when a token
// document in the database has corrupted field types (simulating storage
// corruption), ValidateAndConsumeToken returns ErrEnrollmentTokenInvalid
// rather than panicking.
func TestEnrollmentToken_MalformedTokenInStorage(t *testing.T) {
	svc := newTestEnrollmentTokenService(t)

	// Insert a document with wrong field types for EnrollmentToken.
	// The "token" field is a number instead of a string, and "consumed"
	// is a string instead of a bool. This simulates storage corruption.
	malformedData := json.RawMessage(`{"token": 12345, "user_id": "user-x", "cli_session_id": "cli-x", "consumed": "not-a-bool", "expires_at": "2099-01-01T00:00:00Z", "created_at": "2026-01-01T00:00:00Z"}`)
	err := svc.db.DocSet(marshaler.CollectionName(constants.CollectionEnrollmentTokens), "malformedtoken1234567890abcdef1234567890ab", malformedData)
	require.NoError(t, err)

	_, err = svc.ValidateAndConsumeToken("malformedtoken1234567890abcdef1234567890ab")
	assert.True(t, errors.Is(err, constants.ErrEnrollmentTokenInvalid),
		"expected ErrEnrollmentTokenInvalid for malformed token in storage, got %v", err)
}

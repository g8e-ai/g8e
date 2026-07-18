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

package lattice

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryWithBackoff_ReturnsNilOnFirstSuccess(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return nil
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetryWithBackoff_RetriesOnUnavailableAndSucceedsOnAttempt2(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		c := calls.Add(1)
		if c == 1 {
			return status.Error(codes.Unavailable, "transient")
		}
		return nil
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestRetryWithBackoff_DoesNotRetryOnInvalidArgument(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return status.Error(codes.InvalidArgument, "bad request")
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetryWithBackoff_DoesNotRetryOnPermissionDenied(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return status.Error(codes.PermissionDenied, "denied")
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetryWithBackoff_DoesNotRetryOnUnauthenticated(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return status.Error(codes.Unauthenticated, "auth required")
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetryWithBackoff_DoesNotRetryOnNonGRPCError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return errors.New("plain error")
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetryWithBackoff_RespectsContextCancellationDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32

	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()

	err := retryWithBackoff(ctx, func(ctx context.Context) error {
		calls.Add(1)
		return status.Error(codes.Unavailable, "transient")
	}, RetryOpts{MaxAttempts: 5, InitialBackoff: 50 * time.Millisecond})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryWithBackoff_ReturnsLastErrorAfterMaxAttemptsExhausted(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(ctx context.Context) error {
		calls.Add(1)
		return status.Error(codes.Unavailable, "persistent failure")
	}, RetryOpts{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, int32(3), calls.Load())
}

func TestIsRetryable_ClassifiesRetryableCodes(t *testing.T) {
	t.Parallel()

	retryable := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
		codes.Internal,
		codes.Aborted,
		codes.Unknown,
		codes.NotFound,
	}

	for _, code := range retryable {
		code := code
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			assert.True(t, isRetryable(status.Error(code, "test")))
		})
	}
}

func TestIsRetryable_ClassifiesTerminalCodesAsNotRetryable(t *testing.T) {
	t.Parallel()

	terminal := []codes.Code{
		codes.InvalidArgument,
		codes.PermissionDenied,
		codes.Unauthenticated,
		codes.FailedPrecondition,
		codes.AlreadyExists,
	}

	for _, code := range terminal {
		code := code
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			assert.False(t, isRetryable(status.Error(code, "test")))
		})
	}
}

func TestIsRetryable_ReturnsFalseForNonGRPCError(t *testing.T) {
	t.Parallel()

	assert.False(t, isRetryable(errors.New("plain error")))
}

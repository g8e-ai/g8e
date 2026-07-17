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
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryOpts configures retryWithBackoff behavior.
type RetryOpts struct {
	MaxAttempts    int
	InitialBackoff time.Duration
}

// DefaultRetryOpts returns the standard retry configuration for unary RPCs.
func DefaultRetryOpts() RetryOpts {
	return RetryOpts{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Second,
	}
}

// retryableCodes classifies gRPC status codes as retryable or terminal.
// Based on the Lattice retry guide.
var retryableCodes = map[codes.Code]bool{
	codes.Unavailable:       true,
	codes.DeadlineExceeded:  true,
	codes.ResourceExhausted: true,
	codes.Internal:          true,
	codes.Aborted:           true,
	codes.Unknown:           true,
	codes.NotFound:          true, // eventual consistency
}

// isRetryable returns true if the error's gRPC status code is classified as
// retryable. Non-gRPC errors are not retried.
func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return retryableCodes[st.Code()]
}

// retryWithBackoff retries op with exponential backoff and jitter.
// It is for unary RPCs only — stream reconnect uses a separate backoff loop.
// UNAUTHENTICATED is not retried here; the caller wraps op with refresh logic.
func retryWithBackoff(ctx context.Context, op func(context.Context) error, opts RetryOpts) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 1 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt < opts.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryable(err) {
			return err
		}

		if attempt < opts.MaxAttempts-1 {
			backoff := opts.InitialBackoff * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Int63n(int64(backoff) / 5))
			backoff = backoff + jitter - (jitter / 2)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}

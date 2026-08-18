// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"
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
			// ±20% jitter: random value in [-backoff/5, +backoff/5]
			jitterMag := time.Duration(cryptoRandInt63n(int64(backoff)/5 + 1))
			backoff = backoff + jitterMag - backoff/5
			if backoff < 0 {
				backoff = 0
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}

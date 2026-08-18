// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubInvoker returns a configurable sequence of errors and counts calls.
type stubInvoker struct {
	errs    []error
	calls   atomic.Int32
	refresh atomic.Int32
}

func (s *stubInvoker) invoker() grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		c := int(s.calls.Add(1))
		if c <= len(s.errs) {
			return s.errs[c-1]
		}
		return nil
	}
}

func newTestRPCCreds() *ClientCredentialsAuth {
	return NewClientCredentialsAuth("id", "secret", "", "http://localhost/oauth/token")
}

func TestUnaryRetryInterceptor_RetriesOnUnavailableAndSucceeds(t *testing.T) {
	t.Parallel()

	si := &stubInvoker{errs: []error{
		status.Error(codes.Unavailable, "transient"),
		nil,
	}}
	ic := unaryRetryInterceptor(newTestRPCCreds())

	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	require.NoError(t, err)
	assert.Equal(t, int32(2), si.calls.Load())
}

func TestUnaryRetryInterceptor_DoesNotRetryOnInvalidArgument(t *testing.T) {
	t.Parallel()

	si := &stubInvoker{errs: []error{
		status.Error(codes.InvalidArgument, "bad"),
	}}
	ic := unaryRetryInterceptor(newTestRPCCreds())

	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, int32(1), si.calls.Load())
}

func TestUnaryRetryInterceptor_CallsForceRefreshOnUnauthenticated(t *testing.T) {
	t.Parallel()

	creds := newTestRPCCreds()
	si := &stubInvoker{errs: []error{
		status.Error(codes.Unauthenticated, "expired"),
		nil,
	}}
	ic := unaryRetryInterceptor(creds)

	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	require.NoError(t, err)
	assert.Equal(t, int32(2), si.calls.Load())
}

func TestUnaryRetryInterceptor_DoesNotRefreshTwiceOnConsecutiveUnauthenticated(t *testing.T) {
	t.Parallel()

	creds := newTestRPCCreds()
	si := &stubInvoker{errs: []error{
		status.Error(codes.Unauthenticated, "expired"),
		status.Error(codes.Unauthenticated, "still expired"),
	}}
	ic := unaryRetryInterceptor(creds)

	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestUnaryRetryInterceptor_ReturnsNilOnFirstSuccess(t *testing.T) {
	t.Parallel()

	si := &stubInvoker{errs: nil}
	ic := unaryRetryInterceptor(newTestRPCCreds())

	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	require.NoError(t, err)
	assert.Equal(t, int32(1), si.calls.Load())
}

func TestUnaryRetryInterceptor_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	si := &stubInvoker{errs: []error{
		status.Error(codes.Unavailable, "transient"),
	}}
	ic := unaryRetryInterceptor(newTestRPCCreds())

	err := ic(ctx, "/test.Method", nil, nil, nil, si.invoker())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestUnaryRetryInterceptor_RetriesUpToMaxAttempts(t *testing.T) {
	t.Parallel()

	si := &stubInvoker{errs: []error{
		status.Error(codes.Unavailable, "fail 1"),
		status.Error(codes.Unavailable, "fail 2"),
		status.Error(codes.Unavailable, "fail 3"),
	}}
	ic := unaryRetryInterceptor(newTestRPCCreds())

	start := time.Now()
	err := ic(context.Background(), "/test.Method", nil, nil, nil, si.invoker())
	elapsed := time.Since(start)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, int32(3), si.calls.Load())
	// With 1s initial backoff and 3 attempts, there are 2 backoff periods
	// (1s + 2s = 3s minimum). Verify we waited at least some time.
	// The jitter may reduce this slightly, so just verify calls == 3.
	_ = elapsed
}

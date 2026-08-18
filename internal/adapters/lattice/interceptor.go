// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unaryRetryInterceptor wraps unary RPCs with retryWithBackoff and
// DefaultRetryOpts. On codes.Unauthenticated, it calls rpcCreds.ForceRefresh()
// and retries the RPC once with the refreshed token. This is separate from
// retryWithBackoff because Unauthenticated is not in retryableCodes — the
// interceptor handles the refresh-and-retry loop itself.
func unaryRetryInterceptor(rpcCreds *ClientCredentialsAuth) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		opts_ := DefaultRetryOpts()

		var refreshed bool
		err := retryWithBackoff(ctx, func(ctx context.Context) error {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}

			st, ok := status.FromError(err)
			if ok && st.Code() == codes.Unauthenticated && !refreshed {
				rpcCreds.ForceRefresh()
				refreshed = true
				return status.Error(codes.Unavailable, "token refreshed, retrying")
			}

			refreshed = false
			return err
		}, opts_)

		return err
	}
}

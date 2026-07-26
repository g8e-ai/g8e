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
				return err
			}

			refreshed = false
			return err
		}, opts_)

		return err
	}
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import "context"

type attemptStoreKey struct{}

// WithAttemptStore binds provider response storage to one governed execution.
func WithAttemptStore(ctx context.Context, store AttemptStore) context.Context {
	return context.WithValue(ctx, attemptStoreKey{}, store)
}

func attemptStoreFromContext(ctx context.Context) AttemptStore {
	store, _ := ctx.Value(attemptStoreKey{}).(AttemptStore)
	return store
}

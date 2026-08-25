// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storagetest

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
)

type testEntry struct {
	value     string
	expiresAt time.Time
}

// TestTokenStore is an in-memory storage.TokenStore for use in unit tests.
// It is thread-safe and supports TTL expiry.
type TestTokenStore struct {
	mu   sync.RWMutex
	data map[string]testEntry
}

// NewTestTokenStore creates a new in-memory TestTokenStore.
func NewTestTokenStore() *TestTokenStore {
	return &TestTokenStore{data: make(map[string]testEntry)}
}

func (f *TestTokenStore) KVSet(_ context.Context, key, value string, ttlSeconds int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var exp time.Time
	if ttlSeconds > 0 {
		exp = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}
	f.data[key] = testEntry{value: value, expiresAt: exp}
	return nil
}

func (f *TestTokenStore) KVGet(_ context.Context, key string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	e, ok := f.data[key]
	if !ok {
		return "", constants.ErrKeyNotFound
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		return "", constants.ErrKeyNotFound
	}
	return e.value, nil
}

func (f *TestTokenStore) KVScanPrefix(_ context.Context, prefix string) (map[string]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]string)
	for k, e := range f.data {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
			continue
		}
		result[k] = e.value
	}
	return result, nil
}

var _ storage.TokenStore = (*TestTokenStore)(nil)

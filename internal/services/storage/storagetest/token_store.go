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

package storagetest

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
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

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

package keystore

import (
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
)

// sharedTestKeyStorage is package-level shared storage for test backend.
// This allows all test instances to share the same master key.
var sharedTestKeyStorage struct {
	mu  sync.RWMutex
	key []byte
}

// testBackend is an in-memory backend for testing only.
// It uses shared package-level storage so all instances share the same master key.
type testBackend struct{}

func NewTestBackend() (Backend, error) {
	return &testBackend{}, nil
}

func (b *testBackend) Name() string {
	return string(constants.EnvironmentTest)
}

func (b *testBackend) RetrieveMasterKey() ([]byte, error) {
	sharedTestKeyStorage.mu.RLock()
	defer sharedTestKeyStorage.mu.RUnlock()
	if sharedTestKeyStorage.key == nil {
		return nil, ErrKeyNotFound
	}
	keyCopy := make([]byte, len(sharedTestKeyStorage.key))
	copy(keyCopy, sharedTestKeyStorage.key)
	return keyCopy, nil
}

func (b *testBackend) StoreMasterKey(key []byte) error {
	sharedTestKeyStorage.mu.Lock()
	defer sharedTestKeyStorage.mu.Unlock()
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	sharedTestKeyStorage.key = keyCopy
	return nil
}

func (b *testBackend) DeleteMasterKey() error {
	sharedTestKeyStorage.mu.Lock()
	defer sharedTestKeyStorage.mu.Unlock()
	sharedTestKeyStorage.key = nil
	return nil
}

// ResetTestStorage clears the shared test key storage.
// This should be called in TestMain to prevent cross-test contamination.
func ResetTestStorage() {
	sharedTestKeyStorage.mu.Lock()
	defer sharedTestKeyStorage.mu.Unlock()
	sharedTestKeyStorage.key = nil
}

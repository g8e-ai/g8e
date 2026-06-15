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

// testBackend is an in-memory backend for testing only.
// It is instance-based, ensuring each test instance has its own master key.
type testBackend struct {
	mu  sync.RWMutex
	key []byte
}

func NewTestBackend() (Backend, error) {
	return &testBackend{}, nil
}

func (b *testBackend) Name() string {
	return string(constants.EnvironmentTest)
}

func (b *testBackend) RetrieveMasterKey() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.key == nil {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	keyCopy := make([]byte, len(b.key))
	copy(keyCopy, b.key)
	return keyCopy, nil
}

func (b *testBackend) StoreMasterKey(key []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	b.key = keyCopy
	return nil
}

func (b *testBackend) DeleteMasterKey() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.key = nil
	return nil
}

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

// memoryKeyring is an in-memory keyring for testing only.
// It is instance-based, ensuring each test instance has its own master key.
type memoryKeyring struct {
	mu  sync.RWMutex
	key []byte
}

// NewMemoryKeyring creates an in-memory keyring for use in tests.
func NewMemoryKeyring() (Keyring, error) {
	return &memoryKeyring{}, nil
}

func (m *memoryKeyring) Name() string {
	return "memory"
}

func (m *memoryKeyring) RetrieveMasterKey() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.key == nil {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	keyCopy := make([]byte, len(m.key))
	copy(keyCopy, m.key)
	return keyCopy, nil
}

func (m *memoryKeyring) StoreMasterKey(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	m.key = keyCopy
	return nil
}

func (m *memoryKeyring) DeleteMasterKey() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key = nil
	return nil
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystoretest

import (
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
)

// memoryKeyring is an in-memory keyring for testing only.
// It is instance-based, ensuring each test instance has its own master key.
type memoryKeyring struct {
	mu  sync.RWMutex
	key []byte
}

// NewMemoryKeyring creates an in-memory keyring for use in tests.
func NewMemoryKeyring() keystore.Keyring {
	return &memoryKeyring{}
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

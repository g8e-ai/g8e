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

package testutil

import (
	"encoding/json"
	"time"

	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
)

// MockReplayStore implements ReplayStore interface for testing.
// Simple version that never detects replays (returns false).
type MockReplayStore struct{}

func (m *MockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	return false, nil
}

// StatefulMockReplayStore implements ReplayStore with nonce tracking.
type StatefulMockReplayStore struct {
	Nonces map[string]bool
}

func NewStatefulMockReplayStore() *StatefulMockReplayStore {
	return &StatefulMockReplayStore{Nonces: make(map[string]bool)}
}

func (m *StatefulMockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	if m.Nonces[nonce] {
		return true, nil
	}
	m.Nonces[nonce] = true
	return false, nil
}

// MockStateRootProvider implements StateRootProvider interface for testing.
type MockStateRootProvider struct {
	Root string
}

func (m *MockStateRootProvider) GetCurrentStateRoot() (string, error) {
	return m.Root, nil
}

// NewMockStateRootProvider creates a mock with the given root value.
func NewMockStateRootProvider(root string) *MockStateRootProvider {
	return &MockStateRootProvider{Root: root}
}

// MockL3Notary implements L3Notary interface for testing.
// Simple version that always approves (returns true).
type MockL3Notary struct{}

func (m *MockL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return true, nil
}

// ConfigurableMockL3Notary implements L3Notary with configurable pass/fail behavior.
type ConfigurableMockL3Notary struct {
	ShouldPass bool
}

func (m *ConfigurableMockL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.ShouldPass, nil
}

// NewConfigurableMockL3Notary creates a mock with the given pass behavior.
func NewConfigurableMockL3Notary(shouldPass bool) *ConfigurableMockL3Notary {
	return &ConfigurableMockL3Notary{ShouldPass: shouldPass}
}

// SlowMockL3Notary implements L3Notary with artificial delay for testing race conditions.
type SlowMockL3Notary struct {
	Delay time.Duration
}

func (m *SlowMockL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	time.Sleep(m.Delay)
	return true, nil
}

// NewSlowMockL3Notary creates a mock with the given delay.
func NewSlowMockL3Notary(delay time.Duration) *SlowMockL3Notary {
	return &SlowMockL3Notary{Delay: delay}
}

// MockTransactionAudit implements TransactionAuditStore interface for testing.
// Simple version that is a no-op.
type MockTransactionAudit struct{}

func (m *MockTransactionAudit) DocSet(collection, id string, data json.RawMessage) error {
	return nil
}

// ConfigurableMockAuditStore implements TransactionAuditStore with configurable behavior and call tracking.
type ConfigurableMockAuditStore struct {
	DocSetFunc func(collection, id string, data json.RawMessage) error
	Calls      []struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}
}

func (m *ConfigurableMockAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	m.Calls = append(m.Calls, struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}{collection, id, data})
	if m.DocSetFunc != nil {
		return m.DocSetFunc(collection, id, data)
	}
	return nil
}

// NewConfigurableMockAuditStore creates a mock with the given docSetFunc.
func NewConfigurableMockAuditStore(docSetFunc func(collection, id string, data json.RawMessage) error) *ConfigurableMockAuditStore {
	return &ConfigurableMockAuditStore{DocSetFunc: docSetFunc}
}

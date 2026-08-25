// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

// MockReplayStore implements ReplayStore interface for testing.
// Simple version that never detects replays (returns false).
type MockReplayStore struct{}

func (m *MockReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	return false, nil
}

func (m *MockReplayStore) FinalizeNonce(nonce string) error {
	return nil
}

func (m *MockReplayStore) ReleaseNonce(nonce string) error {
	return nil
}

func (m *MockReplayStore) Close() error {
	return nil
}

// StatefulMockReplayStore implements ReplayStore with nonce tracking.
type StatefulMockReplayStore struct {
	mu     sync.RWMutex
	Nonces map[string]bool
}

func NewStatefulMockReplayStore() *StatefulMockReplayStore {
	return &StatefulMockReplayStore{Nonces: make(map[string]bool)}
}

func (m *StatefulMockReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Nonces[nonce] {
		return true, nil
	}
	m.Nonces[nonce] = true
	return false, nil
}

func (m *StatefulMockReplayStore) FinalizeNonce(nonce string) error {
	// No-op for mock - nonce is already marked as used
	return nil
}

func (m *StatefulMockReplayStore) ReleaseNonce(nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Nonces, nonce)
	return nil
}

func (m *StatefulMockReplayStore) Close() error {
	return nil
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

func (m *MockL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return true, nil
}

// VerifyPasskeyProof delegates to VerifyL3Proof so this mock also satisfies
// governance.PasskeyVerifier for tests that wire it as the passkey delegate of
// NewGatewayL3Notary.
func (m *MockL3Notary) VerifyPasskeyProof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
}

// L3Notary is now a unified interface in the governance package.
// These mocks are kept here for internal testutil usage but implement the governance.L3Notary interface.

// ConfigurableMockL3Notary implements L3Notary with configurable pass/fail behavior.
type ConfigurableMockL3Notary struct {
	ShouldPass bool
}

func (m *ConfigurableMockL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.ShouldPass, nil
}

// VerifyPasskeyProof delegates to VerifyL3Proof so this mock also satisfies
// governance.PasskeyVerifier for tests that wire it as the passkey delegate of
// NewGatewayL3Notary.
func (m *ConfigurableMockL3Notary) VerifyPasskeyProof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
}

// NewConfigurableMockL3Notary creates a mock with the given pass behavior.
func NewConfigurableMockL3Notary(shouldPass bool) *ConfigurableMockL3Notary {
	return &ConfigurableMockL3Notary{ShouldPass: shouldPass}
}

// SlowMockL3Notary implements L3Notary with artificial delay for testing race conditions.
type SlowMockL3Notary struct {
	Delay time.Duration
}

func (m *SlowMockL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	time.Sleep(m.Delay)
	return true, nil
}

// VerifyPasskeyProof delegates to VerifyL3Proof so this mock also satisfies
// governance.PasskeyVerifier.
func (m *SlowMockL3Notary) VerifyPasskeyProof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
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

func (m *MockTransactionAudit) DocDelete(collection, id string) error {
	return nil
}

// ConfigurableMockAuditStore implements TransactionAuditStore with configurable behavior and call tracking.
type ConfigurableMockAuditStore struct {
	DocSetFunc    func(collection, id string, data json.RawMessage) error
	DocDeleteFunc func(collection, id string) error
	DocSetCalls   []struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}
	DocDeleteCalls []struct {
		Collection string
		ID         string
	}
}

func (m *ConfigurableMockAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	m.DocSetCalls = append(m.DocSetCalls, struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}{collection, id, data})
	if m.DocSetFunc != nil {
		return m.DocSetFunc(collection, id, data)
	}
	return nil
}

func (m *ConfigurableMockAuditStore) DocDelete(collection, id string) error {
	m.DocDeleteCalls = append(m.DocDeleteCalls, struct {
		Collection string
		ID         string
	}{collection, id})
	if m.DocDeleteFunc != nil {
		return m.DocDeleteFunc(collection, id)
	}
	return nil
}

// NewConfigurableMockAuditStore creates a mock with the given docSetFunc.
func NewConfigurableMockAuditStore(docSetFunc func(collection, id string, data json.RawMessage) error) *ConfigurableMockAuditStore {
	return &ConfigurableMockAuditStore{DocSetFunc: docSetFunc}
}

// ConfigurableMockGovernedDocStore implements governance.GovernedDocumentStore
// with configurable behavior and call tracking. Used by unit tests that need
// to verify the handler dispatches to DocReplace vs DocMerge correctly.
type ConfigurableMockGovernedDocStore struct {
	DocReplaceFunc  func(collection, id string, data json.RawMessage) error
	DocMergeFunc    func(collection, id string, fields json.RawMessage) error
	DocDeleteFunc   func(collection, id string) error
	DocReplaceCalls []struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}
	DocMergeCalls []struct {
		Collection string
		ID         string
		Fields     json.RawMessage
	}
	DocDeleteCalls []struct {
		Collection string
		ID         string
	}
}

func (m *ConfigurableMockGovernedDocStore) DocReplace(collection, id string, data json.RawMessage) error {
	m.DocReplaceCalls = append(m.DocReplaceCalls, struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}{collection, id, data})
	if m.DocReplaceFunc != nil {
		return m.DocReplaceFunc(collection, id, data)
	}
	return nil
}

func (m *ConfigurableMockGovernedDocStore) DocMerge(collection, id string, fields json.RawMessage) error {
	m.DocMergeCalls = append(m.DocMergeCalls, struct {
		Collection string
		ID         string
		Fields     json.RawMessage
	}{collection, id, fields})
	if m.DocMergeFunc != nil {
		return m.DocMergeFunc(collection, id, fields)
	}
	return nil
}

func (m *ConfigurableMockGovernedDocStore) DocDelete(collection, id string) error {
	m.DocDeleteCalls = append(m.DocDeleteCalls, struct {
		Collection string
		ID         string
	}{collection, id})
	if m.DocDeleteFunc != nil {
		return m.DocDeleteFunc(collection, id)
	}
	return nil
}

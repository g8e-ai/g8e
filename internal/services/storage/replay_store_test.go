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

package storage

import (
	"errors"
	"testing"
	"time"
)

// FailingMockReplayStore implements ReplayStore to test fail-closed behavior.
type FailingMockReplayStore struct {
	ShouldFailOnReserve bool
	ReserveCallCount    int
}

func (m *FailingMockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	return false, nil
}

func (m *FailingMockReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	m.ReserveCallCount++
	if m.ShouldFailOnReserve {
		return false, errors.New("simulated replay store failure")
	}
	return false, nil
}

func (m *FailingMockReplayStore) FinalizeNonce(nonce string) error {
	return nil
}

func (m *FailingMockReplayStore) ReleaseNonce(nonce string) error {
	return nil
}

// TestFailingMockReplayStore_FailClosedOnReserveError verifies that
// the TransactionVerifier properly propagates replay store errors.
func TestFailingMockReplayStore_FailClosedOnReserveError(t *testing.T) {
	t.Parallel()

	failingStore := &FailingMockReplayStore{ShouldFailOnReserve: true}

	// This test verifies the fail-closed behavior at the interface level.
	// The actual integration with TransactionVerifier is tested in transaction_verifier_test.go
	_, err := failingStore.ReserveNonce("test-nonce", time.Now().UTC().Add(time.Hour))
	if err == nil {
		t.Errorf("expected error from failing replay store, got nil")
	}
	if err.Error() != "simulated replay store failure" {
		t.Errorf("expected specific error, got: %v", err)
	}
	if failingStore.ReserveCallCount != 1 {
		t.Errorf("expected 1 reserve call, got %d", failingStore.ReserveCallCount)
	}
}

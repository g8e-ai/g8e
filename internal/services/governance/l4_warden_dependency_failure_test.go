// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

var errReplayStoreDB = errors.New("database connection lost")

// errorReplayStore simulates a ReplayStore whose backing database is unavailable.
type errorReplayStore struct{}

func (m *errorReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	return false, errReplayStoreDB
}
func (m *errorReplayStore) FinalizeNonce(nonce string) error { return nil }
func (m *errorReplayStore) ReleaseNonce(nonce string) error  { return nil }
func (m *errorReplayStore) Close() error                     { return nil }

var errStateRootComputation = errors.New("state root computation failed")

// errorStateRootProvider simulates a StateRootProvider whose computation fails.
type errorStateRootProvider struct{}

func (m *errorStateRootProvider) GetCurrentStateRoot() (string, error) {
	return "", errStateRootComputation
}

// TestL4Warden_ReplayStoreError_FailClosed verifies that when the ReplayStore
// returns an error from ReserveNonce, the warden fails closed and rejects the
// transaction rather than silently accepting it.
func TestL4Warden_ReplayStoreError_FailClosed(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(
		t,
		&errorReplayStore{},
		testutil.NewMockStateRootProvider("root-1"),
		testutil.NewConfigurableMockL3Notary(true),
		"notary",
	)
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when ReplayStore fails, got nil")
	}
	if !errors.Is(err, errReplayStoreDB) {
		t.Fatalf("expected error wrapping errReplayStoreDB, got %v", err)
	}
}

// TestL4Warden_StateRootProviderError_FailClosed verifies that when the
// StateRootProvider returns an error from GetCurrentStateRoot, the warden
// fails closed and rejects the transaction.
func TestL4Warden_StateRootProviderError_FailClosed(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(
		t,
		testutil.NewStatefulMockReplayStore(),
		&errorStateRootProvider{},
		testutil.NewConfigurableMockL3Notary(true),
		"notary",
	)
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when StateRootProvider fails, got nil")
	}
	if !errors.Is(err, errStateRootComputation) {
		t.Fatalf("expected error wrapping errStateRootComputation, got %v", err)
	}
}

// TestL4Warden_PartialValidation_NonceNotLeaked verifies that when stateless
// validation passes but stateful validation fails, the nonce is released back
// to the replay store and can be reused in a subsequent attempt. This guards
// against nonce reservation leaks that would cause false replay rejections
// on retry.
func TestL4Warden_PartialValidation_NonceNotLeaked(t *testing.T) {
	t.Parallel()
	replayStore := testutil.NewStatefulMockReplayStore()

	// First attempt: state root mismatch causes stateful validation to fail
	// after stateless validation has already passed.
	verifier, privKey := createStrictVerifier(
		t,
		replayStore,
		testutil.NewMockStateRootProvider("wrong-root"),
		testutil.NewConfigurableMockL3Notary(true),
		"notary",
	)
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if !errors.Is(err, ErrStateRootMismatch) {
		t.Fatalf("expected ErrStateRootMismatch on first attempt, got %v", err)
	}

	// The nonce must have been released after the stateful failure.
	// Verify by attempting to reserve the same nonce — it should not be
	// flagged as a replay.
	replayed, reserveErr := replayStore.ReserveNonce(env.Nonce, time.Now().UTC().Add(time.Hour))
	if reserveErr != nil {
		t.Fatalf("failed to re-reserve nonce after release: %v", reserveErr)
	}
	if replayed {
		t.Fatal("nonce should not be marked as replayed after being released from a stateful validation failure")
	}
}

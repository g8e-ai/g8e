// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestWindowStore_SaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := NewWindowStore(fileSvc)
	require.NoError(t, err)

	window := testAttestationWindow(t, "attempt-1")
	require.NoError(t, store.Save(ctx, window))

	loaded, err := store.Load(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, window.GetProviderAttemptId(), loaded.GetProviderAttemptId())
	assert.Equal(t, window.GetAttestationDigest(), loaded.GetAttestationDigest())
	assert.Equal(t, window.GetObservedModelDigest(), loaded.GetObservedModelDigest())
}

func TestValidateAttestationWindow_RejectsDigestMismatch(t *testing.T) {
	t.Parallel()
	window := testAttestationWindow(t, "attempt-1")
	window.AttestationDigest = "deadbeef"
	err := ValidateAttestationWindow(window)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest mismatch")
}

func TestWindowStore_LoadMissingWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := NewWindowStore(fileSvc)
	require.NoError(t, err)

	_, err = store.Load(ctx, "missing-attempt")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestComputeAttestationDigest_RequiresProviderAttemptID(t *testing.T) {
	t.Parallel()
	_, err := ComputeAttestationDigest(&evalv1.ModelProvenanceAttestationWindow{
		ProvenanceOperatorId: "test-provenance-operator",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

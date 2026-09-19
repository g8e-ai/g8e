// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version.0.

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestInspectTrustState_TrustedWithNoStaleAnchors(t *testing.T) {
	installer := &mockTrustInstaller{
		isTrusted:    true,
		staleAnchors: []platform.StaleAnchor{},
	}

	state, err := InspectTrustState(context.Background(), installer, "abc123")
	require.NoError(t, err)
	assert.True(t, state.Trusted)
	assert.Empty(t, state.StaleAnchors)
}

func TestInspectTrustState_NotTrustedWithStaleAnchors(t *testing.T) {
	stale := []platform.StaleAnchor{
		{Fingerprint: "stale-fp-1", CommonName: "old-g8e-root", Handle: "/tmp/old.pem"},
		{Fingerprint: "stale-fp-2", CommonName: "older-g8e-root", Handle: "/tmp/older.pem"},
	}
	installer := &mockTrustInstaller{
		isTrusted:    false,
		staleAnchors: stale,
	}

	state, err := InspectTrustState(context.Background(), installer, "live-fp")
	require.NoError(t, err)
	assert.False(t, state.Trusted)
	assert.Len(t, state.StaleAnchors, 2)
	assert.Equal(t, "old-g8e-root", state.StaleAnchors[0].CommonName)
}

func TestInspectTrustState_IsTrustedUnsupportedReturnsEmptyState(t *testing.T) {
	installer := &mockTrustInstaller{
		isTrustedErr: constants.ErrSystemTrustUnsupported,
	}

	state, err := InspectTrustState(context.Background(), installer, "fp")
	require.NoError(t, err)
	assert.False(t, state.Trusted)
	assert.Nil(t, state.StaleAnchors)
}

func TestInspectTrustState_ListStaleAnchorsUnsupportedReturnsTrustedOnly(t *testing.T) {
	installer := &mockTrustInstaller{
		isTrusted: true,
		staleErr:  constants.ErrSystemTrustUnsupported,
	}

	state, err := InspectTrustState(context.Background(), installer, "fp")
	require.NoError(t, err)
	assert.True(t, state.Trusted)
	assert.Nil(t, state.StaleAnchors)
}

func TestInspectTrustState_IsTrustedErrorReturnsWrappedErrSystemTrustInstallFailed(t *testing.T) {
	installer := &mockTrustInstaller{
		isTrustedErr: errors.New("trust store read failure"),
	}

	_, err := InspectTrustState(context.Background(), installer, "fp")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInstallFailed)
}

func TestInspectTrustState_ListStaleAnchorsErrorReturnsWrappedErrSystemTrustInstallFailed(t *testing.T) {
	installer := &mockTrustInstaller{
		isTrusted: true,
		staleErr:  errors.New("trust store list failure"),
	}

	_, err := InspectTrustState(context.Background(), installer, "fp")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInstallFailed)
}

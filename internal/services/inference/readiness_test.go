// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nilStatusBackend returns a nil status without an error, exercising the
// fail-closed nil-status path that stubBackend's default prevents.
type nilStatusBackend struct{}

func (nilStatusBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	return nil, fmt.Errorf("nilStatusBackend: generate: %w", constants.ErrInferenceBackendUnavailable)
}

func (nilStatusBackend) Status(ctx context.Context) (*models.BackendStatus, error) {
	return nil, nil
}

func TestVerifyProviderReady_AvailableProviderPasses(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{Available: true}}

	err := VerifyProviderReady(context.Background(), backend)
	require.NoError(t, err, "the Operator configures no models, so readiness verifies only the provider")
}

func TestVerifyProviderReady_ProviderWithoutModelsPasses(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{Available: true, Models: []string{}}}

	err := VerifyProviderReady(context.Background(), backend)
	require.NoError(t, err, "a model the provider lacks fails the request that names it, not Operator startup")
}

func TestVerifyProviderReady_UnreachableProviderPropagatesUnavailable(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusErr: constants.ErrInferenceBackendUnavailable}

	err := VerifyProviderReady(context.Background(), backend)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_MalformedStatusPropagatesInvalid(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusErr: constants.ErrInferenceProviderResponseInvalid}

	err := VerifyProviderReady(context.Background(), backend)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestVerifyProviderReady_UnavailableStatusReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{Available: false}}

	err := VerifyProviderReady(context.Background(), backend)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_NilStatusReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()

	err := VerifyProviderReady(context.Background(), nilStatusBackend{})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_NilBackendFailsClosed(t *testing.T) {
	t.Parallel()

	err := VerifyProviderReady(context.Background(), nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendNotRegistered)
}

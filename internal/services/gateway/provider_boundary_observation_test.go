// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
)

type stubProviderBoundaryOperatorLister struct {
	operators []models.OperatorDocumentGo
	err       error
}

func (s *stubProviderBoundaryOperatorLister) ListOperatorsForObservation() ([]models.OperatorDocumentGo, error) {
	return s.operators, s.err
}

func TestProviderBoundaryObservationCoordinator_EnsureObserver_SubscribesToOwnerScopedObserver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	pubsubHandler := NewGatewayWebSocketHandler(logger)

	coordinator := NewProviderBoundaryObservationCoordinator(
		&DispatchService{},
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                "observer-1",
				OperatorSessionID: "sess-observer-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	assert.True(t, coordinator.ensureObserver(context.Background()))
	assert.NotNil(t, coordinator.observer)
	assert.Equal(t, "sess-observer-1", coordinator.observer.OperatorSessionID)
}

func TestProviderBoundaryObservationCoordinator_EnsureObserver_LogsNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)

	coordinator := NewProviderBoundaryObservationCoordinator(
		&DispatchService{},
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                "data-1",
				OperatorSessionID: "sess-data-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
			},
		}},
		NewGatewayWebSocketHandler(logger),
		windowStore,
		logger,
	)

	assert.False(t, coordinator.ensureObserver(context.Background()))
	assert.Nil(t, coordinator.observer)
}

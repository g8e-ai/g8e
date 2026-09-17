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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
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

func TestProviderBoundaryObservationCoordinator_IngestAfterDispatchContextCanceled(t *testing.T) {
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

	dispatchCtx, cancel := context.WithCancel(context.Background())
	require.True(t, coordinator.ensureObserver(dispatchCtx))
	cancel()

	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-async-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     time.Unix(1_700_000_000, 0).UnixMilli(),
		AttemptCompletedAtUnixMs:   time.Unix(1_700_000_010, 0).UnixMilli(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			HostRamAvailability:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			HostRamUsedBytes:           1,
			HostRamTotalBytes:          2,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest

	completion := &evalv1.ProviderBoundaryObservationCompleted{Window: window}
	payload, err := proto.Marshal(completion)
	require.NoError(t, err)
	env := &commonv1.GovernanceEnvelope{
		EventType: string(constants.Event.Operator.ProviderBoundaryObservation.Completed),
		Payload:   payload,
	}
	wire, err := protojson.Marshal(env)
	require.NoError(t, err)

	resultsChannel := pubsub.ResultsChannel("observer-1", "sess-observer-1")
	pubsubHandler.Publish(resultsChannel, wire)

	loaded, err := windowStore.Load(context.Background(), "attempt-async-1")
	require.NoError(t, err)
	assert.Equal(t, "attempt-async-1", loaded.GetProviderAttemptId())
}

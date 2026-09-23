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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testModelProvenanceWindow(t *testing.T, providerAttemptID string) *evalv1.ModelProvenanceAttestationWindow {
	t.Helper()
	digest := strings.Repeat("a", 64)
	window := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              model_provenance.SchemaVersion,
		ProviderAttemptId:          providerAttemptID,
		ProvenanceOperatorId:       "prov-1",
		ServedModelTag:             "probe-model:7b",
		ExpectedModelDigest:        digest,
		ObservedModelDigest:        digest,
		ManifestDigest:             strings.Repeat("b", 64),
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		AttestedAtUnixMs:           1_700_000_000_000,
		DigestMatch:                true,
	}
	attestationDigest, err := model_provenance.ComputeAttestationDigest(window)
	require.NoError(t, err)
	window.AttestationDigest = attestationDigest
	return window
}

func TestModelProvenanceObservationCoordinator_EnsureOperator_SubscribesToProvenanceOperator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	pubsubHandler := NewGatewayWebSocketHandler(logger)

	coordinator := NewModelProvenanceObservationCoordinator(
		&DispatchService{},
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	assert.NoError(t, coordinator.ensureOperator(context.Background()))
	assert.NotNil(t, coordinator.operator)
	assert.Equal(t, "sess-prov-1", coordinator.operator.OperatorSessionID)
}

func TestModelProvenanceObservationCoordinator_PreflightCommandDelivery_FailsWithoutCmdSubscriber(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	pubsubHandler := NewGatewayWebSocketHandler(logger)

	coordinator := NewModelProvenanceObservationCoordinator(
		&DispatchService{pubsub: pubsubHandler},
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	err = coordinator.PreflightCommandDelivery(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
}

func TestModelProvenanceObservationCoordinator_IngestPersistsAttestationWindow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	pubsubHandler := NewGatewayWebSocketHandler(logger)

	coordinator := NewModelProvenanceObservationCoordinator(
		&DispatchService{},
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	require.NoError(t, coordinator.ensureOperator(context.Background()))
	window := testModelProvenanceWindow(t, "attempt-async-1")
	completion := &evalv1.ModelProvenanceObservationCompleted{Window: window}
	payload, err := proto.Marshal(completion)
	require.NoError(t, err)
	env := &commonv1.GovernanceEnvelope{
		EventType: string(constants.Event.Operator.ModelProvenanceObservation.Completed),
		Payload:   payload,
	}
	wire, err := protojson.Marshal(env)
	require.NoError(t, err)

	pubsubHandler.Publish(pubsub.ResultsChannel("prov-1", "sess-prov-1"), wire)

	loaded, err := windowStore.Load(context.Background(), "attempt-async-1")
	require.NoError(t, err)
	assert.Equal(t, "attempt-async-1", loaded.GetProviderAttemptId())
}

func TestModelProvenanceObservationCoordinator_NotifyAttemptBegin_FailsWithoutCmdSubscriber(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	op := &models.OperatorDocumentGo{
		ID:                "prov-1",
		OperatorSessionID: "sess-prov-1",
	}
	dispatchSvc, pubsubHandler := newTestDispatchService(t, "root-abc", op)

	coordinator := NewModelProvenanceObservationCoordinator(
		dispatchSvc,
		&stubProviderBoundaryOperatorLister{operators: []models.OperatorDocumentGo{
			{
				ID:                op.ID,
				OperatorSessionID: op.OperatorSessionID,
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	err = coordinator.NotifyAttemptBegin(
		context.Background(),
		"user-1",
		"attempt-1",
		"probe-model:7b",
		strings.Repeat("a", 64),
		strings.Repeat("c", 64),
		"campaign-1",
		time.Now().UnixMilli(),
		0,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDispatchNoDelivery)
}

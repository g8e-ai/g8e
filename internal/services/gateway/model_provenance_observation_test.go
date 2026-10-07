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
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
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

type stubModelProvenanceOperatorLister struct {
	operators []*operatorv1.OperatorDocument
	err       error
}

func (s *stubModelProvenanceOperatorLister) ListOperatorsForProvenance() ([]*operatorv1.OperatorDocument, error) {
	return s.operators, s.err
}

func TestModelProvenanceObservationCoordinator_EnsureOperator_SubscribesToProvenanceOperator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	pubsubHandler := NewGatewayWebSocketHandler(logger)

	coordinator := NewModelProvenanceObservationCoordinator(
		&DispatchService{},
		&stubModelProvenanceOperatorLister{operators: []*operatorv1.OperatorDocument{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	assert.NoError(t, coordinator.synchronizeOperatorSubscription(context.Background()))
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
		&stubModelProvenanceOperatorLister{operators: []*operatorv1.OperatorDocument{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
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
		&stubModelProvenanceOperatorLister{operators: []*operatorv1.OperatorDocument{
			{
				ID:                "prov-1",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
			},
		}},
		pubsubHandler,
		windowStore,
		logger,
	)

	require.NoError(t, coordinator.synchronizeOperatorSubscription(context.Background()))
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
	op := &operatorv1.OperatorDocument{
		ID:                "prov-1",
		OperatorSessionID: "sess-prov-1",
	}
	dispatchSvc, pubsubHandler := newTestDispatchService(t, "root-abc", op)

	coordinator := NewModelProvenanceObservationCoordinator(
		dispatchSvc,
		&stubModelProvenanceOperatorLister{operators: []*operatorv1.OperatorDocument{
			{
				ID:                op.ID,
				OperatorSessionID: op.OperatorSessionID,
				Status:            constants.OperatorStatusActive,
				OperatorType:      constants.OperatorTypeRemote,
				RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
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

// A preflight must be driven by completion delivery, never by store reads.
type eventOnlyProvenanceStore struct{ model_provenance.WindowStore }

func (eventOnlyProvenanceStore) Load(context.Context, string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	panic("preflight attempted to poll provenance storage")
}

func TestModelProvenanceObservationCoordinator_PreflightStorageAttestation(t *testing.T) {
	for _, scenario := range []string{"begin failure", "finalize failure", "delayed finalize failure", "success with unrelated failure", "cancellation", "silent operator"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
				store, err := model_provenance.NewWindowStore(storagetest.NewTestFileSvc(t, t.TempDir()))
				require.NoError(t, err)
				op := operatorv1.OperatorDocument{ID: "prov-1", OperatorSessionID: "sess-prov-1", Status: constants.OperatorStatusActive,
					OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}}
				dispatch, broker := newTestDispatchService(t, "root-abc", &op)
				coordinator := NewModelProvenanceObservationCoordinator(dispatch, &stubModelProvenanceOperatorLister{operators: []*operatorv1.OperatorDocument{op}}, broker, eventOnlyProvenanceStore{store}, logger)
				defer coordinator.Stop()
				cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
				subscriber := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
				broker.subscribe(cmdChannel, subscriber)
				defer broker.unsubscribe(cmdChannel, subscriber)
				requestTimeout := 2 * time.Second
				if scenario == "silent operator" {
					requestTimeout = 12 * time.Second
				}
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()
				commands := 0
				unregister := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, data []byte) {
					commands++
					env := &commonv1.GovernanceEnvelope{}
					require.NoError(t, protojson.Unmarshal(data, env))
					command := &evalv1.ModelProvenanceObservationCommand{}
					require.NoError(t, proto.Unmarshal(env.Payload, command))
					finalize := command.Phase == evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE
					if scenario == "begin failure" || ((scenario == "finalize failure" || scenario == "delayed finalize failure") && finalize) || scenario == "success with unrelated failure" {
						txID := env.Id
						if scenario == "success with unrelated failure" {
							txID = "unrelated-transaction"
						}
						payload, err := proto.Marshal(&operatorv1.ActionReceipt{TransactionId: txID, Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, ResultSummary: "manifest missing"})
						require.NoError(t, err)
						wire, err := protojson.Marshal(&commonv1.GovernanceEnvelope{EventType: string(constants.Event.Operator.Receipt.Recorded), OperatorId: op.ID, OperatorSessionId: op.OperatorSessionID, Payload: payload})
						require.NoError(t, err)
						// Simulate broker fan-out after receipt signature verification.
						if scenario == "delayed finalize failure" {
							go func() {
								select {
								case <-ctx.Done():
								case <-time.After(10 * time.Millisecond):
									broker.Publish(pubsub.ReceiptsChannel(op.ID, op.OperatorSessionID), wire)
								}
							}()
						} else {
							broker.Publish(pubsub.ReceiptsChannel(op.ID, op.OperatorSessionID), wire)
						}
					}
					if !finalize && scenario != "begin failure" && scenario != "silent operator" {
						payload, err := proto.Marshal(&operatorv1.ActionReceipt{TransactionId: env.Id, Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED})
						require.NoError(t, err)
						wire, err := protojson.Marshal(&commonv1.GovernanceEnvelope{EventType: string(constants.Event.Operator.Receipt.Recorded), OperatorId: op.ID, OperatorSessionId: op.OperatorSessionID, Payload: payload})
						require.NoError(t, err)
						broker.Publish(pubsub.ReceiptsChannel(op.ID, op.OperatorSessionID), wire)
					}
					if finalize && scenario == "success with unrelated failure" {
						payload, err := proto.Marshal(&evalv1.ModelProvenanceObservationCompleted{Window: testModelProvenanceWindow(t, command.ProviderAttemptId)})
						require.NoError(t, err)
						wire, err := protojson.Marshal(&commonv1.GovernanceEnvelope{EventType: string(constants.Event.Operator.ModelProvenanceObservation.Completed), Payload: payload})
						require.NoError(t, err)
						broker.Publish(pubsub.ResultsChannel(op.ID, op.OperatorSessionID), wire)
					}
					if finalize && scenario == "cancellation" {
						cancel()
					}
				})
				defer unregister()
				window, err := coordinator.PreflightStorageAttestation(ctx, "probe-model:7b", strings.Repeat("a", 64))
				switch scenario {
				case "success with unrelated failure":
					require.NoError(t, err)
					require.NotNil(t, window)
				case "cancellation":
					require.ErrorIs(t, err, context.Canceled)
				case "silent operator":
					require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
					require.ErrorContains(t, err, "did not acknowledge BEGIN")
					require.NoError(t, ctx.Err(), "acknowledgement deadline must precede the full probe deadline")
				default:
					require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
					require.ErrorContains(t, err, "manifest missing")
					require.NoError(t, ctx.Err(), "failure must arrive before request timeout")
				}
				if scenario == "begin failure" || scenario == "silent operator" {
					require.Equal(t, 1, commands)
				}
				if scenario != "begin failure" && scenario != "silent operator" {
					require.Equal(t, 2, commands)
				}
				broker.handlersMu.RLock()
				remaining := len(broker.handlers[pubsub.ReceiptsChannel(op.ID, op.OperatorSessionID)])
				broker.handlersMu.RUnlock()
				require.Zero(t, remaining, "preflight receipt subscription must be cleaned up")
			})
		})
	}
}

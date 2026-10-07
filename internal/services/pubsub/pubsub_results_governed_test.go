// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func newResultsTestService(t *testing.T) (*PubSubResultsService, *pubsubtest.MockOperatorPubSubClient, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	client := pubsubtest.NewMockOperatorPubSubClient()
	svc, err := NewPubSubResultsService(cfg, testutil.NewTestLogger(), client)
	require.NoError(t, err)
	return svc, client, cfg
}

// requireSinglePublished asserts exactly one result was published and returns
// its channel and decoded envelope.
func requireSinglePublished(t *testing.T, client *pubsubtest.MockOperatorPubSubClient) (string, *commonv1.GovernanceEnvelope) {
	t.Helper()
	require.Equal(t, 1, client.PublishedCount(), "exactly one result must be published")
	last := client.LastPublished()
	require.NotNil(t, last)
	return last.Channel, mustUnmarshalGovernanceEnvelope(t, last.Data)
}

func TestPubSubResultsService_PublishShutdownAcknowledgement_CorrelatesWithTheShutdownCommand(t *testing.T) {
	svc, client, cfg := newResultsTestService(t)
	request := &operatorv1.ShutdownRequested{Reason: "maintenance window"}
	original := &PubSubCommandMessage{
		ID:                "shutdown-msg",
		EventType:         constants.Event.Operator.ShutdownRequested,
		CaseID:            "case-1",
		OperatorSessionID: "sess-shutdown",
	}

	require.NoError(t, svc.PublishShutdownAcknowledgement(t.Context(), request, original))

	channel, env := requireSinglePublished(t, client)
	assert.Equal(t, ResultsChannel(cfg.OperatorID, "sess-shutdown"), channel)
	assert.Equal(t, string(constants.Event.Operator.ShutdownAcknowledged), env.EventType)
	assert.Equal(t, "shutdown-msg", env.Id)
	assert.Equal(t, "case-1", env.CaseId)
	var echoed operatorv1.ShutdownRequested
	require.NoError(t, proto.Unmarshal(env.Payload, &echoed))
	assert.Equal(t, "maintenance window", echoed.Reason)
}

func TestPubSubResultsService_PublishShutdownAcknowledgement_FailsWhenCommandIsNotAGovernedRequest(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	original := &PubSubCommandMessage{ID: "m", EventType: constants.Event.Operator.Command.Completed}

	err := svc.PublishShutdownAcknowledgement(t.Context(), &operatorv1.ShutdownRequested{}, original)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish shutdown acknowledgement")
	assert.Zero(t, client.PublishedCount(), "nothing may be published without a resolvable governed action")
}

func inferenceCommand(operatorID *string) *PubSubCommandMessage {
	taskID := "task-9"
	return &PubSubCommandMessage{
		ID:                "infer-msg",
		EventType:         constants.Event.Operator.Inference.Requested,
		CaseID:            "case-7",
		InvestigationID:   "inv-7",
		TaskID:            &taskID,
		WebSessionID:      "web-7",
		CLISessionID:      "cli-7",
		OperatorSessionID: "sess-infer",
		OperatorID:        operatorID,
	}
}

func TestPubSubResultsService_PublishInferenceProgress_RoutesByCommandSessionAndCarriesCorrelation(t *testing.T) {
	override := "op-override"
	tests := []struct {
		name         string
		operatorID   *string
		wantOperator func(cfg *config.Config) string
	}{
		{"defaults to the service operator id", nil, func(cfg *config.Config) string { return cfg.OperatorID }},
		{"command operator id overrides the service config", &override, func(*config.Config) string { return override }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client, cfg := newResultsTestService(t)
			progress := &operatorv1.InferenceProgressEvent{ProviderAttemptId: "attempt-1", Sequence: 3, ServedModel: "qwen3:4b"}

			require.NoError(t, svc.PublishInferenceProgress(t.Context(), inferenceCommand(tt.operatorID), progress))

			channel, env := requireSinglePublished(t, client)
			assert.Equal(t, ResultsChannel(tt.wantOperator(cfg), "sess-infer"), channel)
			assert.Equal(t, string(constants.Event.Operator.Inference.ProgressUpdated), env.EventType)
			assert.Equal(t, "infer-msg", env.Id)
			assert.Equal(t, "sess-infer", env.OperatorSessionId, "the envelope must be addressed to the command's session")
			assert.Equal(t, "case-7", env.CaseId)
			assert.Equal(t, "inv-7", env.InvestigationId)
			assert.Equal(t, "task-9", env.TaskId)
			assert.Equal(t, "web-7", env.WebSessionId)
			assert.Equal(t, "cli-7", env.CliSessionId)
			var decoded operatorv1.InferenceProgressEvent
			require.NoError(t, proto.Unmarshal(env.Payload, &decoded))
			assert.Equal(t, "attempt-1", decoded.ProviderAttemptId)
			assert.Equal(t, uint32(3), decoded.Sequence)
		})
	}
}

func TestPubSubResultsService_PublishInferenceProgress_RejectsInvalidInput(t *testing.T) {
	progress := &operatorv1.InferenceProgressEvent{ProviderAttemptId: "a"}

	tests := []struct {
		name     string
		original *PubSubCommandMessage
		progress *operatorv1.InferenceProgressEvent
		wantErr  error
	}{
		{"nil command", nil, progress, constants.ErrMissingRequiredField},
		{"nil progress", inferenceCommand(nil), nil, constants.ErrMissingRequiredField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client, _ := newResultsTestService(t)

			err := svc.PublishInferenceProgress(t.Context(), tt.original, tt.progress)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Zero(t, client.PublishedCount())
		})
	}

	t.Run("command that is not a governed request has no originating action", func(t *testing.T) {
		svc, client, _ := newResultsTestService(t)
		original := inferenceCommand(nil)
		original.EventType = constants.Event.Operator.Command.Completed

		err := svc.PublishInferenceProgress(t.Context(), original, progress)

		require.Error(t, err)
		assert.Zero(t, client.PublishedCount())
	})
}

func TestPubSubResultsService_PublishInferenceProgress_SurfacesTransportFailure(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	client.SetPublishError(true)

	err := svc.PublishInferenceProgress(t.Context(), inferenceCommand(nil), &operatorv1.InferenceProgressEvent{ProviderAttemptId: "a"})

	require.ErrorIs(t, err, constants.ErrClientClosed)
	assert.Contains(t, err.Error(), "publish inference progress")
}

func inferenceCommandEnvelope() *commonv1.GovernanceEnvelope {
	return &commonv1.GovernanceEnvelope{
		Id:                "infer-env",
		EventType:         string(constants.Event.Operator.Inference.Requested),
		ActionType:        string(constants.ActionTypeInference),
		OperatorId:        "op-77",
		OperatorSessionId: "sess-77",
		CaseId:            "case-77",
		InvestigationId:   "inv-77",
		TaskId:            "task-77",
		WebSessionId:      "web-77",
		CliSessionId:      "cli-77",
	}
}

func TestPubSubResultsService_PublishInferenceCompletion_MapsReceiptStatusToTerminalEvent(t *testing.T) {
	tests := []struct {
		name      string
		status    operatorv1.ExecutionStatus
		wantEvent constants.EventType
	}{
		{"completed receipt publishes inference.completed", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, constants.Event.Operator.Inference.Completed},
		{"failed receipt publishes inference.failed", operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, constants.Event.Operator.Inference.Failed},
		{"timed-out receipt publishes inference.failed", operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT, constants.Event.Operator.Inference.Failed},
		{"cancelled receipt publishes inference.failed", operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED, constants.Event.Operator.Inference.Failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client, _ := newResultsTestService(t)
			completion := &operatorv1.InferenceCompletion{
				Receipt: &operatorv1.ActionReceipt{TransactionId: "txn-1", Status: tt.status},
			}

			require.NoError(t, svc.PublishInferenceCompletion(t.Context(), inferenceCommandEnvelope(), completion))

			channel, env := requireSinglePublished(t, client)
			assert.Equal(t, ResultsChannel("op-77", "sess-77"), channel,
				"the completion must return on the channel the waiting dispatcher subscribed to")
			assert.Equal(t, string(tt.wantEvent), env.EventType)
			assert.Equal(t, "infer-env", env.Id, "the completion is correlated by the command envelope id")
			assert.Equal(t, "sess-77", env.OperatorSessionId)
			assert.Equal(t, "case-77", env.CaseId)
			assert.Equal(t, "task-77", env.TaskId)
			var decoded operatorv1.InferenceCompletion
			require.NoError(t, proto.Unmarshal(env.Payload, &decoded))
			assert.Equal(t, "txn-1", decoded.Receipt.TransactionId)
			assert.Equal(t, tt.status, decoded.Receipt.Status)
		})
	}
}

func TestPubSubResultsService_PublishInferenceCompletion_RejectsIncompleteInput(t *testing.T) {
	completed := &operatorv1.InferenceCompletion{Receipt: &operatorv1.ActionReceipt{Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}}

	tests := []struct {
		name       string
		env        *commonv1.GovernanceEnvelope
		completion *operatorv1.InferenceCompletion
	}{
		{"nil envelope", nil, completed},
		{"nil completion", inferenceCommandEnvelope(), nil},
		{"completion without a signed receipt", inferenceCommandEnvelope(), &operatorv1.InferenceCompletion{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client, _ := newResultsTestService(t)

			err := svc.PublishInferenceCompletion(t.Context(), tt.env, tt.completion)

			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
			assert.Zero(t, client.PublishedCount())
		})
	}
}

func TestPubSubResultsService_PublishInferenceCompletion_RejectsEnvelopeWhoseActionDoesNotMatchItsEvent(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	env := inferenceCommandEnvelope()
	env.ActionType = string(constants.ActionTypeExecuteBash)
	completion := &operatorv1.InferenceCompletion{Receipt: &operatorv1.ActionReceipt{Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}}

	err := svc.PublishInferenceCompletion(t.Context(), env, completion)

	require.ErrorIs(t, err, constants.ErrTxEventActionMismatch)
	assert.Zero(t, client.PublishedCount(), "a mislabelled completion must never reach the wire")
}

func TestPubSubResultsService_PublishInferenceCompletion_SurfacesTransportFailure(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	client.SetPublishError(true)
	completion := &operatorv1.InferenceCompletion{Receipt: &operatorv1.ActionReceipt{Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}}

	err := svc.PublishInferenceCompletion(t.Context(), inferenceCommandEnvelope(), completion)

	require.ErrorIs(t, err, constants.ErrClientClosed)
	assert.Contains(t, err.Error(), "publish inference completion")
}

func TestPubSubResultsService_PublishProviderBoundaryObservationCompleted_PublishesWindowOnObserverChannel(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	completion := &evalv1.ProviderBoundaryObservationCompleted{
		Window: &evalv1.ProviderBoundaryObservationWindow{ProviderAttemptId: "attempt-pb", ObserverId: "observer-1"},
	}

	require.NoError(t, svc.PublishProviderBoundaryObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "pb-cmd-1", OperatorId: "op-1", OperatorSessionId: "sess-1"}, completion))

	channel, env := requireSinglePublished(t, client)
	assert.Equal(t, ResultsChannel("op-1", "sess-1"), channel)
	assert.Equal(t, string(constants.Event.Operator.ProviderBoundaryObservation.Completed), env.EventType)
	assert.Equal(t, "pb-cmd-1", env.Id, "the completion is correlated with the command by transaction id")
	var decoded evalv1.ProviderBoundaryObservationCompleted
	require.NoError(t, proto.Unmarshal(env.Payload, &decoded))
	assert.Equal(t, "attempt-pb", decoded.GetWindow().GetProviderAttemptId())
	assert.Equal(t, "observer-1", decoded.GetWindow().GetObserverId())
}

func TestPubSubResultsService_PublishModelProvenanceObservationCompleted_PublishesWindowOnProvenanceChannel(t *testing.T) {
	svc, client, _ := newResultsTestService(t)
	completion := &evalv1.ModelProvenanceObservationCompleted{
		Window: &evalv1.ModelProvenanceAttestationWindow{ProviderAttemptId: "attempt-mp", ServedModelTag: "qwen3:4b"},
	}

	require.NoError(t, svc.PublishModelProvenanceObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "mp-cmd-1", OperatorId: "op-1", OperatorSessionId: "sess-1"}, completion))

	channel, env := requireSinglePublished(t, client)
	assert.Equal(t, ResultsChannel("op-1", "sess-1"), channel)
	assert.Equal(t, string(constants.Event.Operator.ModelProvenanceObservation.Completed), env.EventType)
	assert.Equal(t, "mp-cmd-1", env.Id)
	var decoded evalv1.ModelProvenanceObservationCompleted
	require.NoError(t, proto.Unmarshal(env.Payload, &decoded))
	assert.Equal(t, "attempt-mp", decoded.GetWindow().GetProviderAttemptId())
	assert.Equal(t, "qwen3:4b", decoded.GetWindow().GetServedModelTag())
}

func TestPubSubResultsService_ObservationCompletions_RejectMissingCorrelationOrWindow(t *testing.T) {
	boundaryWindow := &evalv1.ProviderBoundaryObservationWindow{ProviderAttemptId: "a"}
	provenanceWindow := &evalv1.ModelProvenanceAttestationWindow{ProviderAttemptId: "a"}

	tests := []struct {
		name string
		call func(svc *PubSubResultsService, t *testing.T) error
	}{
		{"boundary: empty command id", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishProviderBoundaryObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "", OperatorId: "op-1", OperatorSessionId: "sess-1"}, &evalv1.ProviderBoundaryObservationCompleted{Window: boundaryWindow})
		}},
		{"boundary: nil completion", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishProviderBoundaryObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"}, nil)
		}},
		{"boundary: completion without a window", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishProviderBoundaryObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"}, &evalv1.ProviderBoundaryObservationCompleted{})
		}},
		{"provenance: empty command id", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishModelProvenanceObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "", OperatorId: "op-1", OperatorSessionId: "sess-1"}, &evalv1.ModelProvenanceObservationCompleted{Window: provenanceWindow})
		}},
		{"provenance: nil completion", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishModelProvenanceObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"}, nil)
		}},
		{"provenance: completion without a window", func(s *PubSubResultsService, t *testing.T) error {
			return s.PublishModelProvenanceObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"}, &evalv1.ModelProvenanceObservationCompleted{})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client, _ := newResultsTestService(t)

			err := tt.call(svc, t)

			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
			assert.Zero(t, client.PublishedCount())
		})
	}
}

func TestPubSubResultsService_ObservationCompletions_SurfaceTransportFailure(t *testing.T) {
	t.Run("provider boundary", func(t *testing.T) {
		svc, client, _ := newResultsTestService(t)
		client.SetPublishError(true)

		err := svc.PublishProviderBoundaryObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"},
			&evalv1.ProviderBoundaryObservationCompleted{Window: &evalv1.ProviderBoundaryObservationWindow{ProviderAttemptId: "a"}})

		require.ErrorIs(t, err, constants.ErrClientClosed)
		assert.Contains(t, err.Error(), "provider boundary observation completion")
	})

	t.Run("model provenance", func(t *testing.T) {
		svc, client, _ := newResultsTestService(t)
		client.SetPublishError(true)

		err := svc.PublishModelProvenanceObservationCompleted(t.Context(), &commonv1.GovernanceEnvelope{Id: "cmd", OperatorId: "op-1", OperatorSessionId: "sess-1"},
			&evalv1.ModelProvenanceObservationCompleted{Window: &evalv1.ModelProvenanceAttestationWindow{ProviderAttemptId: "a"}})

		require.ErrorIs(t, err, constants.ErrClientClosed)
		assert.Contains(t, err.Error(), "model provenance observation completion")
	})
}

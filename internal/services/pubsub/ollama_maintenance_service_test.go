// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	ollamaTestSessionID = "sess-ollama"
	ollamaTestMessageID = "msg-ollama-1"
)

type ollamaMaintenanceEnv struct {
	svc    *OllamaMaintenanceService
	client *pubsubtest.MockOperatorPubSubClient
	cfg    *config.Config
	audit  *mockAuditStore
}

// newOllamaMaintenanceEnv wires the service to an in-memory pub/sub client and
// the given provider endpoint. An empty endpoint exercises the invalid-endpoint
// path.
func newOllamaMaintenanceEnv(t *testing.T, endpoint string) *ollamaMaintenanceEnv {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.Inference.OllamaEndpoint = endpoint
	client := pubsubtest.NewMockOperatorPubSubClient()
	svc := NewOllamaMaintenanceService(cfg, testutil.NewTestLogger(), client)
	audit := &mockAuditStore{}
	svc.SetAuditStore(audit)
	return &ollamaMaintenanceEnv{svc: svc, client: client, cfg: cfg, audit: audit}
}

// fakeOllama serves the given provider API paths over loopback HTTP.
func fakeOllama(t *testing.T, routes map[string]http.HandlerFunc) string {
	t.Helper()
	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, handler)
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}

func ollamaJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func ollamaStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func ollamaWithPayload(msg *PubSubCommandMessage, payload []byte) *PubSubCommandMessage {
	msg.Payload = payload
	return msg
}

func ollamaInventoryMessage(t *testing.T, executionID string) *PubSubCommandMessage {
	t.Helper()
	return &PubSubCommandMessage{
		ID:                ollamaTestMessageID,
		EventType:         constants.Event.Operator.OllamaModelInventory.Requested,
		OperatorSessionID: ollamaTestSessionID,
		Payload:           mustMarshalProto(t, &operatorv1.OllamaModelInventoryRequested{ExecutionId: executionID}),
	}
}

func ollamaResidencyMessage(t *testing.T, executionID string) *PubSubCommandMessage {
	t.Helper()
	return &PubSubCommandMessage{
		ID:                ollamaTestMessageID,
		EventType:         constants.Event.Operator.OllamaModelResidency.Requested,
		OperatorSessionID: ollamaTestSessionID,
		Payload:           mustMarshalProto(t, &operatorv1.OllamaModelResidencyRequested{ExecutionId: executionID}),
	}
}

// publishedResult decodes the single published result envelope and its typed
// payload, and verifies it was routed to the originating session's results
// channel.
func (e *ollamaMaintenanceEnv) publishedResult(t *testing.T, into proto.Message) *commonv1.GovernanceEnvelope {
	t.Helper()
	require.Equal(t, 1, e.client.PublishedCount(), "exactly one result must be published")
	last := e.client.LastPublished()
	require.NotNil(t, last)
	assert.Equal(t, ResultsChannel(e.cfg.OperatorID, ollamaTestSessionID), last.Channel)
	env := mustUnmarshalGovernanceEnvelope(t, last.Data)
	require.NoError(t, proto.Unmarshal(env.Payload, into))
	return env
}

const (
	ollamaTestDigestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ollamaTestDigestB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func ollamaInventoryRoutes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/tags": ollamaJSON(`{"models":[` +
			`{"name":"alias-one:latest","digest":"sha256:` + ollamaTestDigestA + `"},` +
			`{"name":"alias-two:latest","digest":"sha256:` + ollamaTestDigestB + `"}]}`),
		"/api/show": ollamaJSON(`{"parameters":"num_ctx 8192\n",` +
			`"details":{"format":"gguf","family":"qwen3","parameter_size":"4.0B","quantization_level":"Q4_K_M"},` +
			`"capabilities":["completion","tools"]}`),
	}
}

func TestOllamaMaintenanceService_HandleInventoryRequest_PublishesTypedInventory(t *testing.T) {
	env := newOllamaMaintenanceEnv(t, fakeOllama(t, ollamaInventoryRoutes()))

	env.svc.HandleInventoryRequest(t.Context(), ollamaInventoryMessage(t, "exec-42"))

	var result operatorv1.OllamaModelInventoryResult
	envelope := env.publishedResult(t, &result)
	assert.Equal(t, string(constants.Event.Operator.OllamaModelInventory.Completed), envelope.EventType)
	assert.Equal(t, ollamaTestMessageID, envelope.Id, "the result must be correlated with the originating command")
	assert.Equal(t, "exec-42", result.ExecutionId)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
	require.Len(t, result.Entries, 2)
	assert.Equal(t, "alias-one:latest", result.Entries[0].ServedModelTag)
	assert.Equal(t, ollamaTestDigestA, result.Entries[0].ModelDigest)
	assert.Equal(t, "qwen3", result.Entries[0].ModelFamily)
	assert.Equal(t, "Q4_K_M", result.Entries[0].Quantization)
	assert.Equal(t, uint64(4_000_000_000), result.Entries[0].ParameterCount)
	assert.Equal(t, uint32(8192), result.Entries[0].ContextLimit)
	assert.Equal(t, []string{"completion", "tools"}, result.Entries[0].AdvertisedCapabilities)
	assert.Equal(t, "alias-two:latest", result.Entries[1].ServedModelTag, "distinct served tags must stay distinct entries")
	assert.Equal(t, ollamaTestDigestB, result.Entries[1].ModelDigest)
}

func TestOllamaMaintenanceService_HandleInventoryRequest_ExecutionIDFallsBackToMessageID(t *testing.T) {
	tests := []struct {
		name        string
		executionID string
		want        string
	}{
		{"explicit execution id is preserved", "exec-explicit", "exec-explicit"},
		{"missing execution id uses the envelope id", "", ollamaTestMessageID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newOllamaMaintenanceEnv(t, fakeOllama(t, ollamaInventoryRoutes()))

			env.svc.HandleInventoryRequest(t.Context(), ollamaInventoryMessage(t, tt.executionID))

			var result operatorv1.OllamaModelInventoryResult
			env.publishedResult(t, &result)
			assert.Equal(t, tt.want, result.ExecutionId)
		})
	}
}

func TestOllamaMaintenanceService_HandleInventoryRequest_PublishesTypedFailure(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  func(t *testing.T) string
		message   func(t *testing.T) *PubSubCommandMessage
		wantError string
	}{
		{
			name:     "undecodable request payload",
			endpoint: func(t *testing.T) string { return fakeOllama(t, ollamaInventoryRoutes()) },
			message: func(t *testing.T) *PubSubCommandMessage {
				return ollamaWithPayload(ollamaInventoryMessage(t, ""), []byte{0xff, 0xff})
			},
			wantError: "invalid request payload",
		},
		{
			name:      "provider endpoint is not configured",
			endpoint:  func(*testing.T) string { return "" },
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaInventoryMessage(t, "") },
			wantError: constants.ErrInferenceEndpointInvalid.Error(),
		},
		{
			name: "provider is unavailable",
			endpoint: func(t *testing.T) string {
				return fakeOllama(t, map[string]http.HandlerFunc{"/api/tags": ollamaStatus(http.StatusInternalServerError)})
			},
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaInventoryMessage(t, "") },
			wantError: constants.ErrInferenceBackendUnavailable.Error(),
		},
		{
			name: "provider reports no installed models",
			endpoint: func(t *testing.T) string {
				return fakeOllama(t, map[string]http.HandlerFunc{"/api/tags": ollamaJSON(`{"models":[]}`)})
			},
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaInventoryMessage(t, "") },
			wantError: constants.ErrInferenceModelNotFound.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newOllamaMaintenanceEnv(t, tt.endpoint(t))

			env.svc.HandleInventoryRequest(t.Context(), tt.message(t))

			var result operatorv1.CommandResult
			envelope := env.publishedResult(t, &result)
			assert.Equal(t, string(constants.Event.Operator.OllamaModelInventory.Failed), envelope.EventType)
			assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
			assert.Contains(t, result.Error, tt.wantError)
		})
	}
}

func TestOllamaMaintenanceService_HandleInventoryRequest_RecordsObservedStateEvidence(t *testing.T) {
	t.Run("success records the completed event with the inventory content", func(t *testing.T) {
		env := newOllamaMaintenanceEnv(t, fakeOllama(t, ollamaInventoryRoutes()))

		env.svc.HandleInventoryRequest(t.Context(), ollamaInventoryMessage(t, "exec-1"))

		events := env.audit.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.OllamaModelInventory.Completed, events[0].Type)
		assert.Equal(t, ollamaTestSessionID, events[0].OperatorSessionID)
		assert.Contains(t, events[0].ContentText, "alias-one:latest")
		assert.True(t, events[0].StoredLocally)
	})

	t.Run("failure records the failed event", func(t *testing.T) {
		env := newOllamaMaintenanceEnv(t, "")

		env.svc.HandleInventoryRequest(t.Context(), ollamaInventoryMessage(t, "exec-1"))

		events := env.audit.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.OllamaModelInventory.Failed, events[0].Type)
	})

	t.Run("evidence failure never blocks the published result", func(t *testing.T) {
		env := newOllamaMaintenanceEnv(t, fakeOllama(t, ollamaInventoryRoutes()))
		env.audit.SetRecordEventError(true)

		env.svc.HandleInventoryRequest(t.Context(), ollamaInventoryMessage(t, "exec-1"))

		var result operatorv1.OllamaModelInventoryResult
		env.publishedResult(t, &result)
		assert.Len(t, result.Entries, 2)
	})
}

func TestOllamaMaintenanceService_HandleResidencyRequest_PublishesTypedResidency(t *testing.T) {
	env := newOllamaMaintenanceEnv(t, fakeOllama(t, map[string]http.HandlerFunc{
		"/api/ps": ollamaJSON(`{"models":[{"name":"qwen3:0.6b"},{"name":"qwen3:1.7b"}]}`),
	}))

	env.svc.HandleResidencyRequest(t.Context(), ollamaResidencyMessage(t, "exec-77"))

	var result operatorv1.OllamaModelResidencyResult
	envelope := env.publishedResult(t, &result)
	assert.Equal(t, string(constants.Event.Operator.OllamaModelResidency.Completed), envelope.EventType)
	assert.Equal(t, "exec-77", result.ExecutionId)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
	require.Len(t, result.Models, 2)
	assert.Equal(t, "qwen3:0.6b", result.Models[0].Name)
	assert.Equal(t, "qwen3:1.7b", result.Models[1].Name)
}

func TestOllamaMaintenanceService_HandleResidencyRequest_NoResidentModelsIsAValidEmptyResult(t *testing.T) {
	env := newOllamaMaintenanceEnv(t, fakeOllama(t, map[string]http.HandlerFunc{"/api/ps": ollamaJSON(`{"models":[]}`)}))

	env.svc.HandleResidencyRequest(t.Context(), ollamaResidencyMessage(t, ""))

	var result operatorv1.OllamaModelResidencyResult
	envelope := env.publishedResult(t, &result)
	assert.Equal(t, string(constants.Event.Operator.OllamaModelResidency.Completed), envelope.EventType)
	assert.Empty(t, result.Models)
	assert.Equal(t, ollamaTestMessageID, result.ExecutionId, "missing execution id falls back to the envelope id")
}

func TestOllamaMaintenanceService_HandleResidencyRequest_PublishesTypedFailure(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  func(t *testing.T) string
		message   func(t *testing.T) *PubSubCommandMessage
		wantError string
	}{
		{
			name: "undecodable request payload",
			endpoint: func(t *testing.T) string {
				return fakeOllama(t, map[string]http.HandlerFunc{"/api/ps": ollamaJSON(`{"models":[]}`)})
			},
			message: func(t *testing.T) *PubSubCommandMessage {
				return ollamaWithPayload(ollamaResidencyMessage(t, ""), []byte{0xff, 0xff})
			},
			wantError: "invalid request payload",
		},
		{
			name:      "provider endpoint is not configured",
			endpoint:  func(*testing.T) string { return "" },
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaResidencyMessage(t, "") },
			wantError: constants.ErrInferenceEndpointInvalid.Error(),
		},
		{
			name: "provider is unavailable",
			endpoint: func(t *testing.T) string {
				return fakeOllama(t, map[string]http.HandlerFunc{"/api/ps": ollamaStatus(http.StatusBadGateway)})
			},
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaResidencyMessage(t, "") },
			wantError: constants.ErrInferenceBackendUnavailable.Error(),
		},
		{
			name: "resident model without a name is rejected",
			endpoint: func(t *testing.T) string {
				return fakeOllama(t, map[string]http.HandlerFunc{"/api/ps": ollamaJSON(`{"models":[{"name":""}]}`)})
			},
			message:   func(t *testing.T) *PubSubCommandMessage { return ollamaResidencyMessage(t, "") },
			wantError: constants.ErrInferenceProviderResponseInvalid.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newOllamaMaintenanceEnv(t, tt.endpoint(t))

			env.svc.HandleResidencyRequest(t.Context(), tt.message(t))

			var result operatorv1.CommandResult
			envelope := env.publishedResult(t, &result)
			assert.Equal(t, string(constants.Event.Operator.OllamaModelResidency.Failed), envelope.EventType)
			assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
			assert.Contains(t, result.Error, tt.wantError)
		})
	}
}

func TestOllamaMaintenanceService_BackendTargetsConfiguredEndpoint(t *testing.T) {
	hits := make(chan string, 4)
	endpoint := fakeOllama(t, map[string]http.HandlerFunc{
		"/api/ps": func(w http.ResponseWriter, r *http.Request) {
			hits <- r.Method + " " + r.URL.Path
			_, _ = w.Write([]byte(`{"models":[]}`))
		},
	})
	env := newOllamaMaintenanceEnv(t, endpoint)

	env.svc.HandleResidencyRequest(t.Context(), ollamaResidencyMessage(t, ""))

	require.Len(t, hits, 1, "residency must be a single read-only provider query")
	assert.Equal(t, "GET /api/ps", <-hits)
}

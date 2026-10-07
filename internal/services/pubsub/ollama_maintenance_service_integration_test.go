// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

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

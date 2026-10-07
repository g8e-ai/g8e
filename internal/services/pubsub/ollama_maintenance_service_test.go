// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"net/http"
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

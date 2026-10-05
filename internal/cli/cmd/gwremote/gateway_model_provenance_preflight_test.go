// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

var (
	provenancePreflightPath = constants.APIPaths.InferenceModelProvenanceAttestations + "_preflight"
	provenanceAttestPath    = constants.APIPaths.InferenceModelProvenanceAttestations + "_attest"
)

// provenanceGateway is a fake Gateway serving the model-provenance preflight and
// attestation endpoints. Every inbound request is recorded for assertions.
type provenanceGateway struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (g *provenanceGateway) record(r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, r)
}

func (g *provenanceGateway) attestQueries() []map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []map[string]string
	for _, r := range g.requests {
		if r.URL.Path != provenanceAttestPath {
			continue
		}
		out = append(out, map[string]string{
			"served_model_tag":      r.URL.Query().Get("served_model_tag"),
			"expected_model_digest": r.URL.Query().Get("expected_model_digest"),
		})
	}
	return out
}

func (g *provenanceGateway) requestCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

// startProvenanceGateway starts a fake Gateway whose responses come from handler,
// points the CLI endpoint override at it, and forces the health check healthy.
func startProvenanceGateway(t *testing.T, handler http.HandlerFunc) (*provenanceGateway, fs.RuntimeFileService, *config.Config) {
	t.Helper()
	WithGatewayHealthCheck(t, true)
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	gateway := &provenanceGateway{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.record(r.Clone(r.Context()))
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	cmdtest.WithEndpointOverride(t, server.URL)
	return gateway, fileSvc, cfg
}

func writeJSONBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	_, err := w.Write([]byte(body))
	require.NoError(t, err)
}

func TestPreflightModelProvenanceDelivery(t *testing.T) {
	t.Run("rejects an unhealthy gateway without any network call", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		err := preflightModelProvenanceDelivery(fileSvc, cfg)
		require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
	})

	t.Run("accepts a ready gateway and hits the preflight endpoint", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSONBody(t, w, `{"status":"ready"}`)
		})

		require.NoError(t, preflightModelProvenanceDelivery(fileSvc, cfg))
		require.Equal(t, 1, gateway.requestCount())
		assert.Equal(t, http.MethodGet, gateway.requests[0].Method)
		assert.Equal(t, provenancePreflightPath, gateway.requests[0].URL.Path)
	})

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErrIs   error
		wantMessage string
	}{
		{
			name: "non-ready status is rejected with the reported status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `{"status":"degraded"}`)
			},
			wantMessage: `unexpected status "degraded"`,
		},
		{
			name: "missing status is rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `{}`)
			},
			wantMessage: `unexpected status ""`,
		},
		{
			name: "response of the wrong JSON shape is an invalid JSON response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `[]`)
			},
			wantErrIs: constants.ErrInvalidJSONResponse,
		},
		{
			name: "gateway HTTP error is wrapped with preflight context",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				writeJSONBody(t, w, `{"error":"provenance operator offline"}`)
			},
			wantErrIs:   constants.ErrHTTPStatusError,
			wantMessage: "model provenance preflight",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fileSvc, cfg := startProvenanceGateway(t, tt.handler)

			err := preflightModelProvenanceDelivery(fileSvc, cfg)
			require.Error(t, err)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantMessage != "" {
				assert.Contains(t, err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestPreflightModelProvenanceAttestation(t *testing.T) {
	const tag = "probe-model:7b"
	digest := strings.Repeat("c", 64)

	t.Run("rejects an unhealthy gateway", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		err := PreflightModelProvenanceAttestation(fileSvc, cfg, tag, digest)
		require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
	})

	t.Run("sends the tag and digest as escaped query parameters", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSONBody(t, w, `{"status":"ready"}`)
		})
		trickyTag := "registry.example/team model:7b&x=1"

		require.NoError(t, PreflightModelProvenanceAttestation(fileSvc, cfg, trickyTag, digest))

		queries := gateway.attestQueries()
		require.Len(t, queries, 1)
		assert.Equal(t, trickyTag, queries[0]["served_model_tag"], "tag must round-trip through URL encoding intact")
		assert.Equal(t, digest, queries[0]["expected_model_digest"])
	})

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErrIs   error
		wantMessage string
	}{
		{
			name: "non-ready status names the model tag",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `{"status":"digest_mismatch"}`)
			},
			wantMessage: `for "probe-model:7b": unexpected status "digest_mismatch"`,
		},
		{
			name: "response of the wrong JSON shape is an invalid JSON response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `"ready"`)
			},
			wantErrIs: constants.ErrInvalidJSONResponse,
		},
		{
			name: "gateway HTTP error names the model tag",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				writeJSONBody(t, w, `{"error":"upstream"}`)
			},
			wantErrIs:   constants.ErrHTTPStatusError,
			wantMessage: `attestation preflight for "probe-model:7b"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fileSvc, cfg := startProvenanceGateway(t, tt.handler)

			err := PreflightModelProvenanceAttestation(fileSvc, cfg, tag, digest)
			require.Error(t, err)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantMessage != "" {
				assert.Contains(t, err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestLoadModelProvenanceAttestation_RejectsUnusableGatewayResponses(t *testing.T) {
	const tag = "probe-model:7b"
	digest := strings.Repeat("d", 64)

	t.Run("unhealthy gateway", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		_, err := LoadModelProvenanceAttestation(fileSvc, cfg, tag, digest)
		require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
	})

	undecodableWindow, err := json.Marshal(models.ModelProvenanceAttestResponse{
		Status: "ready",
		Window: json.RawMessage(`"not a canonical attestation window"`),
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErrIs   error
		wantMessage string
	}{
		{
			name: "wrong JSON shape",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `[]`)
			},
			wantErrIs: constants.ErrInvalidJSONResponse,
		},
		{
			name: "non-ready status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `{"status":"pending"}`)
			},
			wantMessage: `unexpected status "pending"`,
		},
		{
			name: "ready status without an attestation window",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, `{"status":"ready"}`)
			},
			wantMessage: "missing attestation window",
		},
		{
			name: "window bytes that are not a canonical attestation",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(t, w, string(undecodableWindow))
			},
			wantMessage: "decode window",
		},
		{
			name: "gateway HTTP error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSONBody(t, w, `{"error":"boom"}`)
			},
			wantErrIs:   constants.ErrHTTPStatusError,
			wantMessage: `attestation preflight for "probe-model:7b"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fileSvc, cfg := startProvenanceGateway(t, tt.handler)

			window, err := LoadModelProvenanceAttestation(fileSvc, cfg, tag, digest)
			require.Error(t, err)
			assert.Nil(t, window)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantMessage != "" {
				assert.Contains(t, err.Error(), tt.wantMessage)
			}
		})
	}
}

// campaignGatewayHandler answers the delivery preflight as ready and answers
// attestation probes with the status chosen per served model tag (default ready).
func campaignGatewayHandler(t *testing.T, statusByTag map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == provenanceAttestPath {
			status := "ready"
			if override, ok := statusByTag[r.URL.Query().Get("served_model_tag")]; ok {
				status = override
			}
			writeJSONBody(t, w, `{"status":"`+status+`"}`)
			return
		}
		writeJSONBody(t, w, `{"status":"ready"}`)
	}
}

func TestPreflightCampaignModelProvenance(t *testing.T) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)

	t.Run("fails fast when the gateway is unhealthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		err := PreflightCampaignModelProvenance(fileSvc, cfg, []evaluation.CampaignModelBinding{
			{ServedModelTag: "m:1", ModelDigest: digestA},
		})
		require.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
	})

	t.Run("succeeds with no bindings after only the delivery preflight", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, campaignGatewayHandler(t, nil))

		require.NoError(t, PreflightCampaignModelProvenance(fileSvc, cfg, nil))
		assert.Equal(t, 1, gateway.requestCount())
		assert.Empty(t, gateway.attestQueries())
	})

	t.Run("attests each usable binding and skips blank or delegated ones", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, campaignGatewayHandler(t, nil))

		err := PreflightCampaignModelProvenance(fileSvc, cfg, []evaluation.CampaignModelBinding{
			{ServedModelTag: "model-a:1", ModelDigest: digestA},
			{ServedModelTag: "   ", ModelDigest: digestB},
			{ServedModelTag: "model-blank-digest:1", ModelDigest: "  "},
			{ServedModelTag: "model-delegated:1", ModelDigest: evaluation.FormationDelegatedRegistryDigestPlaceholder},
			{ServedModelTag: "model-b:2", ModelDigest: digestB},
		})
		require.NoError(t, err)

		assert.Equal(t, []map[string]string{
			{"served_model_tag": "model-a:1", "expected_model_digest": digestA},
			{"served_model_tag": "model-b:2", "expected_model_digest": digestB},
		}, gateway.attestQueries())
	})

	t.Run("stops at the first failing binding and reports its tag", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, campaignGatewayHandler(t, map[string]string{
			"model-bad:1": "digest_mismatch",
		}))

		err := PreflightCampaignModelProvenance(fileSvc, cfg, []evaluation.CampaignModelBinding{
			{ServedModelTag: "model-ok:1", ModelDigest: digestA},
			{ServedModelTag: "model-bad:1", ModelDigest: digestA},
			{ServedModelTag: "model-never-checked:1", ModelDigest: digestB},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"model-bad:1"`)
		assert.Contains(t, err.Error(), "digest_mismatch")

		var probed []string
		for _, q := range gateway.attestQueries() {
			probed = append(probed, q["served_model_tag"])
		}
		assert.Equal(t, []string{"model-ok:1", "model-bad:1"}, probed, "bindings after the failure must not be probed")
	})

	t.Run("a failing delivery preflight prevents any attestation probe", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSONBody(t, w, `{"status":"offline"}`)
		})

		err := PreflightCampaignModelProvenance(fileSvc, cfg, []evaluation.CampaignModelBinding{
			{ServedModelTag: "model-a:1", ModelDigest: digestA},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unexpected status "offline"`)
		assert.Empty(t, gateway.attestQueries())
	})
}

func TestPreflightModelProvenanceContext_SSEReadyBeforeDispatch(t *testing.T) {
	events := make(chan string, 2)
	connected := make(chan struct{})
	_, fileSvc, cfg := startProvenanceGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.APIPaths.SSEStream:
			w.Header().Set("Content-Type", "text/event-stream")
			require.NotEmpty(t, r.Header.Get(constants.HeaderCLISessionID))
			w.(http.Flusher).Flush()
			close(connected)
			for {
				select {
				case data := <-events:
					fmt.Fprintf(w, "data: %s\n\n", data)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		case provenanceAttestPath:
			select {
			case <-connected:
			default:
				t.Error("attestation dispatched before SSE was ready")
			}
			requestID := r.URL.Query().Get("request_id")
			require.NotEmpty(t, requestID)
			events <- fmt.Sprintf(`{"event":{"type":%q,"data":{"request_id":"unrelated","phase":"failed"}}}`, constants.EventModelProvenancePreflightProgress)
			events <- fmt.Sprintf(`{"event":{"type":%q,"data":{"request_id":%q,"served_model_tag":"probe-model:7b","phase":"attesting_storage"}}}`, constants.EventModelProvenancePreflightProgress, requestID)
			<-r.Context().Done()
		default:
			writeJSONBody(t, w, `{"status":"ready"}`)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var phases []string
	err := preflightModelProvenanceAttestationContext(ctx, fileSvc, cfg, "probe-model:7b", strings.Repeat("a", 64), func(event models.ModelProvenancePreflightProgress) {
		phases = append(phases, event.Phase)
		cancel()
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"attesting_storage"}, phases)
}

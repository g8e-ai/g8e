// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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

func writeJSONBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	_, err := w.Write([]byte(body))
	require.NoError(t, err)
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

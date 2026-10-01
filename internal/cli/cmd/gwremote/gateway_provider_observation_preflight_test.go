// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestPreflightProviderObservationDelivery_AgainstGateway(t *testing.T) {
	preflightPath := constants.APIPaths.InferenceProviderObservations + "_preflight"

	t.Run("accepts a ready gateway and hits the preflight endpoint", func(t *testing.T) {
		gateway, fileSvc, cfg := startProvenanceGateway(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSONBody(t, w, `{"status":"ready"}`)
		})

		require.NoError(t, PreflightProviderObservationDelivery(fileSvc, cfg))
		require.Equal(t, 1, gateway.requestCount())
		assert.Equal(t, http.MethodGet, gateway.requests[0].Method)
		assert.Equal(t, preflightPath, gateway.requests[0].URL.Path)
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
				writeJSONBody(t, w, `{"status":"stalled"}`)
			},
			wantMessage: `provider observation preflight: unexpected status "stalled"`,
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
				w.WriteHeader(http.StatusInternalServerError)
				writeJSONBody(t, w, `{"error":"observer offline"}`)
			},
			wantErrIs:   constants.ErrHTTPStatusError,
			wantMessage: "provider observation preflight",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fileSvc, cfg := startProvenanceGateway(t, tt.handler)

			err := PreflightProviderObservationDelivery(fileSvc, cfg)
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

func TestNewCampaignFormationObservationLoader(t *testing.T) {
	t.Run("returns a loader backed by the gateway reader when healthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, true)
		cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

		loader, err := NewCampaignFormationObservationLoader(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loader)
	})

	t.Run("still builds a local-only loader when the gateway is unhealthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		loader, err := NewCampaignFormationObservationLoader(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loader)
	})
}

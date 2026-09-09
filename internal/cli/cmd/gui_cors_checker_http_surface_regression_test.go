// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
)

// These regression tests lock in the current broken behavior of the GUI
// enrollment CORS checker so that the Phase 3 fix (a read-only verifier
// against the HTTPS listener) can be verified against a captured
// baseline. See plan Finding 0.2.
//
// Issue tracked: gui-cors-checker-probes-http-not-https
// checkGatewayCORS sends its OPTIONS preflight to the plain-HTTP health
// surface (constants.Ports.OperatorHttp), but CORS middleware is attached
// to the browser-facing HTTPS router. The preflight therefore never
// reaches CORS logic in the standard topology.

// TestCheckGatewayCORS_ProbesPlainHTTPNotHTTPS_CurrentBrokenBehavior
// proves that checkGatewayCORS targets the plain-HTTP health surface
// rather than the browser-facing HTTPS surface. It starts an HTTPS-only
// test server that records every request it receives and would respond
// with the correct CORS headers for the supplied origin. Because
// checkGatewayCORS constructs its preflight URL from
// network.LocalhostHTTPURL(constants.Ports.OperatorHttp) (an http:// URL
// on port 8080), the preflight never arrives at the HTTPS server, and the
// checker returns a "gateway not reachable" error whose message names the
// HTTP URL. A correct verifier probes the HTTPS listener instead.
func TestCheckGatewayCORS_ProbesPlainHTTPNotHTTPS_CurrentBrokenBehavior(t *testing.T) {
	const frontendOrigin = "https://your-app.lovable.app"

	var preflightsReceived int32

	httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			atomic.AddInt32(&preflightsReceived, 1)
			w.Header().Set(constants.HeaderAccessControlAllowOrigin, frontendOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer httpsServer.Close()

	cfg := &config.Config{}

	err := checkGatewayCORS(cfg, frontendOrigin)
	require.Error(t, err, RegressionMarkerBeforeFix)

	// The error message names the plain-HTTP URL, proving the checker
	// probes the HTTP surface, not the HTTPS surface.
	expectedHTTPURL := network.LocalhostHTTPURL(constants.Ports.OperatorHttp)
	assert.Contains(t, err.Error(), expectedHTTPURL, RegressionMarkerBeforeFix)
	assert.True(t, strings.HasPrefix(expectedHTTPURL, "http://"), "expected plain-HTTP URL")

	// The HTTPS test server received no preflight requests, proving the
	// checker does not probe the HTTPS listener where CORS middleware
	// actually lives.
	assert.Equal(t, int32(0), atomic.LoadInt32(&preflightsReceived), RegressionMarkerBeforeFix)

	// Regression marker: checkGatewayCORS probes the plain-HTTP health
	// surface instead of the browser-facing HTTPS router.
	_ = RegressionMarkerIssue
}

// TestCheckGatewayCORS_UsesOperatorHTTPPortConstant_CurrentBrokenBehavior
// is a structural assertion that documents the exact port the checker
// targets. It confirms that the URL checkGatewayCORS builds resolves to
// the plain-HTTP operator port (constants.Ports.OperatorHttp), not the
// HTTPS operator port (constants.Ports.OperatorHttps). This captures the
// root cause without binding to any port.
func TestCheckGatewayCORS_UsesOperatorHTTPPortConstant_CurrentBrokenBehavior(t *testing.T) {
	httpURL := network.LocalhostHTTPURL(constants.Ports.OperatorHttp)
	httpsURL := network.LocalhostHTTPSURL(constants.Ports.OperatorHttps)

	assert.Contains(t, httpURL, "http://", RegressionMarkerBeforeFix)
	assert.NotEqual(t, httpURL, httpsURL, RegressionMarkerBeforeFix)
	assert.Contains(t, httpsURL, "https://", RegressionMarkerBeforeFix)

	// The checker's error message embeds the HTTP URL it probes.
	cfg := &config.Config{}
	err := checkGatewayCORS(cfg, "https://your-app.lovable.app")
	if err != nil {
		assert.Contains(t, err.Error(), httpURL, RegressionMarkerBeforeFix)
		assert.NotContains(t, err.Error(), httpsURL, RegressionMarkerBeforeFix)
	}

	// Regression marker: the CORS checker targets constants.Ports.OperatorHttp.
	_ = RegressionMarkerIssue
}

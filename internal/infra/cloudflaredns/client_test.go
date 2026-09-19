// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cloudflaredns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZoneNameForHostname(t *testing.T) {
	zone, err := zoneNameForHostname("opendevops.ai")
	require.NoError(t, err)
	assert.Equal(t, "opendevops.ai", zone)

	zone, err = zoneNameForHostname("www.opendevops.ai")
	require.NoError(t, err)
	assert.Equal(t, "opendevops.ai", zone)
}

func TestDNSRecordName(t *testing.T) {
	assert.Equal(t, "opendevops.ai", dnsRecordName("opendevops.ai"))
	assert.Equal(t, "www", dnsRecordName("www.opendevops.ai"))
}

func TestUpsertTunnelCNAMERoutesApex(t *testing.T) {
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/zones") && !strings.Contains(r.URL.Path, "/dns_records"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  []map[string]string{{"id": "zone-123", "name": "opendevops.ai"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/dns_records"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/dns_records"):
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"content":"tunnel-id.cfargotunnel.com"`)
			assert.Contains(t, string(body), `"name":"opendevops.ai"`)
			created = true
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]string{"id": "rec-1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		token:  "token",
		client: server.Client(),
	}
	client.client = &http.Client{
		Transport: &rewriteTransport{base: server.Client().Transport, target: server.URL},
	}

	err := client.UpsertTunnelCNAME(context.Background(), "opendevops.ai", "tunnel-id")
	require.NoError(t, err)
	assert.True(t, created)
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.target, "https://"), "http://")
	return t.base.RoundTrip(req)
}

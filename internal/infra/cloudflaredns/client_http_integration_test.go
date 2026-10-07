// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package cloudflaredns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeCloudflareClient wires a Client to an httptest server running handler.
func newFakeCloudflareClient(t *testing.T, handler http.HandlerFunc) (*Client, *fakeCloudflare) {
	t.Helper()
	fake := &fakeCloudflare{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.record(r)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return &Client{
		token: "test-token",
		client: &http.Client{
			Transport: &rewriteTransport{base: server.Client().Transport, target: server.URL},
		},
	}, fake
}

func TestZoneIDForHostname(t *testing.T) {
	t.Run("returns the first matching zone and queries by zone name with bearer auth", func(t *testing.T) {
		client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeAPIResult(t, w, []map[string]string{
				{"id": "zone-first", "name": "example.com"},
				{"id": "zone-second", "name": "example.com"},
			})
		})

		id, err := client.ZoneIDForHostname(context.Background(), "deep.sub.example.com")
		require.NoError(t, err)
		assert.Equal(t, "zone-first", id)

		reqs := fake.snapshot()
		require.Len(t, reqs, 1)
		assert.Equal(t, http.MethodGet, reqs[0].Method)
		assert.Equal(t, "/client/v4/zones", reqs[0].Path)
		assert.Equal(t, "name=example.com", reqs[0].Query)
		assert.Equal(t, "Bearer test-token", reqs[0].Authorization)
		assert.Equal(t, "application/json", reqs[0].ContentType)
	})

	t.Run("fails when the account owns no matching zone", func(t *testing.T) {
		client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeAPIResult(t, w, []map[string]string{})
		})

		_, err := client.ZoneIDForHostname(context.Background(), "www.example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `zone "example.com" not found`)
	})

	t.Run("rejects an invalid hostname without calling the API", func(t *testing.T) {
		client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("API must not be called for an invalid hostname")
		})

		_, err := client.ZoneIDForHostname(context.Background(), "localhost")
		require.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Empty(t, fake.snapshot())
	})

	t.Run("surfaces the first API error message", func(t *testing.T) {
		client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(t, w, http.StatusForbidden, "Authentication error")
		})

		_, err := client.ZoneIDForHostname(context.Background(), "example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Authentication error")
	})
}

func TestUpsertTunnelCNAME_Validation(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		tunnelID string
	}{
		{name: "missing hostname", hostname: "", tunnelID: "tunnel"},
		{name: "missing tunnel id", hostname: "example.com", tunnelID: ""},
		{name: "whitespace-only hostname", hostname: "  ", tunnelID: "tunnel"},
		{name: "whitespace-only tunnel id", hostname: "example.com", tunnelID: "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("API must not be called when required fields are missing")
			})

			err := client.UpsertTunnelCNAME(context.Background(), tt.hostname, tt.tunnelID)
			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
			assert.Empty(t, fake.snapshot())
		})
	}
}

func TestUpsertTunnelCNAME_PatchesEveryExistingTunnelRecord(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, upsertHandler(t, []map[string]any{
		{"id": "rec-cname", "type": "CNAME", "name": "api.example.com"},
		{"id": "rec-tunnel", "type": "Tunnel", "name": "api.example.com"},
	}))

	require.NoError(t, client.UpsertTunnelCNAME(context.Background(), " api.example.com ", " tunnel-9 "))

	reqs := fake.snapshot()
	require.Len(t, reqs, 4, "zone lookup, record list, then one PATCH per existing record")
	assert.Equal(t, "name=api", reqs[1].Query, "record lookup uses the zone-relative name")

	for i, wantID := range []string{"rec-cname", "rec-tunnel"} {
		patch := reqs[2+i]
		assert.Equal(t, http.MethodPatch, patch.Method)
		assert.Equal(t, "/client/v4/zones/zone-1/dns_records/"+wantID, patch.Path)

		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(patch.Body), &body))
		assert.Equal(t, "CNAME", body["type"])
		assert.Equal(t, "api", body["name"])
		assert.Equal(t, "tunnel-9.cfargotunnel.com", body["content"])
		assert.Equal(t, true, body["proxied"])
		assert.EqualValues(t, 1, body["ttl"], "ttl 1 is Cloudflare's automatic TTL")
	}
}

func TestUpsertTunnelCNAME_RefusesToOverwriteNonTunnelRecord(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, upsertHandler(t, []map[string]any{
		{"id": "rec-a", "type": "A", "name": "api.example.com"},
	}))

	err := client.UpsertTunnelCNAME(context.Background(), "api.example.com", "tunnel-9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing A record")
	assert.Contains(t, err.Error(), "blocks tunnel routing")

	for _, req := range fake.snapshot() {
		assert.Equal(t, http.MethodGet, req.Method, "no mutation may be sent when a foreign record blocks routing")
	}
}

func TestUpsertTunnelCNAME_PropagatesBackendFailures(t *testing.T) {
	tests := []struct {
		name        string
		failMethod  string
		failPath    string
		wantMessage string
	}{
		{name: "record listing fails", failMethod: http.MethodGet, failPath: "/client/v4/zones/zone-1/dns_records", wantMessage: "list exploded"},
		{name: "create fails", failMethod: http.MethodPost, failPath: "/client/v4/zones/zone-1/dns_records", wantMessage: "create exploded"},
		{name: "update fails", failMethod: http.MethodPatch, failPath: "/client/v4/zones/zone-1/dns_records/rec-1", wantMessage: "patch exploded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The update case needs an existing record so PATCH is reached.
			var existing []map[string]any
			if tt.failMethod == http.MethodPatch {
				existing = []map[string]any{{"id": "rec-1", "type": "CNAME", "name": "api.example.com"}}
			}
			ok := upsertHandler(t, existing)
			client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == tt.failMethod && r.URL.Path == tt.failPath {
					writeAPIError(t, w, http.StatusBadRequest, tt.wantMessage)
					return
				}
				ok(w, r)
			})

			err := client.UpsertTunnelCNAME(context.Background(), "api.example.com", "tunnel-9")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestUpsertTunnelCNAME_CreatesRecordWhenNoneExists(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, upsertHandler(t, []map[string]any{}))

	require.NoError(t, client.UpsertTunnelCNAME(context.Background(), "api.example.com", "tunnel-9"))

	reqs := fake.snapshot()
	require.Len(t, reqs, 3)
	assert.Equal(t, http.MethodPost, reqs[2].Method)
	assert.Equal(t, "/client/v4/zones/zone-1/dns_records", reqs[2].Path)
}

func TestDoJSON_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantMessage string
	}{
		{
			name: "non-JSON response body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "<html>bad gateway</html>")
			},
			wantMessage: "decode response",
		},
		{
			name: "success false with no error details reports the HTTP status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"success":false,"errors":[]}`)
			},
			wantMessage: "HTTP 502",
		},
		{
			name: "success false reports only the first error message",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":1,"message":"first"},{"code":2,"message":"second"}]}`)
			},
			wantMessage: "cloudflare dns: first",
		},
		{
			name: "result does not match the expected shape",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"success":true,"result":{"not":"a list"}}`)
			},
			wantMessage: "decode result",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newFakeCloudflareClient(t, tt.handler)

			var zones []zoneResult
			err := client.getJSON(context.Background(), "/zones", &zones)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
			assert.NotContains(t, err.Error(), "second")
		})
	}
}

func TestDoJSON_SkipsDecodingWhenCallerWantsNoResult(t *testing.T) {
	client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"result":{"anything":"goes"}}`)
	})

	require.NoError(t, client.getJSON(context.Background(), "/zones", nil))
}

func TestDoJSON_SucceedsWithEmptyResult(t *testing.T) {
	client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true}`)
	})

	var zones []zoneResult
	require.NoError(t, client.getJSON(context.Background(), "/zones", &zones))
	assert.Empty(t, zones)
}

func TestDoJSON_ReturnsTransportErrorWhenServerIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := server.URL
	transport := server.Client().Transport
	server.Close()

	client := &Client{
		token:  "test-token",
		client: &http.Client{Transport: &rewriteTransport{base: transport, target: target}},
	}

	err := client.getJSON(context.Background(), "/zones", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestDoJSON_HonorsContextCancellation(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeAPIResult(t, w, []any{})
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.getJSON(ctx, "/zones", nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, fake.snapshot())
}

func TestDoJSON_RejectsUnencodableBodyBeforeSending(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called when the body cannot be encoded")
	})

	err := client.postJSON(context.Background(), "/zones", map[string]any{"bad": make(chan int)}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode request")
	assert.Empty(t, fake.snapshot())
}

func TestDoJSON_RejectsMalformedRequestPath(t *testing.T) {
	client, _ := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called when the request cannot be built")
	})

	err := client.getJSON(context.Background(), "/zones\n", nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "build request"), "got %v", err)
}

func TestDoJSON_SendsJSONBodyForPostAndPatch(t *testing.T) {
	client, fake := newFakeCloudflareClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeAPIResult(t, w, map[string]string{"id": "x"})
	})
	payload := map[string]any{"type": "CNAME"}

	require.NoError(t, client.postJSON(context.Background(), "/p", payload, nil))
	require.NoError(t, client.patchJSON(context.Background(), "/p", payload, nil))

	reqs := fake.snapshot()
	require.Len(t, reqs, 2)
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Equal(t, http.MethodPatch, reqs[1].Method)
	for _, req := range reqs {
		assert.JSONEq(t, `{"type":"CNAME"}`, req.Body)
		assert.Equal(t, "Bearer test-token", req.Authorization)
	}
}

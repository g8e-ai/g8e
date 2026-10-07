// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cloudflaredns

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedRequest captures one request observed by the fake Cloudflare API.
type recordedRequest struct {
	Method        string
	Path          string
	Query         string
	Authorization string
	ContentType   string
	Body          string
}

// fakeCloudflare is an httptest server whose requests are recorded so tests can
// assert on the exact wire traffic the client produced.
type fakeCloudflare struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (f *fakeCloudflare) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recordedRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Authorization: r.Header.Get("Authorization"),
		ContentType:   r.Header.Get("Content-Type"),
		Body:          string(body),
	})
}

func (f *fakeCloudflare) snapshot() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func writeAPIResult(t *testing.T, w http.ResponseWriter, result any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result}))
}

func writeAPIError(t *testing.T, w http.ResponseWriter, status int, message string) {
	t.Helper()
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"errors":  []map[string]any{{"code": 1000, "message": message}},
	}))
}

func TestNewClient(t *testing.T) {
	t.Run("rejects empty and whitespace-only tokens", func(t *testing.T) {
		for _, token := range []string{"", "   ", "\t\n"} {
			client, err := NewClient(token)
			require.ErrorIs(t, err, constants.ErrMissingRequiredField, "token %q", token)
			assert.Nil(t, client)
		}
	})

	t.Run("trims surrounding whitespace and sets a request timeout", func(t *testing.T) {
		client, err := NewClient("  secret-token \n")
		require.NoError(t, err)
		assert.Equal(t, "secret-token", client.token)
		require.NotNil(t, client.client)
		assert.Positive(t, client.client.Timeout)
	})
}

func TestZoneNameForHostname_Table(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
		wantErr  error
	}{
		{name: "apex domain", hostname: "example.com", want: "example.com"},
		{name: "single subdomain", hostname: "www.example.com", want: "example.com"},
		{name: "deep subdomain collapses to the last two labels", hostname: "a.b.c.example.com", want: "example.com"},
		{name: "trailing dot is stripped", hostname: "www.example.com.", want: "example.com"},
		{name: "empty hostname", hostname: "", wantErr: constants.ErrMissingRequiredField},
		{name: "whitespace-only hostname", hostname: "   ", wantErr: constants.ErrMissingRequiredField},
		{name: "lone dot is empty after trimming", hostname: ".", wantErr: constants.ErrMissingRequiredField},
		{name: "single label has no zone", hostname: "localhost", wantErr: constants.ErrValidationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := zoneNameForHostname(tt.hostname)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDNSRecordName_Table(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{name: "apex keeps the zone name", hostname: "example.com", want: "example.com"},
		{name: "subdomain drops the zone suffix", hostname: "api.example.com", want: "api"},
		{name: "nested subdomain keeps every label left of the zone", hostname: "a.b.example.com", want: "a.b"},
		{name: "single label falls back to the input", hostname: "localhost", want: "localhost"},
		{name: "empty input falls back to the input", hostname: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dnsRecordName(tt.hostname))
		})
	}
}

// upsertHandler serves a zone lookup plus the given existing records, and
// answers any mutation with success.
func upsertHandler(t *testing.T, existing []map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/client/v4/zones":
			writeAPIResult(t, w, []map[string]string{{"id": "zone-1", "name": "example.com"}})
		case r.Method == http.MethodGet && r.URL.Path == "/client/v4/zones/zone-1/dns_records":
			writeAPIResult(t, w, existing)
		default:
			writeAPIResult(t, w, map[string]string{"id": "rec"})
		}
	}
}

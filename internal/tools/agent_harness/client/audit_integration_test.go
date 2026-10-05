// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

func TestAuditReceipts(t *testing.T) {
	tests := []struct {
		name              string
		operatorSessionID string
		responseBody      string
		wantErr           bool
		verifyQuery       func(url.Values)
	}{
		{
			name:              "successful retrieval without session ID",
			operatorSessionID: "",
			responseBody:      `{"receipts":[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]}`,
			wantErr:           false,
			verifyQuery: func(v url.Values) {
				if v.Get("operator_session_id") != "" {
					t.Error("should not have operator_session_id query param")
				}
			},
		},
		{
			name:              "successful retrieval with session ID",
			operatorSessionID: "session-123",
			responseBody:      `{"receipts":[{"transaction_id":"tx1"}]}`,
			wantErr:           false,
			verifyQuery: func(v url.Values) {
				if v.Get("operator_session_id") != "session-123" {
					t.Errorf("expected operator_session_id session-123, got %s", v.Get("operator_session_id"))
				}
			},
		},
		{
			name:              "bare array response",
			operatorSessionID: "",
			responseBody:      `[{"transaction_id":"tx1"}]`,
			wantErr:           false,
		},
		{
			name:              "empty receipts",
			operatorSessionID: "",
			responseBody:      `{"receipts":[]}`,
			wantErr:           false,
		},
		{
			name:              "server error",
			operatorSessionID: "",
			responseBody:      `{"error": "internal error"}`,
			wantErr:           false, // AuditReceipts is lenient like ExportReceipts
		},
		{
			name:              "network error",
			operatorSessionID: "",
			responseBody:      ``,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedQuery url.Values

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != constants.APIPaths.AuditReceipts {
					t.Errorf("expected path %s, got %s", constants.APIPaths.AuditReceipts, r.URL.Path)
				}

				receivedQuery = r.URL.Query()
				if tt.verifyQuery != nil {
					tt.verifyQuery(receivedQuery)
				}

				if tt.name == "network error" {
					// Close connection immediately
					hj, ok := w.(http.Hijacker)
					if ok {
						conn, _, _ := hj.Hijack()
						conn.Close()
						return
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			receipts, body, err := client.AuditReceipts(ctx, tt.operatorSessionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("AuditReceipts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(body) == 0 && tt.responseBody != "" {
					t.Error("expected non-empty body")
				}
				if tt.name == "empty receipts" && len(receipts) != 0 {
					t.Errorf("expected 0 receipts, got %d", len(receipts))
				}
				if tt.name == "successful retrieval without session ID" && len(receipts) != 2 {
					t.Errorf("expected 2 receipts, got %d", len(receipts))
				}
			}
		})
	}
}

func TestExportReceipts(t *testing.T) {
	tests := []struct {
		name              string
		operatorSessionID string
		responseBody      []byte
		wantErr           bool
	}{
		{
			name:              "successful export without session ID",
			operatorSessionID: "",
			responseBody:      []byte(`{"receipts":[{"transaction_id":"tx1"}]}`),
			wantErr:           false,
		},
		{
			name:              "successful export with session ID",
			operatorSessionID: "session-123",
			responseBody:      []byte(`{"receipts":[{"transaction_id":"tx1"}]}`),
			wantErr:           false,
		},
		{
			name:              "empty export",
			operatorSessionID: "",
			responseBody:      []byte(`{}`),
			wantErr:           false,
		},
		{
			name:              "server error",
			operatorSessionID: "",
			responseBody:      []byte(`{"error": "internal error"}`),
			wantErr:           false, // ExportReceipts doesn't check status codes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedQuery url.Values

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != constants.APIPaths.AuditReceiptsExport {
					t.Errorf("expected path %s, got %s", constants.APIPaths.AuditReceiptsExport, r.URL.Path)
				}

				receivedQuery = r.URL.Query()
				if tt.operatorSessionID != "" {
					if receivedQuery.Get("operator_session_id") != tt.operatorSessionID {
						t.Errorf("expected operator_session_id %s, got %s", tt.operatorSessionID, receivedQuery.Get("operator_session_id"))
					}
				} else {
					if receivedQuery.Get("operator_session_id") != "" {
						t.Error("should not have operator_session_id query param when empty")
					}
				}

				w.Write(tt.responseBody)
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			body, err := client.ExportReceipts(ctx, tt.operatorSessionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExportReceipts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(body) == 0 && len(tt.responseBody) > 0 {
					t.Error("expected non-empty body")
				}
			}
		})
	}
}

func TestDiscoverOperatorKeepsIdentityPaired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"operators":[{"id":"embedded-operator","operator_session_id":"embedded-session","status":"active","operator_type":"embedded"},{"id":"data-worker","operator_session_id":"data-session","status":"active","operator_type":"remote","operator_role":"data"}]}`))
	}))
	t.Cleanup(server.Close)
	for _, tt := range []struct{ name, id, session, wantID, wantSession string }{
		{"select data worker", "", "", "data-worker", "data-session"},
		{"resolve logical ID", "data-worker", "", "data-worker", "data-session"},
		{"resolve session", "", "data-session", "data-worker", "data-session"},
		{"reject conflicting pair", "embedded-operator", "data-session", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(config.Config{MTLSBaseURL: server.URL, UserID: "user-123", OperatorID: tt.id, OperatorSessionID: tt.session})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			id, session, discoverErr := c.DiscoverOperator(ctx)
			if tt.wantID == "" {
				if discoverErr == nil {
					t.Fatal("expected discovery rejection")
				}
			} else if discoverErr != nil {
				t.Fatal(discoverErr)
			}
			if id != tt.wantID || session != tt.wantSession {
				t.Fatalf("got (%q, %q), want (%q, %q)", id, session, tt.wantID, tt.wantSession)
			}
		})
	}
}

func TestDiscoverOperatorRejectsUnresolvedTargets(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"missing target", `{"success":true,"operators":[]}`, http.StatusOK, constants.ErrEvaluationTargetUnavailable},
		{"registry unauthorized", `{}`, http.StatusUnauthorized, constants.ErrHTTPStatusError},
		{"malformed registry", `broken`, http.StatusOK, constants.ErrInvalidJSONResponse},
		{"incomplete registry", `{"success":false}`, http.StatusOK, constants.ErrInvalidJSONResponse},
		{"ambiguous data workers", `{"success":true,"operators":[{"id":"one","operator_session_id":"s1","status":"active","operator_type":"remote","operator_role":"data"},{"id":"two","operator_session_id":"s2","status":"active","operator_type":"remote","operator_role":"data"}]}`, http.StatusOK, constants.ErrEvaluationTargetAmbiguous},
		{"inactive worker", `{"success":true,"operators":[{"id":"one","operator_session_id":"s1","status":"inactive","operator_type":"remote","operator_role":"data"}]}`, http.StatusOK, constants.ErrEvaluationTargetUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.status); w.Write([]byte(tt.body)) }))
			t.Cleanup(srv.Close)
			c, err := New(config.Config{MTLSBaseURL: srv.URL, UserID: "user-123"})
			if err != nil {
				t.Fatal(err)
			}
			id, session, err := c.DiscoverOperator(t.Context())
			if id != "" || session != "" || !errors.Is(err, tt.want) {
				t.Fatalf("got (%q,%q,%v), want %v", id, session, err, tt.want)
			}
		})
	}
}

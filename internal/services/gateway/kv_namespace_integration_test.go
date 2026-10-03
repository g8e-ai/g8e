// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func kvAppRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, constants.APIPaths.KVPrefix+path, strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), constants.ContextKeyAppID, "cache-app"))
}

func TestKVNamespace_RejectsAuthorityKeysAndUnscopedPatterns(t *testing.T) {
	c, infra := setupTestDataController(t)
	require.NoError(t, infra.KVStore.KVSet("uei_token_secret", "protected", 0))
	for _, key := range []string{"g8e:sessions:operator:session", "uei_token_secret", "g8e:cache:other:record"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run(method+"/"+key, func(t *testing.T) {
				rr := httptest.NewRecorder()
				c.handleKV(rr, kvAppRequest(method, key, `{"value":"forged"}`))
				require.Equal(t, http.StatusForbidden, rr.Code)
			})
		}
		for _, suffix := range []string{"/_ttl", "/_expire"} {
			t.Run(key+suffix, func(t *testing.T) {
				rr := httptest.NewRecorder()
				c.handleKV(rr, kvAppRequest(http.MethodPut, key+suffix, `{"ttl":60}`))
				require.Equal(t, http.StatusForbidden, rr.Code)
			})
		}
	}
	for _, endpoint := range []string{"_keys", "_scan", "_delete_pattern"} {
		for _, pattern := range []string{"*", "g8e:*", "g8e:cache:*", "g8e:cache:doc*", "uei_token_*"} {
			t.Run(endpoint+"/"+pattern, func(t *testing.T) {
				rr := httptest.NewRecorder()
				c.handleKV(rr, kvAppRequest(http.MethodPost, endpoint, `{"pattern":"`+pattern+`"}`))
				require.Equal(t, http.StatusForbidden, rr.Code)
			})
		}
	}
	value, found := infra.KVStore.KVGet("uei_token_secret")
	require.True(t, found)
	require.Equal(t, "protected", value)
}

func TestKVNamespace_CacheWritesAreObserved(t *testing.T) {
	c, infra := setupTestDataController(t)
	for _, key := range []string{"g8e:cache:doc:cases:case-1", "g8e:cache:query:cases:query-1"} {
		rr := httptest.NewRecorder()
		c.handleKV(rr, kvAppRequest(http.MethodPut, key, `{"value":"cached"}`))
		require.Equal(t, http.StatusOK, rr.Code)
		var tier string
		require.NoError(t, infra.KVStore.db.QueryRow("SELECT state_tier FROM kv_store WHERE key = ?", key).Scan(&tier))
		require.Equal(t, "observed", tier)
	}
}

func TestKVNamespaceRouter_AuthenticatesBeforeApplyingCacheScope(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	seedAppPolicy(t, infra, "spiffe://g8e.local/app/g8ee")
	for _, authenticated := range []bool{false, true} {
		for _, tc := range []struct {
			method string
			path   string
			body   string
			status int
		}{
			{http.MethodPut, "g8e:cache:doc:cases:case-1", `{"value":"cached"}`, http.StatusOK},
			{http.MethodPut, "g8e:sessions:operator:forged", `{"value":"forged"}`, http.StatusForbidden},
			{http.MethodPost, "_keys", `{"pattern":"*"}`, http.StatusForbidden},
		} {
			req := httptest.NewRequest(tc.method, constants.APIPaths.KVPrefix+tc.path, strings.NewReader(tc.body))
			if authenticated {
				req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{appOnlyMTLSCert(t)}}
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			want := tc.status
			if !authenticated {
				want = http.StatusUnauthorized
			}
			require.Equal(t, want, rr.Code, rr.Body.String())
		}
	}
	_, found := infra.KVStore.KVGet("g8e:sessions:operator:forged")
	require.False(t, found)
}

func TestKVStore_CacheMutationsDoNotAdvanceStateVersion(t *testing.T) {
	store := setupKVStore(t)
	version := func() int64 {
		var value int64
		require.NoError(t, store.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&value))
		return value
	}
	before := version()
	key := "g8e:cache:doc:cases:case-1"
	require.NoError(t, store.KVSetObserved(key, "first", 0))
	require.Equal(t, before, version(), "cache insertion")
	require.NoError(t, store.KVSetObserved(key, "second", 0))
	require.Equal(t, before, version(), "cache update")
	require.True(t, store.KVExpire(key, 60))
	require.Equal(t, before, version(), "cache expiry")
	require.NoError(t, store.KVDelete(key))
	require.Equal(t, before, version(), "cache deletion")
	require.NoError(t, store.KVSet("bound-key", "bound", 0))
	require.Greater(t, version(), before, "bound insertion still advances version")
}

func TestKVStore_SchemaReopenReplacesOldCacheVersionTriggers(t *testing.T) {
	store := setupKVStore(t)
	_, err := store.db.Exec(`DROP TRIGGER trg_kv_store_insert_version;
		CREATE TRIGGER trg_kv_store_insert_version AFTER INSERT ON kv_store
		BEGIN UPDATE state_version SET version = version + 1 WHERE id = 1; END;`)
	require.NoError(t, err)
	_, err = store.db.Exec(gatewaySchema)
	require.NoError(t, err)
	var before, after int64
	require.NoError(t, store.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&before))
	require.NoError(t, store.KVSetObserved("g8e:cache:query:cases:query-1", "[]", 0))
	require.NoError(t, store.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&after))
	require.Equal(t, before, after)
}

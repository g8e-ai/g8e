// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestReputationSigningRouter_AppUsesGatewayKeyWithoutExport(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	seedAppPolicy(t, infra, "spiffe://g8e.local/app/g8ee")
	h.dataController.auditorKeyProvider = infra.SecretMgr
	root, previous := strings.Repeat("a", 64), strings.Repeat("0", 64)
	body := `{"merkle_root":"` + root + `","prev_root":"` + previous + `","tribunal_command_id":"verdict-1"}`
	for _, authenticated := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GatewayReputationSign, strings.NewReader(body))
		if authenticated {
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{appOnlyMTLSCert(t)}}
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if !authenticated {
			require.Equal(t, http.StatusUnauthorized, rr.Code)
			continue
		}
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var signed models.ReputationSignResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &signed))
		key, err := infra.SecretMgr.GetAuditorHMACKey()
		require.NoError(t, err)
		mac := hmac.New(sha256.New, []byte(key))
		_, err = mac.Write([]byte(root + previous + "verdict-1"))
		require.NoError(t, err)
		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), signed.Signature)
		require.NotContains(t, rr.Body.String(), key)
	}
	for _, invalid := range []string{`{}`, `{"merkle_root":"bad","prev_root":"bad","tribunal_command_id":"verdict-1"}`, `{"merkle_root":"` + root + `","prev_root":"` + previous + `","tribunal_command_id":""}`} {
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GatewayReputationSign, strings.NewReader(invalid))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{appOnlyMTLSCert(t)}}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	}
	h.dataController.auditorKeyProvider = nil
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GatewayReputationSign, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{appOnlyMTLSCert(t)}}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

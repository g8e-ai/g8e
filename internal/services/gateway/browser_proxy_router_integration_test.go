// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestBrowserProxyRouter_MTLSKeyAndSignedBrowserRequests(t *testing.T) {
	type forwardedRequest struct {
		method string
		uri    string
		body   []byte
		header http.Header
	}
	forwarded := make(chan forwardedRequest, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		forwarded <- forwardedRequest{r.Method, r.URL.RequestURI(), body, r.Header.Clone()}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	h, cfg, infra := setupTestHTTPHandler(t)
	cfg.Gateway.EnsembleUpstreamURL = upstream.URL
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := NewBrowserProxySigner(key, "router-test-key")
	require.NoError(t, err)
	controller := h.ensembleBrowserProxyController
	controller.cfg = cfg
	controller.logger = infra.Logger
	controller.responder = infra.Responder
	controller.operators = &gatewayOperatorListerAdapter{svc: infra.Reg}
	controller.signer = signer
	userID := "proxy-router-user"
	seedActiveUser(t, infra, userID)
	sessionID := seedWebSession(t, infra, userID)
	seedAppPolicy(t, infra, "spiffe://g8e.local/app/g8ee")

	for _, credential := range []string{"none", "browser cookie", "bearer"} {
		t.Run("key rejects "+credential, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, constants.APIPaths.GatewayProxySigningKey, nil)
			if credential == "browser cookie" {
				req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: sessionID})
			}
			if credential == "bearer" {
				req.Header.Set("Authorization", "Bearer "+sessionID)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), constants.ErrMTLSCertRequired.Error())
		})
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.GatewayProxySigningKey, nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{appOnlyMTLSCert(t)}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var exported models.ActuatorPublicKeyExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))
	publicKey, err := hex.DecodeString(exported.PublicKey)
	require.NoError(t, err)
	require.Equal(t, "router-test-key", exported.KeyID)

	for _, path := range []string{constants.APIPaths.EnsembleChatPrefix + "/send?stream=1", constants.APIPaths.EnsembleSettingsPrefix + "/llm/get"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"message":"router test"}`))
			req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: sessionID})
			req.Header.Set(constants.HeaderProxyUserID, "forged-user")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
			got := <-forwarded
			require.Equal(t, userID, got.header.Get(constants.HeaderProxyUserID))
			require.Equal(t, sessionID, got.header.Get(constants.HeaderProxyWebSessionID))
			require.Empty(t, got.header.Get("X-G8E-Gateway-Browser-Proxy"))
			issued, err := strconv.ParseInt(got.header.Get(constants.HeaderProxyIssuedAt), 10, 64)
			require.NoError(t, err)
			digest := sha256.Sum256(got.body)
			canonical, err := (BrowserProxyStamp{
				Method: got.method, RequestURI: got.uri, BodySHA256: hex.EncodeToString(digest[:]),
				UserID: userID, UserEmail: got.header.Get(constants.HeaderProxyUserEmail), WebSessionID: sessionID,
				IssuedAtUnix: issued, Nonce: got.header.Get(constants.HeaderProxyNonce),
			}).CanonicalBytes()
			require.NoError(t, err)
			signature, err := hex.DecodeString(got.header.Get(constants.HeaderProxySignature))
			require.NoError(t, err)
			require.True(t, ed25519.Verify(publicKey, canonical, signature), "the mTLS-exported key must verify the exact forwarded request")
		})
	}
}
